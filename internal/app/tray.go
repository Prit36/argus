package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"

	"github.com/egoist/mygo"

	"taskmgr/internal/winapi"
)

// The notification area's icon, as Task Manager's: a meter of the CPU's
// use, redrawn as it changes, its tooltip the CPU, memory, disk and
// network figures. A click shows the window (hidden there while
// minimized, when Settings say so); a right click offers Show and Close.

// trayIcons are the meter's PNGs, made once per level and size.
var trayIcons = map[int][]byte{}

func (a *App) trayPx() int {
	scale := float64(1)
	if a.win != nil {
		if dpi := winapi.GetDpiForWindow(a.hwnd); dpi > 0 {
			scale = float64(dpi) / 96
		}
	}
	return min(64, max(16, int(math.Round(16*scale))))
}

// meter is the icon: a frame, filled from the bottom as busy as the CPU.
func meter(px, level int) []byte {
	key := px<<8 | level
	if b, ok := trayIcons[key]; ok {
		return b
	}
	img := image.NewNRGBA(image.Rect(0, 0, px, px))
	inset := px / 8
	left, right := inset, px-inset
	fillTop := px - inset - int(math.Round(float64(px-2*inset-2)*float64(level)/100)) - 1
	edge := color.NRGBA{0x8a, 0x8a, 0x8a, 0xff}
	fill := color.NRGBA{0x2e, 0xb8, 0x2e, 0xff} // Task Manager's green
	empty := color.NRGBA{0x1c, 0x1c, 0x1c, 0xff}
	for y := inset; y < px-inset; y++ {
		for x := left; x < right; x++ {
			switch {
			case y == inset || y == px-inset-1 || x == left || x == right-1:
				img.SetNRGBA(x, y, edge)
			case y >= fillTop:
				img.SetNRGBA(x, y, fill)
			default:
				img.SetNRGBA(x, y, empty)
			}
		}
	}
	var b bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestSpeed}
	enc.Encode(&b, img)
	trayIcons[key] = b.Bytes()
	return b.Bytes()
}

func (a *App) trayTip() string {
	return "CPU: " + fmtPercent(a.sampler.CPU*100) +
		"\nMemory: " + fmtPercent(a.mem.Load()*100) +
		"\nDisk: " + fmtPercent(a.disks.Busiest()*100) +
		"\nNetwork: " + fmtPercent(a.net.Load()*100)
}

func (a *App) cpuLevel() int { return int(math.Round(float64(a.sampler.CPU * 100))) }

func (a *App) addTray() {
	level := a.cpuLevel()
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "cpu", Icon: meter(a.trayPx(), level), ToolTip: a.trayTip()})
	if err != nil {
		return
	}
	a.tray, a.trayLevel = t, level
	t.OnClick(a.showWindow)
	t.OnRightClick(func() {
		t.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Show Task Manager", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
			mygo.Separator(),
			{Label: "Close", Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.win != nil {
					a.win.Close()
				}
			}},
		}))
		t.PopUpMenu()
		t.SetMenu(nil)
	})
}

// updateTray redraws the meter (when its level changed) and the tooltip.
func (a *App) updateTray() {
	if a.tray == nil {
		return
	}
	if level := a.cpuLevel(); level != a.trayLevel {
		a.trayLevel = level
		a.tray.SetIcon(meter(a.trayPx(), level), false)
	}
	a.tray.SetToolTip(a.trayTip())
}

// showWindow shows the window, restored and in front.
func (a *App) showWindow() {
	if a.win == nil {
		return
	}
	a.win.Show()
	if a.win.IsMinimized() {
		a.win.Restore()
	}
	a.win.Focus()
	a.dirty = true
	a.syncPerformance(false)
}
