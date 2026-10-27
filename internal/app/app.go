// Package app is the task manager: its state (what was sampled, the last
// minute of each figure, the page shown, how its tables are sorted, what
// is selected and what dialog is open), the sampling on its timer, the
// commands, and the view mygo draws from it.
//
// Every field of App belongs to the UI thread. Work that may block (a
// program's version resource, an account's name, the services, PDH's
// counters, icons) runs on goroutines that hand their results back
// through Window.Update.
package app

import (
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"taskmgr/internal/sys"
)

// Page is a page of the window. The numbers are kept in settings.ini.
type Page uint8

const (
	PageProcesses Page = iota
	PagePerformance
	PageStartup
	PageUsers
	PageDetails
	PageServices
	PageSettings
	// Last, so the numbers kept in settings.ini keep their pages.
	PageAppHistory
)

func (p Page) Title() string {
	switch p {
	case PageProcesses:
		return "Processes"
	case PagePerformance:
		return "Performance"
	case PageStartup:
		return "Startup apps"
	case PageUsers:
		return "Users"
	case PageDetails:
		return "Details"
	case PageServices:
		return "Services"
	case PageSettings:
		return "Settings"
	}
	return "App history"
}

// navOrder is the pages in the order the navigation lists them (Ctrl+1–8,
// Ctrl+Tab).
var navOrder = [...]Page{PageProcesses, PagePerformance, PageAppHistory, PageStartup, PageUsers, PageDetails, PageServices, PageSettings}

// Speed is the update speed.
type Speed uint8

const (
	SpeedHigh Speed = iota
	SpeedNormal
	SpeedLow
	SpeedPaused
)

func (s Speed) Interval() time.Duration {
	switch s {
	case SpeedHigh:
		return 500 * time.Millisecond
	case SpeedNormal:
		return time.Second
	case SpeedLow:
		return 4 * time.Second
	}
	return 0
}

func (s Speed) Title() string {
	switch s {
	case SpeedHigh:
		return "High"
	case SpeedNormal:
		return "Normal"
	case SpeedLow:
		return "Low"
	}
	return "Paused"
}

// ThemeMode is the app's theme setting.
type ThemeMode uint8

const (
	ThemeSystem ThemeMode = iota
	ThemeLight
	ThemeDark
)

// PerfKind is a part of the machine the Performance page shows.
type PerfKind uint8

const (
	PerfCPU PerfKind = iota
	PerfMemory
	PerfDisk
	PerfNet
	PerfGPU
)

// PerfTab is the part chosen: its kind, and which disk, adapter or GPU.
type PerfTab struct {
	Kind  PerfKind
	Index int
}

