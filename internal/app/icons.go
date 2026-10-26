package app

import (
	"image"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"taskmgr/internal/sys"
)

// Programs' icons, made only for rows that show, on a goroutine of its own
// (the shell's icon extraction reads the file), at the screen's density.
// Recently visible icons are kept; a long session must not retain every
// program ever seen in the process list or history.

const maxExeIcons = 256

type exeIcon struct {
	bmp   *ui.Bitmap
	px    int
	state uint8 // 0 none, 1 asked for, 2 made (bmp nil if it has none)
	used  uint64
}

type iconJob struct {
	exe *sys.Exe // nil: the generic icon
	px  int
}

type iconResult struct {
	job iconJob
	img *image.RGBA
}

type iconCache struct {
	byExe    map[*sys.Exe]*exeIcon
	clock    uint64
	generic  exeIcon
	jobs     chan iconJob
	stop     chan struct{}
	stopOnce sync.Once

	mu       sync.Mutex
	results  []iconResult
	notified atomic.Bool
	notify   func()
}

// get keeps the most recently drawn icons. Called only by the UI thread.
func (ic *iconCache) get(exe *sys.Exe) *exeIcon {
	ic.clock++
	e := ic.byExe[exe]
	if e == nil {
		if len(ic.byExe) >= maxExeIcons {
			var oldest *sys.Exe
			used := ^uint64(0)
			for key, icon := range ic.byExe {
				if icon.used < used {
					oldest, used = key, icon.used
				}
			}
			delete(ic.byExe, oldest)
		}
		e = &exeIcon{}
		ic.byExe[exe] = e
	}
	e.used = ic.clock
	return e
}

func (ic *iconCache) init(notify func()) {
	ic.byExe = make(map[*sys.Exe]*exeIcon, 128)
	ic.jobs = make(chan iconJob, 512)
	ic.stop = make(chan struct{})
	ic.notify = notify
	go ic.work()
}

func (ic *iconCache) work() {
	// The shell's icon handlers want a thread of their own in an apartment.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED) == nil {
		defer windows.CoUninitialize()
	}
	for {
		var j iconJob
		select {
		case <-ic.stop:
			return
		case j = <-ic.jobs:
		}
		var img *image.RGBA
		if j.exe == nil {
			img = sys.GenericIcon(j.px)
		} else {
			img = sys.FileIcon(j.exe.Path, j.px)
		}
		ic.mu.Lock()
		ic.results = append(ic.results, iconResult{j, img})
		ic.mu.Unlock()
		if ic.notified.CompareAndSwap(false, true) && ic.notify != nil {
			ic.notify()
		}
	}
}

func (ic *iconCache) close() {
	if ic.stop != nil {
		ic.stopOnce.Do(func() { close(ic.stop) })
	}
}

// take applies the icons made since the last call (UI thread).
func (ic *iconCache) take() {
	ic.notified.Store(false)
	ic.mu.Lock()
	results := ic.results
	ic.results = nil
	ic.mu.Unlock()
	for _, r := range results {
		e := &ic.generic
		if r.job.exe != nil {
			e = ic.byExe[r.job.exe]
			if e == nil {
				continue
			}
		}
		if e.px != r.job.px {
			continue // asked for at another size since
		}
		e.state = 2
		e.bmp = nil
		if r.img != nil {
			e.bmp = ui.NewBitmap(r.img)
		}
	}
}

// ask asks for an icon unless it was asked for at px.
func (ic *iconCache) ask(e *exeIcon, exe *sys.Exe, px int) {
	if e.px == px && e.state != 0 {
		return
	}
	select {
	case ic.jobs <- iconJob{exe, px}:
		e.px, e.state = px, 1
	default:
		// Full: asked again as the row shows next.
	}
}

// iconPx is the icons' size in pixels at the screen's density.
func (a *App) iconPx() int {
	return max(16, int(16*a.scale+0.5))
}

// exeIcon draws a program's icon in a row: its own, else (none, or its
// file can't be read) Windows' icon for programs, as Task Manager shows.
// Blank while its own is still being made, so it doesn't flicker.
func (a *App) exeIcon(c *ui.Context, exe *sys.Exe) {
	px := a.iconPx()
	ic := &a.icons
	if exe != nil {
		e := ic.get(exe)
		if e.px != px || e.state == 0 {
			ic.ask(e, exe, px)
		}
		if e.state != 2 {
			ui.Box(c).Size(16, 16).Shrink(0)
			return
		}
		if e.bmp != nil {
			ui.Image(c, e.bmp).Size(16, 16).Shrink(0)
			return
		}
	}
	g := &ic.generic
	if g.px != px || g.state == 0 {
		ic.ask(g, nil, px)
	}
	if g.bmp != nil {
		ui.Image(c, g.bmp).Size(16, 16).Shrink(0)
		return
	}
	ui.Box(c).Size(16, 16).Shrink(0)
}
