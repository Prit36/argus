package app

import (
	"hash/maphash"
	"slices"
	"strings"

	"taskmgr/internal/sys"
)

// The Processes page's rows, as Task Manager groups them: the apps (a
// process with a window, and every process under it), background
// processes, and Windows's own processes; each group a row that opens to
// its processes, with its figures summed. Built again when something
// changed (a sample, the sort, the search, a group opened), not on every
// frame; its slices and maps are reused from build to build.

// Category is a group of the Processes page.
type Category uint8

const (
	CatApps Category = iota
	CatBackground
	CatWindows
)

func (c Category) Title() string {
	switch c {
	case CatApps:
		return "Apps"
	case CatBackground:
		return "Background processes"
	}
	return "Windows processes"
}

// Sort is what a table is sorted by.
type Sort uint8

const (
	SortName Sort = iota
	SortStatus
	SortCPU
	SortMemory
	SortDisk
	SortPID
	SortUser
	SortArch
	SortDescription
	SortThreads
	SortHandles
	SortPublisher
	SortPath
	SortType
	SortProcessName
	SortCommandLine
	SortNetwork
	SortGPU
	SortGPUEngine
	SortSession
	SortCPUTime
	SortWorkingSet
	SortCommit
	SortBasePriority
	SortElevated
	SortIsolation
)

// RowKind is what a row of the Processes page is.
type RowKind uint8

const (
	RowHeader RowKind = iota
	RowGroup
	RowProc
)

// Row is a row of the Processes page.
type Row struct {
	Kind     RowKind
	Category Category
	Depth    uint8
	Expanded bool
	Title    string
	// Start and Len are a group's processes: members[Start:Start+Len].
	Start, Len int32
	// Proc is the process the row stands for (a group's first).
	Proc       int32
	Key        uint64
	CPU        float32
	Memory     uint64
	Disk       float32
	Net        float32
	GPU        float32
	Suspended  bool
	Hung       bool
	Efficiency bool

	// The figures as text, made when the row first shows.
	cells     bool
	cpuText   string
	memText   string
	diskText  string
	netText   string
	gpuText   string
	titleText string
}

type group struct {
	key       uint64
	category  Category
	root      int32
	start     int32
	len       int32
	title     string
	cpu       float32
	memory    uint64
	disk      float32
	net       float32
	gpu       float32
	suspended bool
	hung      bool
	visible   bool
}

// Model is the Processes page's rows.
type Model struct {
	Rows     []Row
	members  []int32
	groups   []group
	titles   []string
	of       []int32
	byPID    map[uint32]int32
	groupAt  map[uint64]int32
	expanded map[uint64]struct{}
	// Counts are the groups shown of each category.
	Counts [3]int
}

func newModel() *Model {
	return &Model{byPID: make(map[uint32]int32, 512), groupAt: make(map[uint64]int32, 256), expanded: map[uint64]struct{}{}}
}

// Toggle opens or closes a group.
func (m *Model) Toggle(key uint64) {
	if _, ok := m.expanded[key]; ok {
		delete(m.expanded, key)
	} else {
		m.expanded[key] = struct{}{}
	}
}

// buildOptions say how the rows are built.
type buildOptions struct {
	sort      Sort
	ascending bool
	byType    bool
	query     string
	// windir is the Windows folder, lower-cased with its trailing slash.
	windir string
	// title names a process (a Service Host after its service).
	title func(p *sys.Proc) string
}

var seed = maphash.MakeSeed()

// mix scrambles a process's identity into a key.
func mix(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	return x ^ x>>31
}

// procKey is a process's key: its pid and start time.
func procKey(p *sys.Proc) uint64 {
	return mix(uint64(p.PID)<<32^uint64(p.Info.CreateTime)) ^ 3
}

