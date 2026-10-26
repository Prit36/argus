package sys

import (
	"time"
	"unicode/utf16"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// Arch is a process's architecture.
type Arch uint8

const (
	ArchUnknown Arch = iota
	ArchX86
	ArchX64
	ArchARM64
)

func (a Arch) String() string {
	switch a {
	case ArchX86:
		return "x86"
	case ArchX64:
		return "x64"
	case ArchARM64:
		return "ARM64"
	}
	return ""
}

// Info is what a process is for its life, kept from sample to sample by
// its pid and start time. Only the UI thread writes it; the resolver's
// findings reach it through Sampler.ApplyResolved.
type Info struct {
	PID        uint32
	CreateTime int64
	// Name is the image's file name ("chrome.exe"), or the kernel's name
	// for it.
	Name string
	Exe  *Exe
	// User is the account it runs as ("SYSTEM", the user).
	User string
	Arch Arch
	// Resolved: path, user and architecture were looked for.
	Resolved bool
	// Cmdline is the command line it was started with ("" if unreadable).
	Cmdline string
	// Elevated: it runs with an administrator's token.
	Elevated bool
	// AppContainer: it runs in an AppContainer (the Isolation column).
	AppContainer bool
	// Efficiency: in efficiency mode (EcoQoS), as last checked.
	Efficiency        bool
	EfficiencyChecked bool

	seen      uint64
	queued    bool
	prevCPU   uint64
	prevRead  uint64
	prevWrite uint64
}

// Title is what Task Manager calls it: the file's description, else its
// name.
func (i *Info) Title() string {
	if i.Exe != nil && i.Exe.Description != "" {
		return i.Exe.Description
	}
	if i.PID == 0 {
		return "System Idle Process"
	}
	return i.Name
}

// Path is the program's file, "" if unknown.
func (i *Info) Path() string {
	if i.Exe != nil {
		return i.Exe.Path
	}
	return ""
}

// Company is the program's publisher, "" if unknown.
func (i *Info) Company() string {
	if i.Exe != nil {
		return i.Exe.Company
	}
	return ""
}

// File is the program's file name, lower-cased if known: what groups
// processes.
func (i *Info) File() string {
	if i.Exe != nil {
		return i.Exe.File
	}
	return i.Name
}

// Proc is a process in one sample.
type Proc struct {
	PID, PPID    uint32
	Session      uint32
	Threads      uint32
	Handles      uint32
	BasePriority int32
	// CPU is its share of the whole machine, 0–100.
	CPU     float32
	CPUTime uint64
	// PrivateWS is Task Manager's "Memory (active private working set)".
	PrivateWS  uint64
	WorkingSet uint64
	Commit     uint64
	// ReadRate and WriteRate are bytes a second (all I/O, as the kernel
	// counts it).
	ReadRate, WriteRate float32
	Suspended           bool
	// Windowed: it has a window on the desktop, one of the "Apps".
	Windowed bool
	// Hung: a window of it doesn't answer.
	Hung bool
	// CPUDelta is the processor time used since the last sample, in 100 ns.
	CPUDelta uint64
	// Net is bytes sent and received a second, and NetDelta since the last
	// sample, when per-process figures can be had.
	Net      float32
	NetDelta uint64
	// GPU is its busiest engine's use, 0–100, and which: the adapter and
	// the engine's kind (GPUKindOther for another).
	GPU        float32
	GPUAdapter uint8
	GPUKind    uint8
	Info       *Info
}

// IO is its disk (all I/O) bytes a second.
func (p *Proc) IO() float32 { return p.ReadRate + p.WriteRate }

type procKey struct {
	pid  uint32
	time int64
}

// Sampler takes samples of every process and processor.
type Sampler struct {
	buf     []uint64 // 8-byte aligned, for the kernel's structures
	Procs   []Proc
	infos   map[procKey]*Info
	Exes    *ExeCache
	resolve *Resolver

	generation uint64
	cpus       []winapi.ProcessorPerformance
	cpuNow     []winapi.ProcessorPerformance
	// CoreUse is each logical processor's use, 0–1.
	CoreUse []float32
	// CPU is the machine's use, 0–1, and CPUKernel its kernel's share.
	CPU, CPUKernel   float32
	prevTime         time.Time
	Threads, Handles uint32

	windows map[uint32]struct{}
	hung    map[uint32]struct{}
	gone    []procKey
	SelfPID uint32
}

// NewSampler makes a sampler whose processes are resolved by r.
func NewSampler(exes *ExeCache, r *Resolver) *Sampler {
	return &Sampler{
		infos:   make(map[procKey]*Info, 512),
		Exes:    exes,
		resolve: r,
		windows: make(map[uint32]struct{}, 64),
		hung:    make(map[uint32]struct{}, 4),
		SelfPID: windows.GetCurrentProcessId(),
	}
}

// query fills the buffer with an information class, growing it as asked.
func (s *Sampler) query(class uint32) ([]byte, bool) {
	if len(s.buf) == 0 {
		s.buf = make([]uint64, 512*1024/8)
	}
	for {
		var need uint32
		size := uint32(len(s.buf) * 8)
		st := winapi.NtQuerySystemInformation(class, unsafe.Pointer(&s.buf[0]), size, &need)
		if st == winapi.StatusInfoLengthMismatch {
			n := max(need+64*1024, size*2)
			s.buf = make([]uint64, (n+7)/8)
			continue
		}
		if st != 0 {
			return nil, false
		}
		return unsafe.Slice((*byte)(unsafe.Pointer(&s.buf[0])), size), true
	}
}

// sampleCPUs reads every processor's times; the processors' total time
// since the last sample in 100 ns (0 the first time).
func (s *Sampler) sampleCPUs() uint64 {
	n := int(windows.GetActiveProcessorCount(windows.ALL_PROCESSOR_GROUPS))
	n = min(n, 256)
	if cap(s.cpuNow) < n {
		s.cpuNow = make([]winapi.ProcessorPerformance, n)
		s.CoreUse = make([]float32, n)
	}
	now := s.cpuNow[:n]
	s.CoreUse = s.CoreUse[:n]
	size := uint32(n) * uint32(unsafe.Sizeof(winapi.ProcessorPerformance{}))
	if winapi.NtQuerySystemInformation(winapi.SystemProcessorPerformanceInformation, unsafe.Pointer(&now[0]), size, nil) != 0 {
		return 0
	}
	first := len(s.cpus) != n
	var total, idle, kernelBusy uint64
	var zero winapi.ProcessorPerformance
	for i := range now {
		c, was := &now[i], &zero
		if !first {
			was = &s.cpus[i]
		}
		// Kernel time includes idle time.
		t := uint64(max(0, (c.Kernel-was.Kernel)+(c.User-was.User)))
		id := min(uint64(max(0, c.Idle-was.Idle)), t)
		k := uint64(max(0, c.Kernel-was.Kernel))
		total += t
		idle += id
		if k > id {
			kernelBusy += k - id
		}
		if t > 0 {
			s.CoreUse[i] = 1 - float32(id)/float32(t)
		} else {
			s.CoreUse[i] = 0
		}
	}
	s.cpus = append(s.cpus[:0], now...)
	if total > 0 {
		s.CPU = 1 - float32(idle)/float32(total)
		s.CPUKernel = float32(kernelBusy) / float32(total)
	}
	if first {
		return 0
	}
	return total
}

// enumTarget is the sampler EnumWindows reports to: the callback is made
// once, and EnumWindows calls it on the caller's thread.
var (
	enumTarget   *Sampler
	enumCallback = windows.NewCallback(eachWindow)
)

// findWindows finds the processes with a window on the desktop (the
// "Apps"), and those whose windows don't answer.
func (s *Sampler) findWindows() {
	clear(s.windows)
	clear(s.hung)
	enumTarget = s
	windows.EnumWindows(enumCallback, nil)
	enumTarget = nil
}

var shellClasses = [...]string{"Progman", "WorkerW", "Shell_TrayWnd", "Shell_SecondaryTrayWnd"}

func eachWindow(h windows.HWND, _ uintptr) uintptr {
	s := enumTarget
	if s == nil || !windows.IsWindowVisible(h) {
		return 1
	}
	if winapi.GetWindow(h, winapi.GwOwner) != 0 {
		return 1
	}
	if int32(winapi.GetWindowLongPtr(h, winapi.GwlExStyle))&winapi.WsExToolWindow != 0 {
		return 1
	}
	// Titled (read without messaging the window: a hung app can't stall us).
	var title [2]uint16
	if winapi.InternalGetWindowText(h, title[:]) == 0 {
		return 1
	}
	// The desktop and the taskbar are the shell's, not an app's.
	var class [32]uint16
	if n := winapi.GetClassName(h, class[:]); n > 0 {
		for _, name := range shellClasses {
			if utf16Equal(class[:n], name) {
				return 1
			}
		}
	}
	// Windows the shell keeps hidden (suspended Store apps' frames).
	if winapi.DwmCloaked(h) {
		return 1
	}
	var pid uint32
	windows.GetWindowThreadProcessId(h, &pid)
	if pid != 0 {
		s.windows[pid] = struct{}{}
		if winapi.IsHungAppWindow(h) {
			s.hung[pid] = struct{}{}
		}
	}
	return 1
}

func utf16Equal(a []uint16, s string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(s) {
		if a[i] < utf8.RuneSelf && s[j] < utf8.RuneSelf {
			if a[i] != uint16(s[j]) {
				return false
			}
			i++
			j++
			continue
		}
		r := rune(a[i])
		i++
		if utf16.IsSurrogate(r) {
			decoded := utf8.RuneError
			if i < len(a) {
				decoded = utf16.DecodeRune(r, rune(a[i]))
				if decoded != utf8.RuneError {
					i++
				}
			}
			r = decoded
		}
		sr, size := utf8.DecodeRuneInString(s[j:])
		j += size
		if r != sr {
			return false
		}
	}
	return i == len(a) && j == len(s)
}

// Sample takes a sample: every process and processor.
func (s *Sampler) Sample(now time.Time) {
	s.generation++
	total := s.sampleCPUs()
	s.findWindows()
	data, ok := s.query(winapi.SystemProcessInformation)
	if !ok {
		return
	}
	oldProcs := s.Procs
	s.Procs = s.Procs[:0]
	s.Threads, s.Handles = 0, 0
	var dt float64
	if !s.prevTime.IsZero() {
		dt = now.Sub(s.prevTime).Seconds()
	}
	s.prevTime = now
	for off := uintptr(0); ; {
		sp := (*winapi.SystemProcessInfo)(unsafe.Pointer(&data[off]))
		s.add(sp, total, dt)
		if sp.NextEntryOffset == 0 {
			break
		}
		off += uintptr(sp.NextEntryOffset)
	}
	s.forget()
	// The backing array is reused, but exited processes' Info (including
	// command lines) must not stay reachable through its unused tail.
	if len(s.Procs) < len(oldProcs) {
		clear(oldProcs[len(s.Procs):])
	}
}

func (s *Sampler) add(sp *winapi.SystemProcessInfo, total uint64, dt float64) {
	pid := uint32(sp.UniqueProcessID)
	key := procKey{pid, sp.CreateTime}
	info, found := s.infos[key]
	if !found {
		info = &Info{PID: pid, CreateTime: sp.CreateTime, Name: windows.UTF16ToString(sp.ImageName.Slice())}
		s.infos[key] = info
	}
	info.seen = s.generation
	// The idle process's time is the processors' idle time.
	cpuTime := uint64(max(0, sp.KernelTime+sp.UserTime))
	read := uint64(max(0, sp.ReadBytes))
	write := uint64(max(0, sp.WriteBytes+sp.OtherBytes))
	_, windowed := s.windows[pid]
	_, hung := s.hung[pid]
	s.Procs = append(s.Procs, Proc{
		PID:          pid,
		PPID:         uint32(sp.InheritedFromPID),
		Session:      sp.SessionID,
		Threads:      sp.NumberOfThreads,
		Handles:      sp.HandleCount,
		BasePriority: sp.BasePriority,
		CPUTime:      cpuTime,
		PrivateWS:    uint64(max(0, sp.PrivateWorkingSet)),
		WorkingSet:   uint64(sp.WorkingSetSize),
		Commit:       uint64(sp.PrivatePageCount),
		Windowed:     windowed,
		Hung:         hung,
		GPUKind:      GPUKindOther,
		Suspended:    suspended(sp),
		Info:         info,
	})
	p := &s.Procs[len(s.Procs)-1]
	if found {
		d := sub(cpuTime, info.prevCPU)
		p.CPUDelta = d
		if total > 0 {
			p.CPU = min(100, 100*float32(d)/float32(total))
		}
		if dt > 0 {
			p.ReadRate = float32(float64(sub(read, info.prevRead)) / dt)
			p.WriteRate = float32(float64(sub(write, info.prevWrite)) / dt)
		}
	}
	info.prevCPU, info.prevRead, info.prevWrite = cpuTime, read, write
	s.Threads += sp.NumberOfThreads
	s.Handles += sp.HandleCount
	if !info.Resolved && !info.queued && s.resolve != nil {
		info.queued = s.resolve.queue(info, sp.SessionID)
	}
}

func sub(a, b uint64) uint64 {
	if a < b {
		return 0
	}
	return a - b
}

// suspended reports whether every thread waits to be resumed.
func suspended(sp *winapi.SystemProcessInfo) bool {
	if sp.NumberOfThreads == 0 || sp.UniqueProcessID == 0 {
		return false
	}
	threads := unsafe.Slice((*winapi.SystemThreadInfo)(unsafe.Add(unsafe.Pointer(sp), unsafe.Sizeof(*sp))), sp.NumberOfThreads)
	for i := range threads {
		if threads[i].State != 5 || threads[i].WaitReason != 5 {
			return false
		}
	}
	return true
}

// forget drops the processes that have exited.
func (s *Sampler) forget() {
	s.gone = s.gone[:0]
	for k, info := range s.infos {
		if info.seen != s.generation {
			s.gone = append(s.gone, k)
		}
	}
	for _, k := range s.gone {
		delete(s.infos, k)
	}
}

// ApplyResolved takes what the resolver found since the last call; whether
// anything was.
func (s *Sampler) ApplyResolved() bool {
	if s.resolve == nil {
		return false
	}
	return s.resolve.drain(s.applyOne)
}

// Pending reports whether processes wait to be resolved.
func (s *Sampler) Pending() bool { return s.resolve != nil && s.resolve.busy() }

// ByPID is the process pid now, if it runs.
func (s *Sampler) ByPID(pid uint32) *Proc {
	for i := range s.Procs {
		if s.Procs[i].PID == pid {
			return &s.Procs[i]
		}
	}
	return nil
}

// Cores is the number of logical processors.
func (s *Sampler) Cores() int { return len(s.CoreUse) }
