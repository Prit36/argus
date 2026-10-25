package sys

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HistoryEntry is a program's use, summed since History.Since.
type HistoryEntry struct {
	// Key is the program's path lower-cased: what it's kept by.
	Key, Path, Title string
	// CPU is processor time, in 100 ns; Net bytes sent and received.
	CPU, Net uint64
	// App: it had a window, one of the user's apps.
	App bool
	Exe *Exe
}

// History is App history: the processor time and network each program of
// the user's has used, summed sample by sample since a date, and kept
// between runs in history.tsv. (Task Manager reads the System Resource
// Usage Monitor's database, which only administrators can; this counts
// what the program sees itself.)
type History struct {
	path  string
	List  []HistoryEntry
	index map[string]int
	byExe map[*Exe]int
	// Since is when counting started, as a FILETIME.
	Since   uint64
	changed bool
	saved   time.Time
}

// LoadHistory reads the history kept in dir.
func LoadHistory(dir string) *History {
	h := &History{path: filepath.Join(dir, "history.tsv"), index: map[string]int{}, byExe: map[*Exe]int{}}
	h.load()
	if h.Since == 0 {
		h.Since = filetimeNow()
	}
	return h
}

// Add adds a sample's processes of session (the user's).
func (h *History) Add(procs []Proc, session uint32) {
	for i := range procs {
		p := &procs[i]
		if p.Session != session || p.PID <= 4 {
			continue
		}
		if p.CPUDelta == 0 && p.NetDelta == 0 && !p.Windowed {
			continue
		}
		e := p.Info.Exe
		if e == nil {
			continue
		}
		en := h.entry(e)
		en.CPU += p.CPUDelta
		en.Net += p.NetDelta
		if p.Windowed {
			en.App = true
		}
		h.changed = true
	}
}

func (h *History) entry(e *Exe) *HistoryEntry {
	if i, ok := h.byExe[e]; ok {
		return &h.List[i]
	}
	i, ok := h.index[e.Key]
	if !ok {
		title := e.Description
		if title == "" {
			title = filepath.Base(e.Path)
		}
		h.List = append(h.List, HistoryEntry{Key: e.Key, Path: e.Path, Title: title})
		i = len(h.List) - 1
		h.index[e.Key] = i
	}
	h.byExe[e] = i
	h.List[i].Exe = e
	return &h.List[i]
}

// Clear forgets it all and counts from now ("Delete usage history").
func (h *History) Clear() {
	h.List = h.List[:0]
	clear(h.index)
	clear(h.byExe)
	h.Since = filetimeNow()
	h.changed = true
	h.Save()
}

// SaveEvery saves now and then (every minute).
func (h *History) SaveEvery(now time.Time) {
	if !h.changed || now.Sub(h.saved) < time.Minute {
		return
	}
	h.saved = now
	h.Save()
}

// Save writes the file: its text is made here, and written on a goroutine
// of its own, so a slow disk can't hold up the window.
func (h *History) Save() {
	h.changed = false
	data, path := h.encode(), h.path
	go writeFile(path, data)
}

// SaveNow writes the file before returning: when the program ends.
func (h *History) SaveNow() {
	h.changed = false
	writeFile(h.path, h.encode())
}

func writeFile(path string, data []byte) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
}

func (h *History) encode() []byte {
	var b bytes.Buffer
	b.WriteString("since\t")
	b.WriteString(strconv.FormatUint(h.Since, 10))
	b.WriteString("\r\n")
	for _, e := range h.List {
		b.WriteString(strconv.FormatUint(e.CPU, 10))
		b.WriteByte('\t')
		b.WriteString(strconv.FormatUint(e.Net, 10))
		if e.App {
			b.WriteString("\t1\t")
		} else {
			b.WriteString("\t0\t")
		}
		b.WriteString(e.Path)
		b.WriteByte('\t')
		b.WriteString(e.Title)
		b.WriteString("\r\n")
	}
	return b.Bytes()
}

func (h *History) load() {
	data, err := os.ReadFile(h.path)
	if err != nil {
		return
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		cols := strings.Split(strings.TrimRight(sc.Text(), "\r"), "\t")
		if len(cols) >= 2 && cols[0] == "since" {
			h.Since, _ = strconv.ParseUint(cols[1], 10, 64)
			continue
		}
		if len(cols) < 5 {
			continue
		}
		cpu, err1 := strconv.ParseUint(cols[0], 10, 64)
		net, err2 := strconv.ParseUint(cols[1], 10, 64)
		if err1 != nil || err2 != nil || cols[3] == "" || len(cols[3]) > 1024 {
			continue
		}
		key := strings.ToLower(cols[3])
		if _, ok := h.index[key]; ok {
			continue
		}
		h.List = append(h.List, HistoryEntry{Key: key, Path: cols[3], Title: cols[4], CPU: cpu, Net: net, App: cols[2] == "1"})
		h.index[key] = len(h.List) - 1
	}
}

func filetimeNow() uint64 {
	// 100 ns intervals since 1601-01-01.
	return uint64(time.Now().UnixNano()/100) + 116444736000000000
}

// FiletimeDate is a FILETIME as the date Task Manager writes ("10/25/2026").
func FiletimeDate(ft uint64) string {
	t := time.Unix(0, int64(ft-116444736000000000)*100).UTC()
	return t.Format("1/2/2006")
}
