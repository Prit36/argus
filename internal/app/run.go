package app

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"fmt"
	"runtime"
	"runtime/debug"
	"runtime/pprof"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
)

//go:embed icon.png
var appIcon []byte

// Run opens the task manager's window and runs until it closes.
//
// The machine is read while mygo starts: the first sample, and the names
// of the processes in it, are ready when the window draws its first frame.
// A second sample follows soon (rates need two), then one at each tick of
// the update speed.
func Run() error {
	// The heap is small and long-lived: collect when it grows by half
	// rather than doubles, to keep the working set close to what's live.
	debug.SetGCPercent(50)
	// Draw in software: frames here are small and few (a second apart, and
	// mostly the cells that changed), and the GPU's driver would add about
	// 45 MB and two dozen threads for them. MYGO_GPU=1 draws on the GPU.
	if _, set := os.LookupEnv("MYGO_GPU"); !set {
		os.Setenv("MYGO_GPU", "0")
	}
	if path := os.Getenv("TASKMAN_CPUPROFILE"); path != "" {
		// For measuring: the CPU profile of the whole run.
		if f, err := os.Create(path); err == nil && pprof.StartCPUProfile(f) == nil {
			defer func() { pprof.StopCPUProfile(); f.Close() }()
		}
	}
	mygo.App.SetName("Task Manager")
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running task manager comes forward
	}
	ready := make(chan *App, 1)
	go func() {
		a := newApp()
		a.netmon.Start()
		a.sample()
		a.sampler.WaitResolved(400 * time.Millisecond)
		a.dirty = true
		ready <- a
	}()
	var a *App
	mygo.App.WhenReady(func() {
		a = <-ready
		a.start()
	})
	mygo.App.OnSecondInstance(func([]string, string) {
		if a != nil {
			a.showWindow()
		}
	})
	return mygo.App.Run()
}

// savedState reports whether mygo kept the window's place from a run
// before (window-state.json, under "main").
func savedState() bool {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return false
	}
	data, err := os.ReadFile(filepath.Join(dir, "window-state.json"))
	return err == nil && bytes.Contains(data, []byte(`"main"`))
}

// fitContent sizes the window so its content is w by h DIPs: the first
// time, before mygo remembers where the user put it. (A hidden title bar's
// window has borders of its own that UseContentSize doesn't count.)
func fitContent(win *mygo.Window, w, h int) {
	// The window was made w by h; its content is that, less its borders.
	cw, ch := win.ContentSize()
	if cw == w && ch == h {
		return
	}
	win.SetSize(w+w-cw, h+h-ch)
	win.Center()
}

// start makes the window, the tray icon and the timers (UI thread).
func (a *App) start() {
	a.darkSystem = systemDark()
	a.accent = systemAccent()
	switch a.set.Theme {
	case ThemeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case ThemeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
	w, h := 1080, 720
	if a.set.Width >= 640 && a.set.Width <= 3840 && a.set.Height >= 460 && a.set.Height <= 2160 {
		w, h = int(a.set.Width), int(a.set.Height)
	}
	background := os.Getenv("TASKMAN_BACKGROUND") != "" || os.Getenv("TASKMAN_NO_ACTIVATE") != ""
	win := mygo.NewWindow(mygo.WindowOptions{
		Title:             "Task Manager",
		Width:             w,
		Height:            h,
		MinWidth:          640,
		MinHeight:         460,
		StateKey:          "main",
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		AlwaysOnTop:       a.set.AlwaysOnTop,
		BackgroundColor:   "light-dark(#f3f3f3, #1f1f1f)",
		Hidden:            background,
		Content:           ui.View(a.view),
	})
	a.win = win
	a.hwnd = windows.HWND(win.NativeHandle())
	if !savedState() {
		// Once the window has its frame.
		a.post(func() { fitContent(win, w, h) })
	}
	win.SetIcon(appIcon)
	a.maximized = win.IsMaximized()
	a.icons.notify = func() { a.post(func() {}) }
	if background {
		win.ShowInactive()
		a.active = false
	}
	win.OnFocus(func() { a.active = true; win.Invalidate() })
	win.OnBlur(func() { a.active = false; win.Invalidate() })
	sync := func() {
		a.maximized = win.IsMaximized()
		a.dirty = true
		a.syncPerformance(false)
		win.Invalidate()
	}
	win.OnMaximize(sync)
	win.OnUnmaximize(sync)
	win.OnRestore(sync)
	win.OnShow(sync)
	win.OnMinimize(func() {
		// Minimized, into the notification area.
		if a.set.HideMinimized {
			win.Hide()
		}
		a.syncPerformance(false)
	})
	win.OnClose(func(*mygo.CloseEvent) {
		// The content's size, kept as the window was left.
		if !win.IsMaximized() && !win.IsMinimized() {
			if w, h := win.ContentSize(); w >= 640 && h >= 460 {
				a.set.Width, a.set.Height = uint32(w), uint32(h)
			}
		}
	})
	win.OnClosed(func() {
		if path := os.Getenv("TASKMAN_MEMPROFILE"); path != "" {
			// For measuring: the heap as the window closes.
			writeHeapProfile(path)
		}
		a.shutdown()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	mygo.Theme.OnUpdated(func() {
		a.darkSystem = systemDark()
		a.accent = systemAccent()
		win.Invalidate()
	})
	a.addTray()
	a.opened(win)
	// For testing: the page to show first (its number in settings.ini).
	if n, err := strconv.Atoi(os.Getenv("TASKMAN_PAGE")); err == nil && n >= 0 && n <= int(PageAppHistory) {
		a.goTo(Page(n))
	}
	a.syncPerformance(true)
	// The first figures that are rates need a second sample: soon.
	time.AfterFunc(300*time.Millisecond, func() {
		a.post(func() {
			a.sample()
			a.updateTray()
			a.startTimer()
		})
	})
}

// writeHeapProfile writes the live heap, and the runtime's memory figures,
// to path (and path.txt).
func writeHeapProfile(path string) {
	runtime.GC()
	if f, err := os.Create(path); err == nil {
		pprof.Lookup("heap").WriteTo(f, 0)
		f.Close()
	}
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	os.WriteFile(path+".txt", []byte(fmt.Sprintf("heap inuse %d KB, heap idle %d KB, released %d KB, stacks %d KB, sys %d KB, objects %d, gc %d",
		m.HeapInuse>>10, m.HeapIdle>>10, m.HeapReleased>>10, m.StackInuse>>10, m.Sys>>10, m.HeapObjects, m.NumGC)), 0o644)
}
