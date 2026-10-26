package sys

import (
	"bufio"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	// Since is when counting started, as a FILETIME.
	Since   uint64
	changed bool
	saved   time.Time
	writes  historyWriter
}

// LoadHistory reads the history kept in dir.
func LoadHistory(dir string) *History {
	h := &History{path: filepath.Join(dir, "history.tsv"), index: map[string]int{}}
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
	h.List[i].Exe = e
	return &h.List[i]
}

// Clear forgets it all and counts from now ("Delete usage history").
func (h *History) Clear() {
	clear(h.List)
	h.List = h.List[:0]
	clear(h.index)
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
	h.writes.save(h.path, h.encode())
}

// SaveNow writes the file before returning: when the program ends.
func (h *History) SaveNow() {
	h.changed = false
	<-h.writes.save(h.path, h.encode())
}

// Keep one disk write in flight and at most one pending snapshot. A newer
// save replaces the pending data; the final save waits behind older writes
// so they cannot overwrite it during shutdown or after clearing history.
type historyWriter struct {
	mu      sync.Mutex
	pending []byte
	done    chan struct{}
}

func (w *historyWriter) save(path string, data []byte) <-chan struct{} {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending = data
	if w.done == nil {
		w.done = make(chan struct{})
		go w.run(path, writeFile)
	}
	return w.done
}

func (w *historyWriter) run(path string, write func(string, []byte)) {
	for {
		w.mu.Lock()
		data := w.pending
		w.pending = nil
		if data == nil {
			close(w.done)
			w.done = nil
			w.mu.Unlock()
			return
		}
		w.mu.Unlock()
		write(path, data)
	}
}

func writeFile(path string, data []byte) {
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
}

func (h *History) encode() []byte {
	// Reserve for both 20-digit counters, separators and the existing text.
	// Append numbers directly instead of allocating strings per entry.
	size := 28
	for i := range h.List {
		e := &h.List[i]
		size += len(e.Path) + len(e.Title) + 47
	}
	b := make([]byte, 0, size)
	b = append(b, "since\t"...)
	b = strconv.AppendUint(b, h.Since, 10)
	b = append(b, '\r', '\n')
	for _, e := range h.List {
		b = strconv.AppendUint(b, e.CPU, 10)
		b = append(b, '\t')
		b = strconv.AppendUint(b, e.Net, 10)
		if e.App {
			b = append(b, '\t', '1', '\t')
		} else {
			b = append(b, '\t', '0', '\t')
		}
		b = append(b, e.Path...)
		b = append(b, '\t')
		b = append(b, e.Title...)
		b = append(b, '\r', '\n')
	}
	return b
}

func (h *History) load() {
	f, err := os.Open(h.path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		first, rest, ok := strings.Cut(strings.TrimRight(sc.Text(), "\r"), "\t")
		if !ok {
			continue
		}
		if first == "since" {
			value, _, _ := strings.Cut(rest, "\t")
			h.Since, _ = strconv.ParseUint(value, 10, 64)
			continue
		}
		netText, rest, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		app, rest, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		path, title, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		title, _, _ = strings.Cut(title, "\t")
		cpu, err1 := strconv.ParseUint(first, 10, 64)
		net, err2 := strconv.ParseUint(netText, 10, 64)
		if err1 != nil || err2 != nil || path == "" || len(path) > 1024 {
			continue
		}
		key := strings.ToLower(path)
		if _, ok := h.index[key]; ok {
			continue
		}
		h.List = append(h.List, HistoryEntry{Key: key, Path: path, Title: title, CPU: cpu, Net: net, App: app == "1"})
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
