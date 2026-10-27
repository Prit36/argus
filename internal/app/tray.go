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

// trayIcons are the meter's PNGs, made once per filled pixel row and size.
var trayIcons = map[int][]byte{}

// The tray is UI-thread-only: reuse the PNG encoder's buffer across levels.
type trayBufferPool struct{ buffer *png.EncoderBuffer }

func (p *trayBufferPool) Get() *png.EncoderBuffer {
	b := p.buffer
	p.buffer = nil
	return b
}

func (p *trayBufferPool) Put(b *png.EncoderBuffer) { p.buffer = b }

// The four-color icon stays compact as an indexed PNG. Skipping compression
// avoids retaining the much larger fast-compressor scratch buffer.
var trayEncoder = png.Encoder{CompressionLevel: png.NoCompression, BufferPool: &trayBufferPool{}}
var trayPalette = color.Palette{
	color.NRGBA{},
	color.NRGBA{0x8a, 0x8a, 0x8a, 0xff},
	color.NRGBA{0x2e, 0xb8, 0x2e, 0xff},
	color.NRGBA{0x1c, 0x1c, 0x1c, 0xff},
}

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
	key := meterKey(px, level)
	if b, ok := trayIcons[key]; ok {
		return b
	}
	img := image.NewPaletted(image.Rect(0, 0, px, px), trayPalette)
	inset := px / 8
	left, right := inset, px-inset
	fillTop := px - inset - (key & 0xff) - 1
	for y := inset; y < px-inset; y++ {
		for x := left; x < right; x++ {
			switch {
			case y == inset || y == px-inset-1 || x == left || x == right-1:
				img.SetColorIndex(x, y, 1)
			case y >= fillTop:
				img.SetColorIndex(x, y, 2)
			default:
				img.SetColorIndex(x, y, 3)
			}
		}
	}
	var b bytes.Buffer
	trayEncoder.Encode(&b, img)
	trayIcons[key] = b.Bytes()
	return b.Bytes()
}

func meterKey(px, level int) int {
	fill := int(math.Round(float64(px-2*(px/8)-2) * float64(min(100, max(0, level))) / 100))
	return px<<8 | fill
}

func (a *App) trayTip() string {
	return "Argus\nCPU: " + fmtPercent(a.sampler.CPU*100) +
		"\nMemory: " + fmtPercent(a.mem.Load()*100) +
		"\nDisk: " + fmtPercent(a.disks.Busiest()*100) +
		"\nNetwork: " + fmtPercent(a.net.Load()*100)
}

func (a *App) cpuLevel() int { return int(math.Round(float64(a.sampler.CPU * 100))) }

func (a *App) addTray() {
	level := a.cpuLevel()
	px, tip := a.trayPx(), a.trayTip()
	t, err := mygo.NewTray(mygo.TrayOptions{ID: "cpu", Icon: meter(px, level), ToolTip: tip})
	if err != nil {
		return
	}
	a.tray, a.trayKey, a.trayText = t, meterKey(px, level), tip
	t.OnClick(a.showWindow)
	t.OnRightClick(func() {
		t.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Label: "Show Argus", Click: func(*mygo.MenuItem, *mygo.Window) { a.showWindow() }},
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

// updateTray publishes only changes to the pixels, DPI size or tooltip.
func (a *App) updateTray() {
	if a.tray == nil {
		return
	}
	px, level := a.trayPx(), a.cpuLevel()
	if key := meterKey(px, level); key != a.trayKey {
		a.trayKey = key
		a.tray.SetIcon(meter(px, level), false)
	}
	if tip := a.trayTip(); tip != a.trayText {
		a.trayText = tip
		a.tray.SetToolTip(tip)
	}
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