func (m *Model) build(list []sys.Proc, o buildOptions) {
	m.Rows = m.Rows[:0]
	m.members = m.members[:0]
	m.groups = m.groups[:0]
	clear(m.groupAt)
	clear(m.byPID)
	m.Counts = [3]int{}
	m.titles = slices.Grow(m.titles[:0], len(list))[:len(list)]
	m.of = slices.Grow(m.of[:0], len(list))[:len(list)]
	for i := range list {
		m.byPID[list[i].PID] = int32(i)
		m.titles[i] = o.title(&list[i])
	}
	// Each process's group.
	for i := range list {
		m.of[i] = -1
		if list[i].PID == 0 {
			continue
		}
		key, cat, root := m.groupOf(list, int32(i), o.windir)
		gi, ok := m.groupAt[key]
		if !ok {
			gi = int32(len(m.groups))
			m.groupAt[key] = gi
			m.groups = append(m.groups, group{key: key, category: cat, root: root, suspended: true})
		}
		m.of[i] = gi
		m.groups[gi].len++
	}
	// Members together, group by group.
	var at int32
	for gi := range m.groups {
		g := &m.groups[gi]
		g.start = at
		at += g.len
		g.len = 0
	}
	m.members = slices.Grow(m.members, int(at))[:at]
	for i, gi := range m.of {
		if gi < 0 {
			continue
		}
		g := &m.groups[gi]
		m.members[g.start+g.len] = int32(i)
		g.len++
	}
	// Figures, the search, and each group's order.
	q := strings.TrimSpace(o.query)
	for gi := range m.groups {
		g := &m.groups[gi]
		mem := m.members[g.start : g.start+g.len]
		g.title = m.titles[g.root]
		keep := 0
		for _, pi := range mem {
			p := &list[pi]
			g.cpu += p.CPU
			g.memory += p.PrivateWS
			g.disk += p.IO()
			g.net += p.Net
			g.gpu = min(100, g.gpu+p.GPU)
			if !p.Suspended {
				g.suspended = false
			}
			if p.Hung {
				g.hung = true
			}
			if q != "" && !m.matches(p, pi, q) {
				continue
			}
			mem[keep] = pi
			keep++
		}
		g.visible = keep > 0
		// Searching: only the processes that match.
		if q != "" {
			g.len = int32(keep)
		}
		// Collapsed groups only show aggregate figures; order their members
		// when they become visible instead of sorting hidden rows every tick.
		_, expanded := m.expanded[g.key]
		if g.visible && (expanded || q != "") {
			m.sortMembers(list, m.members[g.start:g.start+g.len], o, g.category)
		}
		if g.visible {
			m.Counts[g.category]++
		}
	}
	slices.SortFunc(m.groups, func(a, b group) int {
		return m.compare(o, m.groupFigures(list, o, &a), m.groupFigures(list, o, &b))
	})
	searching := q != ""
	if o.byType {
		for cat := CatApps; cat <= CatWindows; cat++ {
			if m.Counts[cat] == 0 {
				continue
			}
			m.Rows = append(m.Rows, Row{Kind: RowHeader, Category: cat, Key: 0x4ead + uint64(cat), Proc: -1})
			for gi := range m.groups {
				if g := &m.groups[gi]; g.category == cat && g.visible {
					m.emit(list, g, searching)
				}
			}
		}
	} else {
		for gi := range m.groups {
			if g := &m.groups[gi]; g.visible {
				m.emit(list, g, searching)
			}
		}
	}
}

func (m *Model) emit(list []sys.Proc, g *group, searching bool) {
	root := &list[g.root]
	_, expanded := m.expanded[g.key]
	open := g.len > 1 && (expanded || searching)
	r := Row{
		Kind:       RowProc,
		Category:   g.category,
		Title:      g.title,
		Start:      g.start,
		Len:        g.len,
		Proc:       g.root,
		Key:        g.key,
		CPU:        g.cpu,
		Memory:     g.memory,
		Disk:       g.disk,
		Net:        g.net,
		GPU:        g.gpu,
		Suspended:  g.suspended,
		Hung:       g.hung,
		Efficiency: root.Info.Efficiency,
		Expanded:   open,
	}
	if g.len > 1 {
		r.Kind = RowGroup
	} else {
		r.Proc = m.members[g.start]
	}
	m.Rows = append(m.Rows, r)
	if !open {
		return
	}
	for _, pi := range m.members[g.start : g.start+g.len] {
		p := &list[pi]
		m.Rows = append(m.Rows, Row{
			Kind:       RowProc,
			Category:   g.category,
			Title:      m.titles[pi],
			Proc:       pi,
			Depth:      1,
			Key:        procKey(p),
			CPU:        p.CPU,
			Memory:     p.PrivateWS,
			Disk:       p.IO(),
			Net:        p.Net,
			GPU:        p.GPU,
			Suspended:  p.Suspended,
			Hung:       p.Hung,
			Efficiency: p.Info.Efficiency,
		})
	}
}

// Members are the processes of a group row.
func (m *Model) Members(r *Row) []int32 {
	if r.Kind != RowGroup {
		return nil
	}
	return m.members[r.Start : r.Start+r.Len]
}

func (m *Model) matches(p *sys.Proc, i int32, q string) bool {
	if containsFold(m.titles[i], q) || containsFold(p.Info.Name, q) || containsFold(p.Info.Company(), q) {
		return true
	}
	return pidHasPrefix(p.PID, q)
}

