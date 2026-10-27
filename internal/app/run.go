package app

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"runtime/debug"
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
// The first process sample runs while mygo starts. Descriptions and hardware
// discovery continue in the background without delaying the first frame.
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
	mygo.App.SetName("Argus")
	if !mygo.App.RequestSingleInstanceLock() {
		return nil // the running task manager comes forward
	}
	ready := make(chan *App, 1)
	go func() {
		a := newAppState()
		a.hardware = &hardwareLoad{}
		a.netmon.Start()
		a.sample()
		a.dirty = true
		ready <- a
		h := readHardware()
		if a.hardware.publish(h) {
			a.post(a.takeHardware)
		}
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
		Title:             "Argus",
		Width:             w,
		Height:            h,
		MinWidth:          640,
		MinHeight:         460,
		StateKey:          "main",
		TitleBarStyle:     mygo.TitleBarHidden,
		HideWindowButtons: true,
		AlwaysOnTop:       a.set.AlwaysOnTop,
		BackgroundColor:   "light-dark(#f3f3f3, #1f1f1f)",
		Hidden:            true,
		Content:           ui.View(a.view),
	})
	a.win = win
	win.OnReadyToShow(func() {
		win.SetOpacity(1)
		if background {
			win.ShowInactive()
			a.active = false
		} else {
			win.Show()
		}
	})
	a.hwnd = windows.HWND(win.NativeHandle())
	if !savedState() {
		// Once the window has its frame.
		a.post(func() { fitContent(win, w, h) })
	}
	win.SetIcon(appIcon)
	a.maximized = win.IsMaximized()
	win.OnFocus(func() { a.active = true; win.Invalidate() })
	win.OnBlur(func() { a.active = false; win.Invalidate() })
	sync := func() {
		a.maximized = win.IsMaximized()
		a.dirty = true
		a.syncPerformance(false)
		a.stopTimer()
		a.startTimer()
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
		a.stopTimer()
		a.startTimer()
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
	// Windows doesn't send WM_PAINT to a hidden native surface. Let it
	// paint without exposing the empty client area or taking focus, then
	// reveal the completed frame in OnReadyToShow. Opacity 1 removes the
	// temporary layered-window style, so normal rendering has no extra cost.
	win.SetOpacity(0)
	win.ShowInactive()
	// The first figures that are rates need a second sample: soon.
	a.warmTimer = time.AfterFunc(300*time.Millisecond, func() {
		a.post(func() {
			a.warmTimer = nil
			if a.set.Speed == SpeedPaused {
				return
			}
			a.sample()
			a.updateTray()
			a.startTimer()
		})
	})
}