// App is the task manager's state.
type App struct {
	hardware      *hardwareLoad
	hardwareReady bool
	win           *mygo.Window
	// The window as goroutines see it, and what they posted before it
	// was made (the first sample runs while mygo starts).
	posted  atomic.Pointer[mygo.Window]
	earlyMu sync.Mutex
	early   []func()
	hwnd    windows.HWND
	tray    *mygo.Tray
	closed  bool

	exes     *sys.ExeCache
	sampler  *sys.Sampler
	resolver *sys.Resolver
	cpuInfo  sys.CPU
	memHW    sys.MemoryHardware
	mem      sys.Memory
	disks    *sys.Disks
	net      *sys.Net
	gpus     []*sys.GPU
	perf     *sys.PerfWorker
	// gpuWarm: the GPU counters were collected twice, so their rates
	// mean something; cpuPerf is the processor's performance against its
	// base speed, in percent.
	gpuWarm   bool
	cpuPerf   float32
	gpuPerPID map[uint32]sys.ProcGPU

	services      *sys.Services
	serviceReader sys.ServiceReader
	serviceGroups sys.ServiceGroups
	servicesAt    time.Time
	servicesBusy  bool
	startup       []sys.StartupItem
	startupRead   bool
	startupBusy   bool
	sessions      []sys.Session
	sessionsAt    time.Time
	sessionsBusy  bool
	netmon        sys.NetMonitor
	netBytes      map[uint32]uint64
	history       *sys.History
	// session is the session this program runs in: the user's.
	session uint32

	histCPU, histKernel, histMem sys.Ring
	cores                        []sys.Ring
	model                        *Model
	// dirty: the rows are to be built again before they're drawn.
	dirty       bool
	samples     uint64
	lastSample  time.Time
	resumeSpeed Speed
	timerGen    uint64
	timerOn     bool
	timer       *time.Timer
	warmTimer   *time.Timer
	// windir is the Windows folder, lower-cased with its trailing slash.
	windir string

	set         Settings
	k           colors
	themeAccent accent
	uiTheme     *ui.Theme
	listTheme   *ui.Theme
	page        Page
	perfSummary bool
	darkSystem  bool
	accent      accent
	active      bool
	maximized   bool

	// Processes.
	sort      Sort
	ascending bool
	selected  uint64
	selIdx    int
	procList  ui.ListState
	query     string
	lastQuery string
	queries   [8]string
	rowsPage  Page
	search    ui.Handle
	revealSel bool

	// Details.
	detailsSort Sort
	detailsAsc  bool
	details     []int32
	detailsSel  uint64
	detailsIdx  int
	detailsList ui.ListState
	revealDet   bool

	// Services.
	servicesPID  uint32
	servicesSort uint8
	servicesAsc  bool
	servicesRows []int32
	serviceSel   string
	serviceIdx   int
	servicesList ui.ListState
	revealSvc    bool

	// Startup, users, app history.
	startupSort  uint8
	startupAsc   bool
	startupOrder []int32
	startupSel   int
	startupIdx   int
	startupList  ui.ListState
	userSel      uint32
	hasUserSel   bool
	userProcSel  uint64
	usersOpen    map[uint32]bool
	userRows     []int32
	userOrder    []int32
	userDisplay  []int32 // negative: -1-session index; otherwise process index
	usersList    ui.ListState
	userSums     map[uint32]userSum
	historySort  uint8
	historyAsc   bool
	historyRows  []int32
	historyList  ui.ListState

	perfTab PerfTab

	// Dialogs.
	runOpen           bool
	runText           string
	runAdmin          bool
	runFocus          bool
	confirmEnd        bool
	confirmPIDs       []uint32
	confirmTitle      string
	confirmEfficiency bool
	confirmUser       userAction
	historyConfirm    bool
	affinityOpen      bool
	affinityPID       uint32
	affinityMask      uint64
	toasts            []ui.Toast

	icons     iconCache
	scale     float32
	dumpState atomic.Uint32
	dumpWhere string
	trayKey   int
	trayText  string
	pendingUI atomic.Bool
}

type userAction uint8

const (
	userNone userAction = iota
	userDisconnect
	userSignOut
)

func newApp() *App {
	a := newAppState()
	a.applyHardware(readHardware())
	return a
}

