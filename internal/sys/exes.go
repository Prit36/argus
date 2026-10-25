package sys

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// Exe is what is known of a program file once read: its path, and its
// version resource's description and company. It doesn't change once
// made, so any goroutine may read it.
type Exe struct {
	// Path is as Windows reports it; Key is it lower-cased, what it's
	// kept by.
	Path, Key string
	// File is the file name, lower-cased: what groups processes.
	File        string
	Description string
	Company     string
}

// ExeCache keeps every program file read, and the names of the accounts
// processes run as. Safe for concurrent use.
type ExeCache struct {
	mu    sync.Mutex
	byKey map[string]*Exe
	users map[string]string

	devOnce sync.Once
	devices []device
}

// device is a volume's NT name ("\device\harddiskvolume3", lower-cased)
// and drive ("C:").
type device struct{ nt, dos string }

func NewExeCache() *ExeCache {
	return &ExeCache{byKey: make(map[string]*Exe, 256), users: make(map[string]string, 8)}
}

// Get returns the program at path, reading it the first time.
func (c *ExeCache) Get(path string) *Exe {
	key := strings.ToLower(path)
	c.mu.Lock()
	e := c.byKey[key]
	c.mu.Unlock()
	if e != nil {
		return e
	}
	e = &Exe{Path: path, Key: key, File: filepath.Base(key)}
	readVersion(e)
	c.mu.Lock()
	if old := c.byKey[key]; old != nil {
		e = old
	} else {
		c.byKey[key] = e
	}
	c.mu.Unlock()
	return e
}

// Lookup returns the program at path if it was read.
func (c *ExeCache) Lookup(path string) *Exe {
	key := strings.ToLower(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byKey[key]
}

func readVersion(e *Exe) {
	size, err := windows.GetFileVersionInfoSize(e.Path, nil)
	if err != nil || size == 0 || size > 1<<20 {
		return
	}
	data := make([]byte, size)
	if windows.GetFileVersionInfo(e.Path, 0, size, unsafe.Pointer(&data[0])) != nil {
		return
	}
	// The first language the file has, else US English in Unicode.
	lang := [2]uint16{0x0409, 0x04b0}
	var tp unsafe.Pointer
	var tl uint32
	if windows.VerQueryValue(unsafe.Pointer(&data[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&tp), &tl) == nil && tl >= 4 {
		lang = *(*[2]uint16)(tp)
	}
	e.Description = versionString(data, lang, "FileDescription")
	e.Company = versionString(data, lang, "CompanyName")
	// Some files name only US English.
	us := [2]uint16{0x0409, 0x04b0}
	if e.Description == "" && lang != us {
		e.Description = versionString(data, us, "FileDescription")
	}
	if e.Company == "" && lang != us {
		e.Company = versionString(data, us, "CompanyName")
	}
}

func versionString(data []byte, lang [2]uint16, name string) string {
	sub := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lang[0], lang[1], name)
	var vp unsafe.Pointer
	var vl uint32
	if windows.VerQueryValue(unsafe.Pointer(&data[0]), sub, unsafe.Pointer(&vp), &vl) != nil || vl == 0 {
		return ""
	}
	s := unsafe.Slice((*uint16)(vp), vl)
	for len(s) > 0 && s[len(s)-1] == 0 {
		s = s[:len(s)-1]
	}
	return strings.TrimSpace(windows.UTF16ToString(s))
}

// UserOf is the account a process runs as ("SYSTEM", "LOCAL SERVICE",
// the user), "" if its token can't be read.
func (c *ExeCache) UserOf(process windows.Handle) string {
	var tok windows.Token
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &tok) != nil {
		return ""
	}
	defer tok.Close()
	tu, err := tok.GetTokenUser()
	if err != nil {
		return ""
	}
	sid := tu.User.Sid
	// The SID's bytes are its key: revision, count, authority, parts.
	b := unsafe.Slice((*byte)(unsafe.Pointer(sid)), 8+4*int(*(*byte)(unsafe.Add(unsafe.Pointer(sid), 1))))
	c.mu.Lock()
	u, ok := c.users[string(b)]
	c.mu.Unlock()
	if ok {
		return u
	}
	u, _ = winapi.LookupAccountSid(sid)
	c.mu.Lock()
	c.users[string(b)] = u
	c.mu.Unlock()
	return u
}

// NTPath is the image path of pid from the kernel, with its drive letter:
// for processes that can't be opened.
func (c *ExeCache) NTPath(pid uint32) string {
	var raw [1024]uint16
	info := winapi.SystemProcessIDInfo{PID: uintptr(pid), Name: winapi.UnicodeString{MaxLength: uint16(len(raw) * 2), Buffer: &raw[0]}}
	if winapi.NtQuerySystemInformation(winapi.SystemProcessIDInformation, unsafe.Pointer(&info), uint32(unsafe.Sizeof(info)), nil) != 0 {
		return ""
	}
	nt := windows.UTF16ToString(info.Name.Slice())
	if nt == "" {
		return ""
	}
	c.devOnce.Do(c.readDevices)
	lower := strings.ToLower(nt)
	for _, d := range c.devices {
		if len(lower) > len(d.nt) && strings.HasPrefix(lower, d.nt) && lower[len(d.nt)] == '\\' {
			return d.dos + nt[len(d.nt):]
		}
	}
	return ""
}

func (c *ExeCache) readDevices() {
	drives, _ := windows.GetLogicalDrives()
	var target [512]uint16
	for i := range 26 {
		if drives&(1<<i) == 0 {
			continue
		}
		dos := string(rune('A'+i)) + ":"
		n, err := windows.QueryDosDevice(windows.StringToUTF16Ptr(dos), &target[0], uint32(len(target)))
		if err != nil || n == 0 {
			continue
		}
		c.devices = append(c.devices, device{nt: strings.ToLower(windows.UTF16ToString(target[:n])), dos: dos})
	}
}
