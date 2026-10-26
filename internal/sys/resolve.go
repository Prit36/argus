package sys

import (
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// Resolver finds what a new process is (its program file, description,
// user, command line, architecture, elevation) on goroutines of its own:
// reading a file's version resource or an account's name can take
// milliseconds, which the window shouldn't wait for. What it finds waits
// in a channel until the UI thread takes it (Sampler.ApplyResolved).
type Resolver struct {
	exes     *ExeCache
	jobs     chan job
	results  chan *resolved
	inflight atomic.Int32
	notified atomic.Bool
	stop     chan struct{}
	stopOnce sync.Once
	// Notify is called (from a worker) when results wait and none were
	// waiting: the app asks its window to take them.
	Notify func()
}

type job struct {
	info    *Info
	pid     uint32
	session uint32
}

type resolved struct {
	info         *Info
	exe          *Exe
	user         string
	arch         Arch
	cmdline      string
	elevated     bool
	appContainer bool
}

// NewResolver starts n workers.
func NewResolver(exes *ExeCache, n int) *Resolver {
	r := &Resolver{exes: exes, jobs: make(chan job, 4096), results: make(chan *resolved, 4096), stop: make(chan struct{})}
	for range n {
		go r.work()
	}
	return r
}

// queue asks for info to be resolved; false if the queue is full (it is
// asked again at the next sample).
func (r *Resolver) queue(info *Info, session uint32) bool {
	select {
	case <-r.stop:
		return false
	default:
	}
	r.inflight.Add(1)
	select {
	case <-r.stop:
		r.inflight.Add(-1)
		return false
	case r.jobs <- job{info: info, pid: info.PID, session: session}:
		return true
	default:
		r.inflight.Add(-1)
		return false
	}
}

func (r *Resolver) busy() bool { return r.inflight.Load() > 0 }

// drain hands every result waiting to apply; whether there were any.
func (r *Resolver) drain(apply func(*resolved)) bool {
	r.notified.Store(false)
	any := false
	for {
		select {
		case res := <-r.results:
			apply(res)
			any = true
		default:
			return any
		}
	}
}

// Wait applies results as they come until none are outstanding or the
// deadline passes: for the first sample, so the first frame has names.
func (s *Sampler) WaitResolved(d time.Duration) {
	r := s.resolve
	if r == nil {
		return
	}
	deadline := time.NewTimer(d)
	defer deadline.Stop()
	for r.busy() {
		select {
		case res := <-r.results:
			s.applyOne(res)
		case <-deadline.C:
			s.ApplyResolved()
			return
		}
	}
	s.ApplyResolved()
}

func (s *Sampler) applyOne(res *resolved) {
	info := res.info
	info.Resolved, info.queued = true, false
	info.Exe, info.User, info.Arch = res.exe, res.user, res.arch
	info.Cmdline, info.Elevated, info.AppContainer = res.cmdline, res.elevated, res.appContainer
}

func (r *Resolver) work() {
	cmd := make([]uint64, 32*1024/8)
	var path [1024]uint16
	for {
		var j job
		select {
		case <-r.stop:
			return
		case j = <-r.jobs:
		}
		res := r.resolve(j, cmd, path[:])
		select {
		case <-r.stop:
			return
		case r.results <- res:
		}
		r.inflight.Add(-1)
		if r.notified.CompareAndSwap(false, true) && r.Notify != nil {
			r.Notify()
		}
	}
}

// Close cancels queued work without waiting for a slow version or account lookup.
func (r *Resolver) Close() {
	if r != nil {
		r.stopOnce.Do(func() { close(r.stop) })
	}
}

func (r *Resolver) resolve(j job, cmd []uint64, buf []uint16) *resolved {
	res := &resolved{info: j.info}
	if j.pid == 0 {
		return res
	}
	var exePath string
	if h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, j.pid); err == nil {
		n := uint32(len(buf))
		if windows.QueryFullProcessImageName(h, 0, &buf[0], &n) == nil {
			exePath = windows.UTF16ToString(buf[:n])
		}
		var pm, native uint16
		if windows.IsWow64Process2(h, &pm, &native) == nil {
			m := pm
			if m == 0 {
				m = native
			}
			switch m {
			case 0x14c:
				res.arch = ArchX86
			case 0x8664:
				res.arch = ArchX64
			case 0xaa64:
				res.arch = ArchARM64
			}
		}
		res.user = r.exes.UserOf(h)
		res.cmdline = commandLine(h, cmd)
		res.elevated, res.appContainer = tokenFlags(h)
		windows.CloseHandle(h)
	}
	// Protected processes: the kernel still says where their image is.
	if exePath == "" {
		exePath = r.exes.NTPath(j.pid)
	}
	if res.user == "" && (j.pid <= 4 || j.session == 0) {
		res.user = "SYSTEM"
	}
	if exePath != "" {
		res.exe = r.exes.Get(exePath)
	}
	return res
}

// commandLine is the command line a process was started with:
// ProcessCommandLineInformation needs only the right to query a little.
func commandLine(h windows.Handle, buf []uint64) string {
	const processCommandLineInformation = 60
	if winapi.NtQueryInformationProcess(h, processCommandLineInformation, unsafe.Pointer(&buf[0]), uint32(len(buf)*8)) != 0 {
		return ""
	}
	us := (*winapi.UnicodeString)(unsafe.Pointer(&buf[0]))
	return windows.UTF16ToString(us.Slice())
}

// tokenFlags reports whether the process runs elevated and in an
// AppContainer.
func tokenFlags(h windows.Handle) (elevated, appContainer bool) {
	var tok windows.Token
	if windows.OpenProcessToken(h, windows.TOKEN_QUERY, &tok) != nil {
		return false, false
	}
	defer tok.Close()
	elevated = tok.IsElevated()
	var v, n uint32
	// TokenIsAppContainer.
	if windows.GetTokenInformation(tok, 29, (*byte)(unsafe.Pointer(&v)), 4, &n) == nil {
		appContainer = v != 0
	}
	return elevated, appContainer
}