func newAppState() *App {
	a := &App{
		exes:          sys.NewExeCache(),
		disks:         &sys.Disks{},
		net:           &sys.Net{},
		serviceGroups: sys.ServiceGroups{},
		netBytes:      map[uint32]uint64{},
		model:         newModel(),
		usersOpen:     map[uint32]bool{},
		dirty:         true,
		ascending:     true,
		detailsAsc:    true,
		servicesAsc:   true,
		startupAsc:    true,
		historyAsc:    true,
		selIdx:        -1,
		detailsIdx:    -1,
		serviceIdx:    -1,
		startupSel:    -1,
		startupIdx:    -1,
		active:        true,
		scale:         1,
		trayKey:       -1,
	}
	a.resolver = sys.NewResolver(a.exes, 4)
	a.resolver.Notify = a.resolvedNotify
	a.sampler = sys.NewSampler(a.exes, a.resolver)
	a.history = sys.LoadHistory(dataDir())
	if dir, err := windows.GetSystemWindowsDirectory(); err == nil {
		a.windir = strings.ToLower(dir)
		if !strings.HasSuffix(a.windir, `\`) {
			a.windir += `\`
		}
	}
	a.set = loadSettings()
	if a.set.StartPage == PageSettings || a.set.StartPage > PageAppHistory {
		a.set.StartPage = PageProcesses
	}
	if a.set.Speed > SpeedPaused {
		a.set.Speed = SpeedNormal
	}
	a.page = a.set.StartPage
	a.sort, a.ascending = a.set.ProcSort, a.set.ProcAscending
	a.detailsSort, a.detailsAsc = a.set.DetailsSort, a.set.DetailsAscending
	a.servicesSort, a.servicesAsc = a.set.ServicesSort, a.set.ServicesAscending
	a.startupSort, a.startupAsc = a.set.StartupSort, a.set.StartupAscending
	a.historySort, a.historyAsc = a.set.HistorySort, a.set.HistoryAscending
	a.resumeSpeed = SpeedNormal
	if a.set.Speed != SpeedPaused {
		a.resumeSpeed = a.set.Speed
	}
	windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &a.session)
	a.icons.init(a.resolvedNotify)
	return a
}

// post runs fn on the UI thread and draws a frame. Safe from any goroutine.
func (a *App) post(fn func()) {
	w := a.posted.Load()
	if w == nil {
		a.earlyMu.Lock()
		if w = a.posted.Load(); w == nil {
			a.early = append(a.early, fn)
			a.earlyMu.Unlock()
			return
		}
		a.earlyMu.Unlock()
	}
	w.Update(func() {
		if !a.closed {
			fn()
		}
	})
}

// opened makes win the window posts go to, and posts to it what came
// before it was made.
func (a *App) opened(win *mygo.Window) {
	a.earlyMu.Lock()
	a.posted.Store(win)
	early := a.early
	a.early = nil
	a.earlyMu.Unlock()
	for _, fn := range early {
		a.post(fn)
	}
}

// resolvedNotify coalesces notifications from resolver and icon workers.
func (a *App) resolvedNotify() {
	// Names and shell icons often finish in a burst. Publish at most once
	// per display interval instead of rebuilding the table per result.
	if !a.pendingUI.CompareAndSwap(false, true) {
		return
	}
	time.AfterFunc(time.Second/60, func() {
		a.post(func() {
			a.pendingUI.Store(false)
			if a.sampler.ApplyResolved() {
				a.dirty = true
			}
		})
	})
}

// shown reports whether the window is on screen: worth drawing.
func (a *App) shown() bool {
	return a.win != nil && a.win.IsVisible() && !a.win.IsMinimized()
}

// sample takes a sample of everything the pages show.
func (a *App) sample() {
	now := time.Now()
	dt := 0.0
	if !a.lastSample.IsZero() {
		dt = now.Sub(a.lastSample).Seconds()
	}
	a.sampler.Sample(now)
	a.sampler.ApplyResolved()
	a.mergeNetwork(dt)
	a.samples++
	a.lastSample = now
	a.mem = sys.ReadMemory()
	if a.samples > 1 {
		a.histCPU.Push(a.sampler.CPU * 100)
		a.histKernel.Push(a.sampler.CPUKernel * 100)
		if n := a.sampler.Cores(); len(a.cores) != n {
			a.cores = slices.Grow(a.cores[:0], n)[:n]
		}
		for i, u := range a.sampler.CoreUse {
			a.cores[i].Push(u * 100)
		}
	}
	a.histMem.Push(float32(a.mem.Used()))
	a.disks.Sample()
	a.net.Sample(now)
	a.syncPerformance(true)
	a.mergeGPU()
	if a.win != nil {
		a.history.Add(a.sampler.Procs, a.session)
		a.history.SaveEvery(now)
	}
	// Services name the Service Host processes; read them now and then.
	every := 15 * time.Second
	if a.page == PageServices {
		every = 2 * time.Second
	}
	a.refreshServices(every)
	if a.page == PageUsers {
		a.refreshSessions(5 * time.Second)
	}
	a.checkEfficiency()
	// Startup and service rows depend on their own snapshots, not process
	// counters. Their loaders invalidate them when those snapshots change.
	switch a.page {
	case PageProcesses, PageDetails, PageUsers, PageAppHistory:
		a.dirty = true
	}
}

// needGPU reports whether the GPU counters are wanted: the Performance
// page, or a GPU column on the Processes page.
func (a *App) needGPU() bool {
	switch a.page {
	case PagePerformance:
		return true
	case PageProcesses:
		return shown(a.set.ProcColumns, pcGPU) || shown(a.set.ProcColumns, pcGPUEngine)
	}
	return false
}

// syncPerformance turns the PDH counters on while they show, off when not;
// with sample, it asks for a fresh collection.
func (a *App) syncPerformance(sample bool) {
	if a.closed {
		return
	}
	if a.hardware != nil && !a.hardwareReady {
		return
	}
	enabled := a.needGPU() && a.shown()
	if !enabled {
		a.gpuWarm, a.cpuPerf = false, 0
		clear(a.gpuPerPID)
		if a.perf != nil {
			a.perf.RequestGPU(false, false)
		}
		return
	}
	if a.perf == nil {
		a.perf = sys.NewPerfWorker(a.gpus, a.deliverPerformance)
	}
	a.perf.RequestGPU(true, sample)
}

// deliverPerformance runs on the worker's goroutine.
func (a *App) deliverPerformance(r *sys.GPUReading, addrs []sys.AdapterAddress) {
	a.post(func() {
		if r != nil {
			a.takeGPU(r)
		}
		for _, ad := range addrs {
			for _, n := range a.net.List {
				if n.LUID == ad.LUID {
					n.IPv4, n.IPv6 = ad.IPv4, ad.IPv6
				}
			}
		}
		a.dirty = true
	})
}

func (a *App) takeGPU(r *sys.GPUReading) {
	if !a.needGPU() {
		return
	}
	a.gpuWarm, a.cpuPerf, a.gpuPerPID = r.Warm, r.CPUPerf, r.PerPID
	for i, f := range r.GPUs {
		if i >= len(a.gpus) {
			break
		}
		g := a.gpus[i]
		g.Use, g.Engines, g.Dedicated, g.Shared = f.Use, f.Engines, f.Dedicated, f.Shared
		if r.Warm {
			g.Hist.Push(g.Use)
			for k := range g.HistEngines {
				g.HistEngines[k].Push(g.Engines[k])
			}
			g.HistDedicated.Push(float32(g.Dedicated))
			g.HistShared.Push(float32(g.Shared))
		}
	}
	a.mergeGPU()
}

// mergeGPU puts each process's GPU use on its row.
func (a *App) mergeGPU() {
	if !a.needGPU() {
		return
	}
	for i := range a.sampler.Procs {
		p := &a.sampler.Procs[i]
		if g, ok := a.gpuPerPID[p.PID]; ok {
			p.GPU, p.GPUAdapter, p.GPUKind = g.Use, g.Adapter, g.Kind
		} else {
			p.GPU, p.GPUAdapter, p.GPUKind = 0, 0, sys.GPUKindOther
		}
	}
}

func (a *App) requestAddresses() {
	if a.hardware != nil && !a.hardwareReady {
		return
	}
	if a.perf == nil {
		a.perf = sys.NewPerfWorker(a.gpus, a.deliverPerformance)
	}
	a.perf.RequestAddresses()
}

// mergeNetwork puts each process's network use since the last sample on
// its row, from the available network source.
func (a *App) mergeNetwork(dt float64) {
	if !a.netmon.Available {
		return
	}
	a.netmon.SampleEnergy(a.sampler.Procs, dt)
	if a.netmon.UsesEnergy() {
		return
	}
	clear(a.netBytes)
	a.netmon.Take(func(pid uint32, bytes uint64) { a.netBytes[pid] += bytes })
	for i := range a.sampler.Procs {
		p := &a.sampler.Procs[i]
		p.NetKnown = true
		b, ok := a.netBytes[p.PID]
		if !ok {
			continue
		}
		p.NetDelta = b
		if dt > 0 {
			p.Net = float32(float64(b) / dt)
		}
	}
}

// checkEfficiency finds which processes are in efficiency mode: only
// those at idle priority can be, so only they are asked.
func (a *App) checkEfficiency() {
	for i := range a.sampler.Procs {
		p := &a.sampler.Procs[i]
		info := p.Info
		if p.BasePriority != 4 {
			info.Efficiency, info.EfficiencyChecked = false, false
			continue
		}
		if info.EfficiencyChecked {
			continue
		}
		info.EfficiencyChecked = true
		info.Efficiency = efficiencyOn(p.PID)
	}
}

// refreshServices reads the services again, on a goroutine, if every has
// gone by.
func (a *App) refreshServices(every time.Duration) {
	if a.servicesBusy || time.Since(a.servicesAt) < every {
		return
	}
	a.servicesBusy = true
	groups := a.serviceGroups
	go func() {
		list := a.serviceReader.Read(groups)
		a.post(func() {
			a.servicesBusy = false
			a.servicesAt = time.Now()
			if list != nil {
				if list == a.services {
					return // Same immutable snapshot; cached rows remain valid.
				}
				a.services = list
			}
			a.dirty = true
		})
	}()
}

// refreshSessions reads the signed-in users again, on a goroutine.
func (a *App) refreshSessions(every time.Duration) {
	if a.sessionsBusy || time.Since(a.sessionsAt) < every {
		return
	}
	a.sessionsBusy = true
	go func() {
		list := sys.ReadSessions()
		a.post(func() {
			a.sessionsBusy = false
			a.sessionsAt = time.Now()
			a.sessions = list
			a.dirty = true
		})
	}()
}

// readStartup reads the startup items, on a goroutine.
func (a *App) readStartup() {
	if a.startupBusy {
		return
	}
	a.startupBusy = true
	go func() {
		list := sys.ReadStartup(a.exes)
		a.post(func() {
			a.startupBusy, a.startupRead = false, true
			selected := a.startupItem()
			a.startup = list
			a.startupSel = -1
			if selected != nil {
				for i := range list {
					if list[i].Name == selected.Name && list[i].Value == selected.Value && list[i].Command == selected.Command {
						a.startupSel = i
						break
					}
				}
			}
			a.dirty = true
		})
	}()
}

// startTimer samples on the update speed's timer.
func (a *App) startTimer() {
	d := a.set.Speed.Interval()
	// Keep tray and history useful while avoiding high-frequency work for
	// a window that cannot be seen. Rates still use actual elapsed time.
	if a.win != nil && !a.shown() && d > 0 {
		d = max(d, 4*time.Second)
	}
	if d == 0 || a.timerOn {
		return
	}
	a.timerOn = true
	a.timerGen++
	gen := a.timerGen
	a.timer = time.AfterFunc(d, func() { a.post(func() { a.onTimer(gen) }) })
}

func (a *App) stopTimer() {
	if a.timer != nil {
		a.timer.Stop()
		a.timer = nil
	}
	a.timerOn = false
	a.timerGen++
}

func (a *App) onTimer(gen uint64) {
	if gen != a.timerGen {
		return
	}
	a.timerOn = false
	a.timer = nil
	if a.set.Speed == SpeedPaused {
		return
	}
	a.sample()
	a.startTimer()
	a.updateTray()
}

func (a *App) setSpeed(s Speed) {
	if s == a.set.Speed {
		return
	}
	a.set.Speed = s
	if s != SpeedPaused {
		a.resumeSpeed = s
	}
	// The next tick comes at the new speed.
	a.stopTimer()
	a.startTimer()
	a.saveSettings()
}

func (a *App) saveSettings() {
	a.set.ProcSort, a.set.ProcAscending = a.sort, a.ascending
	a.set.DetailsSort, a.set.DetailsAscending = a.detailsSort, a.detailsAsc
	a.set.ServicesSort, a.set.ServicesAscending = a.servicesSort, a.servicesAsc
	a.set.StartupSort, a.set.StartupAscending = a.startupSort, a.startupAsc
	a.set.HistorySort, a.set.HistoryAscending = a.historySort, a.historyAsc
	a.set.save()
}

func (a *App) togglePaused() {
	if a.set.Speed == SpeedPaused {
		speed := a.resumeSpeed
		if speed >= SpeedPaused {
			speed = SpeedNormal
		}
		a.setSpeed(speed)
	} else {
		a.setSpeed(SpeedPaused)
	}
}

// refreshNow also reloads the current page's less frequently sampled data.
func (a *App) refreshNow() {
	a.sample()
	switch a.page {
	case PageServices:
		a.refreshServices(0)
	case PageStartup:
		a.readStartup()
	case PageUsers:
		a.refreshSessions(0)
	case PagePerformance:
		if a.perfTab.Kind == PerfNet {
			a.requestAddresses()
		}
	}
	a.updateTray()
}

// procTitle is a process's title on the Processes page: its description,
// and for a Service Host, the service it runs.
func (a *App) procTitle(p *sys.Proc) string {
	t := p.Info.Title()
	if !strings.EqualFold(p.Info.Name, "svchost.exe") {
		return t
	}
	sv := a.services.FirstIn(p.PID)
	if sv == nil {
		return t
	}
	if n := a.services.CountIn(p.PID); n > 1 {
		return "Service Host: " + sv.Display + " (+" + itoa(n-1) + ")"
	}
	return "Service Host: " + sv.Display
}

// refreshRows builds the page's rows again if something changed.
func (a *App) refreshRows() {
	if a.query != a.lastQuery || a.rowsPage != a.page {
		if a.query != a.lastQuery {
			var list *ui.ListState
			switch a.page {
			case PageProcesses:
				list = &a.procList
			case PageDetails:
				list = &a.detailsList
			case PageServices:
				list = &a.servicesList
			case PageStartup:
				list = &a.startupList
			case PageUsers:
				list = &a.usersList
			case PageAppHistory:
				list = &a.historyList
			}
			if list != nil {
				list.ScrollTo(0, ui.Start)
			}
		}
		a.lastQuery = a.query
		a.dirty = true
	}
	if !a.dirty {
		return
	}
	a.dirty = false
	a.rowsPage = a.page
	switch a.page {
	case PageProcesses:
		a.model.build(a.sampler.Procs, buildOptions{
			sort:      a.sort,
			ascending: a.ascending,
			byType:    a.set.GroupByType,
			query:     a.query,
			windir:    a.windir,
			title:     a.procTitle,
		})
	case PageDetails:
		a.buildDetails()
	case PageServices:
		a.buildServices()
	case PageStartup:
		a.buildStartup()
	case PageAppHistory:
		a.buildHistory()
	case PageUsers:
		a.buildUsers()
	}
}

func (a *App) sortBy(key Sort) {
	// A column click requests a global ranking, across all categories.
	a.set.GroupByType = false
	if a.sort == key {
		a.ascending = !a.ascending
	} else {
		a.sort = key
		// Names A to Z; figures largest first.
		switch key {
		case SortCPU, SortMemory, SortDisk, SortNetwork, SortGPU:
			a.ascending = false
		default:
			a.ascending = true
		}
	}
	a.dirty = true
	a.saveSettings()
}

func (a *App) detailsSortBy(key Sort) {
	if a.detailsSort == key {
		a.detailsAsc = !a.detailsAsc
	} else {
		a.detailsSort = key
		switch key {
		case SortCPU, SortMemory, SortDisk, SortThreads, SortHandles, SortCPUTime, SortWorkingSet, SortCommit, SortBasePriority:
			a.detailsAsc = false
		default:
			a.detailsAsc = true
		}
	}
	a.dirty = true
	a.saveSettings()
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func (a *App) buildDetails() {
	list := a.sampler.Procs
	q := strings.TrimSpace(a.query)
	a.details = a.details[:0]
	for i := range list {
		if q != "" && !detailMatches(&list[i], q) {
			continue
		}
		a.details = append(a.details, int32(i))
	}
	by, asc := a.detailsSort, a.detailsAsc
	slices.SortFunc(a.details, func(x, y int32) int {
		p, q := &list[x], &list[y]
		var c int
		switch by {
		case SortPID:
			c = cmp3(p.PID, q.PID)
		case SortCPU:
			c = cmp3(p.CPU, q.CPU)
		case SortCPUTime:
			c = cmp3(p.CPUTime, q.CPUTime)
		case SortSession:
			c = cmp3(p.Session, q.Session)
		case SortWorkingSet:
			c = cmp3(p.WorkingSet, q.WorkingSet)
		case SortCommit:
			c = cmp3(p.Commit, q.Commit)
		case SortBasePriority:
			c = cmp3(int(p.BasePriority), int(q.BasePriority))
		case SortElevated:
			c = cmp3(b2i(p.Info.Elevated), b2i(q.Info.Elevated))
		case SortIsolation:
			c = cmp3(b2i(p.Info.AppContainer), b2i(q.Info.AppContainer))
		case SortPath:
			c = cmpFold(p.Info.Path(), q.Info.Path())
		case SortCommandLine:
			c = cmpFold(p.Info.Cmdline, q.Info.Cmdline)
		case SortMemory:
			c = cmp3(p.PrivateWS, q.PrivateWS)
		case SortDisk:
			c = cmp3(p.IO(), q.IO())
		case SortThreads:
			c = cmp3(p.Threads, q.Threads)
		case SortHandles:
			c = cmp3(p.Handles, q.Handles)
		case SortStatus:
			c = cmp3(status(p.Suspended, p.Hung), status(q.Suspended, q.Hung))
		case SortUser:
			c = cmpFold(p.Info.User, q.Info.User)
		case SortArch:
			c = cmp3(int(p.Info.Arch), int(q.Info.Arch))
		case SortDescription:
			c = cmpFold(p.Info.Title(), q.Info.Title())
		default:
			c = cmpFold(p.Info.Name, q.Info.Name)
		}
		if c == 0 {
			return cmp3(p.PID, q.PID)
		}
		if !asc {
			return -c
		}
		return c
	})
}

func detailMatches(p *sys.Proc, q string) bool {
	return containsFold(p.Info.Name, q) || containsFold(p.Info.Title(), q) || containsFold(p.Info.User, q) ||
		containsFold(p.Info.Company(), q) || containsFold(p.Info.Path(), q) || containsFold(p.Info.Cmdline, q) || pidHasPrefix(p.PID, q)
}

func (a *App) servicesSortBy(col uint8) {
	if a.servicesSort == col {
		a.servicesAsc = !a.servicesAsc
	} else {
		a.servicesSort, a.servicesAsc = col, true
	}
	a.dirty = true
	a.saveSettings()
}

func (a *App) buildServices() {
	a.servicesRows = a.servicesRows[:0]
	if a.services == nil {
		return
	}
	list := a.services.List
	q := strings.TrimSpace(a.query)
	for i := range list {
		if q != "" && !containsFold(list[i].Name, q) && !containsFold(list[i].Display, q) &&
			!containsFold(list[i].Group, q) && !containsFold(list[i].State.String(), q) && !pidHasPrefix(list[i].PID, q) {
			continue
		}
		a.servicesRows = append(a.servicesRows, int32(i))
	}
	by, asc := a.servicesSort, a.servicesAsc
	slices.SortFunc(a.servicesRows, func(x, y int32) int {
		s, t := &list[x], &list[y]
		var c int
		switch by {
		case 1:
			c = cmp3(s.PID, t.PID)
		case 2:
			c = cmpFold(s.Display, t.Display)
		case 3:
			c = cmp3(uint32(t.State), uint32(s.State))
		case 4:
			c = cmpFold(s.Group, t.Group)
		default:
			c = cmpFold(s.Name, t.Name)
		}
		if c == 0 {
			return cmpFold(s.Name, t.Name)
		}
		if !asc {
			return -c
		}
		return c
	})
}

// procByKey is the process with key (procKey).
func (a *App) procByKey(key uint64) *sys.Proc {
	for i := range a.sampler.Procs {
		if procKey(&a.sampler.Procs[i]) == key {
			return &a.sampler.Procs[i]
		}
	}
	return nil
}

// selectedRow is the selected row of the Processes page.
func (a *App) selectedRow() *Row {
	if a.selected == 0 {
		return nil
	}
	for i := range a.model.Rows {
		if r := &a.model.Rows[i]; r.Kind != RowHeader && r.Key == a.selected {
			return r
		}
	}
	return nil
}

// cpuGHz is the processor's speed now: its base speed times its
// performance counter (as Task Manager's "Speed").
func (a *App) cpuGHz() float32 {
	base := float32(a.cpuInfo.BaseMHz)
	if a.gpuWarm && a.cpuPerf > 0 {
		return base * a.cpuPerf / 100 / 1000
	}
	return base / 1000
}

// toast shows a message near the bottom of the window ("success",
// "error", "warning" or "info").
func (a *App) toast(kind, text string) {
	a.toasts = append(a.toasts, ui.Toast{Title: text, Type: kind})
	if a.win != nil {
		a.win.Invalidate()
	}
}

// shutdown runs as the window closes.
func (a *App) shutdown() {
	a.closed = true
	a.stopTimer()
	if a.warmTimer != nil {
		a.warmTimer.Stop()
	}
	if a.hardware != nil {
		a.hardware.close()
	}
	a.resolver.Close()
	a.icons.close()
	a.saveSettings()
	if a.perf != nil {
		a.perf.Close()
	}
	a.netmon.Stop()
	a.history.SaveNow()
	a.disks.Close()
}
