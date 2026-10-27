package sys

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"taskmgr/internal/winapi"
)

const (
	runKey      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`
)

type startupSource struct {
	root registry.Key
	// key is the key with the values, or a folder of shortcuts.
	key      string
	folder   bool
	wow      uint32
	approved string
}

// StartupItem is something Windows starts at sign-in.
type StartupItem struct {
	Name string
	// Value is the registry value's name, or the shortcut's file name.
	Value   string
	Command string
	// Path is the program's path, from the command.
	Path      string
	Publisher string
	Enabled   bool
	Exe       *Exe
	source    startupSource
}

// ReadStartup lists what starts at sign-in, from the Run keys (the user's,
// the machine's, the 32-bit machine's) and the Startup folders, each
// enabled or disabled as Explorer's StartupApproved keys say.
func ReadStartup(cache *ExeCache) []StartupItem {
	sources := []startupSource{
		{root: registry.CURRENT_USER, key: runKey, approved: approvedKey + "Run"},
		{root: registry.LOCAL_MACHINE, key: runKey, wow: registry.WOW64_64KEY, approved: approvedKey + "Run"},
		{root: registry.LOCAL_MACHINE, key: runKey, wow: registry.WOW64_32KEY, approved: approvedKey + "Run32"},
		{root: registry.CURRENT_USER, key: filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`), folder: true, approved: approvedKey + "StartupFolder"},
		{root: registry.LOCAL_MACHINE, key: filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs\StartUp`), folder: true, approved: approvedKey + "StartupFolder"},
	}
	var list []StartupItem
	for _, src := range sources {
		if src.folder {
			list = readStartupFolder(list, src, cache)
		} else {
			list = readStartupKey(list, src, cache)
		}
	}
	sort.SliceStable(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	return list
}

func readStartupKey(list []StartupItem, src startupSource, cache *ExeCache) []StartupItem {
	k, err := registry.OpenKey(src.root, src.key, registry.READ|src.wow)
	if err != nil {
		return list
	}
	defer k.Close()
	names, _ := k.ReadValueNames(256)
	for _, name := range names {
		cmd, kind, err := k.GetStringValue(name)
		if err != nil {
			continue
		}
		if kind == registry.EXPAND_SZ {
			cmd = winapi.ExpandEnv(cmd)
		}
		list = appendStartup(list, src, name, cmd, cache)
	}
	return list
}

func readStartupFolder(list []StartupItem, src startupSource, cache *ExeCache) []StartupItem {
	if src.key == "" {
		return list
	}
	entries, err := os.ReadDir(src.key)
	if err != nil {
		return list
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.EqualFold(name, "desktop.ini") {
			continue
		}
		// Not hidden files.
		if info, err := e.Info(); err == nil {
			if sys, ok := info.Sys().(*syscall.Win32FileAttributeData); ok && sys.FileAttributes&syscall.FILE_ATTRIBUTE_HIDDEN != 0 {
				continue
			}
		}
		list = appendStartup(list, src, name, filepath.Join(src.key, name), cache)
	}
	return list
}

func appendStartup(list []StartupItem, src startupSource, value, command string, cache *ExeCache) []StartupItem {
	path := programOf(command)
	var e *Exe
	if path != "" && strings.HasSuffix(strings.ToLower(path), ".exe") && winapi.FileExists(path) {
		e = cache.Get(path)
	}
	name := value
	if src.folder && strings.HasSuffix(strings.ToLower(name), ".lnk") {
		name = name[:len(name)-4]
	}
	if e != nil && e.Description != "" && !src.folder {
		name = e.Description
	}
	item := StartupItem{Name: name, Value: value, Command: command, Path: path, Enabled: startupEnabled(src, value), Exe: e, source: src}
	if e != nil {
		item.Publisher = e.Company
	}
	return append(list, item)
}

// SetEnabled enables or disables the item, as Task Manager does: in the
// StartupApproved key (the machine's need administrator rights).
func (it *StartupItem) SetEnabled(on bool) error {
	k, _, err := registry.CreateKey(it.source.root, it.source.approved, registry.WRITE|registry.WOW64_64KEY)
	if err != nil {
		return err
	}
	defer k.Close()
	data := make([]byte, 12)
	data[0] = 3
	if on {
		data[0] = 2
	} else {
		var ft windows.Filetime
		windows.GetSystemTimeAsFileTime(&ft)
		binary.LittleEndian.PutUint32(data[4:], ft.LowDateTime)
		binary.LittleEndian.PutUint32(data[8:], ft.HighDateTime)
	}
	if err := k.SetBinaryValue(it.Value, data); err != nil {
		return err
	}
	it.Enabled = on
	return nil
}

// startupEnabled reports whether StartupApproved leaves value name enabled
// (its first byte even, or no entry at all).
func startupEnabled(src startupSource, name string) bool {
	k, err := registry.OpenKey(src.root, src.approved, registry.READ|registry.WOW64_64KEY)
	if err != nil {
		return true
	}
	defer k.Close()
	data, _, err := k.GetBinaryValue(name)
	if err != nil || len(data) == 0 {
		return true
	}
	return data[0]&1 == 0
}

// programOf is the program a command line runs: quoted, or up to ".exe".
func programOf(cmd string) string {
	t := strings.TrimSpace(cmd)
	if strings.HasPrefix(t, `"`) {
		if end := strings.IndexByte(t[1:], '"'); end >= 0 {
			return t[1 : 1+end]
		}
		return t[1:]
	}
	if i := strings.Index(strings.ToLower(t), ".exe"); i >= 0 {
		return t[:i+4]
	}
	if sp := strings.IndexByte(t, ' '); sp >= 0 {
		return t[:sp]
	}
	return t
}