// groupOf is the group process i belongs to: the app whose window it is
// under, else the oldest of the same program it was started by.
func (m *Model) groupOf(list []sys.Proc, i int32, windir string) (key uint64, cat Category, root int32) {
	cur := i
	for depth := 0; depth < 64; depth++ {
		p := &list[cur]
		if p.Windowed {
			return maphash.String(seed, p.Info.File()) ^ 1, CatApps, cur
		}
		parent, ok := m.parentOf(list, cur)
		if !ok {
			break
		}
		cur = parent
		// What the shell starts is its own, not Explorer's.
		if list[cur].Info.File() == "explorer.exe" {
			break
		}
	}
	windows := isWindows(&list[i], windir)
	cat = CatBackground
	if windows {
		cat = CatWindows
	}
	root = i
	for depth := 0; depth < 64; depth++ {
		parent, ok := m.parentOf(list, root)
		if !ok || list[parent].Info.File() != list[root].Info.File() || isWindows(&list[parent], windir) != windows {
			break
		}
		root = parent
	}
	return mix(uint64(list[root].PID)<<32^uint64(list[root].Info.CreateTime)) ^ 2, cat, root
}

// parentOf is the process that started i, if it still runs (and isn't a
// newer process that took its pid).
func (m *Model) parentOf(list []sys.Proc, i int32) (int32, bool) {
	p := &list[i]
	if p.PPID == p.PID || p.PPID == 0 {
		return 0, false
	}
	pi, ok := m.byPID[p.PPID]
	if !ok || list[pi].Info.CreateTime > p.Info.CreateTime {
		return 0, false
	}
	return pi, true
}

// isWindows: part of Windows, the kernel's own processes and programs in
// the Windows folder.
func isWindows(p *sys.Proc, windir string) bool {
	if p.PID <= 4 {
		return true
	}
	path := p.Info.Path()
	if path == "" {
		return p.Session == 0 && p.Info.Resolved
	}
	return windir != "" && len(path) >= len(windir) && strings.EqualFold(path[:len(windir)], windir)
}

// isolation is what the Isolation column says of a process.
func isolation(p *sys.Proc) string {
	if p.Info.AppContainer {
		return "AppContainer"
	}
	return ""
}

// figures are what a row is sorted by.
type figures struct {
	title, text     string
	cpu             float32
	memory          uint64
	disk, net, gpu  float32
	suspended, hung bool
	pid             uint32
}

// sortText is what a process is sorted by when the column is text.
func sortText(o buildOptions, p *sys.Proc, title string, cat Category) string {
	switch o.sort {
	case SortType:
		return cat.Title()
	case SortPublisher:
		return p.Info.Company()
	case SortProcessName:
		return p.Info.Name
	case SortCommandLine:
		return p.Info.Cmdline
	case SortGPUEngine:
		if p.GPU > 0 {
			return sys.KindTitle(p.GPUKind)
		}
		return ""
	case SortIsolation:
		return isolation(p)
	}
	return title
}

func (m *Model) groupFigures(list []sys.Proc, o buildOptions, g *group) figures {
	p := &list[g.root]
	return figures{g.title, sortText(o, p, g.title, g.category), g.cpu, g.memory, g.disk, g.net, g.gpu, g.suspended, g.hung, p.PID}
}

func (m *Model) procFigures(list []sys.Proc, o buildOptions, i int32, cat Category) figures {
	p := &list[i]
	t := m.titles[i]
	return figures{t, sortText(o, p, t, cat), p.CPU, p.PrivateWS, p.IO(), p.Net, p.GPU, p.Suspended, p.Hung, p.PID}
}

func status(suspended, hung bool) int {
	n := 0
	if suspended {
		n++
	}
	if hung {
		n++
	}
	return n
}

func cmp3[T int | uint32 | uint64 | float32](a, b T) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// compare orders two rows as the sort says; ties by title, then pid.
func (m *Model) compare(o buildOptions, a, b figures) int {
	var c int
	switch o.sort {
	case SortCPU:
		c = cmp3(a.cpu, b.cpu)
	case SortMemory:
		c = cmp3(a.memory, b.memory)
	case SortDisk:
		c = cmp3(a.disk, b.disk)
	case SortNetwork:
		c = cmp3(a.net, b.net)
	case SortGPU:
		c = cmp3(a.gpu, b.gpu)
	case SortStatus:
		c = cmp3(status(a.suspended, a.hung), status(b.suspended, b.hung))
	case SortPID:
		c = cmp3(a.pid, b.pid)
	default:
		c = cmpFold(a.text, b.text)
	}
	if c == 0 {
		if t := cmpFold(a.title, b.title); t != 0 {
			return t
		}
		return cmp3(a.pid, b.pid)
	}
	if !o.ascending {
		return -c
	}
	return c
}

func (m *Model) sortMembers(list []sys.Proc, mem []int32, o buildOptions, cat Category) {
	if len(mem) < 2 {
		return
	}
	slices.SortFunc(mem, func(x, y int32) int {
		return m.compare(o, m.procFigures(list, o, x, cat), m.procFigures(list, o, y, cat))
	})
}
