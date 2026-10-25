package app

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// The title bar, drawn in the window as Windows 11's Task Manager draws its
// own: the app's icon and name, the search box in the middle, and the
// minimize, maximize and close buttons. Windows still moves, snaps (the
// maximize button's layouts too) and sizes the window.

const (
	captionW = 46
	titleH   = 48
)

func (a *App) titleBar(c *ui.Context, k *colors) {
	textColor := k.text
	if !a.active {
		textColor = k.tertiary
	}
	ui.Row(c).FillWidth().Height(titleH).AlignItems(ui.Center).Background(k.backdrop).Shrink(0).DragWindow().Children(func() {
		ui.Row(c).Height(titleH).AlignItems(ui.Center).Gap(10).PaddingStart(16).PassThrough().Children(func() {
			logo(c, k, 16)
			ui.Text(c, "Task Manager").FontSize(fontSmall).TextColor(textColor).NoWrap()
		})
		ui.Spacer(c)
		a.searchBox(c, k)
		a.captionButtons(c, k, textColor)
	})
}

// logo is the app's mark: a tile with a pulse on it.
func logo(c *ui.Context, k *colors, size float32) {
	tile := k.cpu
	ui.Box(c).Size(size, size).Shrink(0).Draw(func(p *ui.Painter, r ui.Rect) {
		s := size / 16
		p.Fill(r, tile, 3.5*s)
		var pulse ui.Path
		pulse.MoveTo(r.X+2.5*s, r.Y+9*s).LineTo(r.X+5.5*s, r.Y+9*s).LineTo(r.X+7*s, r.Y+5*s).
			LineTo(r.X+9.3*s, r.Y+12*s).LineTo(r.X+10.8*s, r.Y+8*s).LineTo(r.X+13.5*s, r.Y+8*s)
		p.StrokePath(&pulse, 1.4*s, white)
	})
}

func (a *App) searchBox(c *ui.Context, k *colors) {
	w, _ := c.Size()
	const leftMin, rightMin = 148, 3*captionW + 16
	maxAvail := max(180, w-leftMin-rightMin)
	sw := min(maxAvail, min(max(w*0.38, 220), 420))
	x := min(max((w-sw)/2, leftMin), max(leftMin, w-rightMin-sw))
	focused := a.search.FocusWithin(c)
	border := k.fieldBorder
	if focused {
		border = k.accent
	}
	placeholder := "Type a name, publisher, or PID"
	if a.page == PageServices {
		placeholder = "Search services"
	} else if sw < 340 {
		placeholder = "Search name or PID"
	}
	box := ui.Row(c.Key("search")).Absolute().Left(round(x)).Top(8).Width(round(sw)).Height(32).
		AlignItems(ui.Center).Gap(8).Padding(0, 6, 0, 10).Radius(4).Background(k.field).Border(1, border)
	box.Children(func() {
		icon(c, icSearch, 14, k.secondary)
		in := ui.TextInputBase(c, &a.query).Bind(&a.search).Grow(1).MinWidth(0).FontSize(fontCaption).TextColor(k.text).Placeholder(placeholder).Label("Search")
		// Searching shows the processes found (from any page but these).
		if in.Changed() && a.page != PageProcesses && a.page != PageDetails && a.page != PageServices {
			a.goTo(PageProcesses)
		}
		if a.query != "" {
			x := ui.ButtonBase(c.Key("clear")).Size(24, 24).Radius(4).Label("Clear search").Shrink(0)
			if x.Hovered() {
				x.Background(k.hover)
			}
			if x.Clicked() {
				a.query = ""
			}
			x.Children(func() { icon(c, icDismiss, 11, k.secondary) })
		}
		if focused {
			// Windows 11's underline in the accent, over the bottom edge.
			ui.Box(c).Absolute().Left(0).Right(0).Bottom(-1).Height(2).Radius(1).Background(k.accent)
		}
	})
}

// captionButtons are minimize, maximize (or restore) and close.
func (a *App) captionButtons(c *ui.Context, k *colors, fg ui.Color) {
	ui.Row(c).Height(titleH).Shrink(0).Children(func() {
		for i := range 3 {
			b := ui.ButtonBase(c.Key(i)).Size(captionW, titleH).Shrink(0).FocusRing(false)
			hot, pressed := b.Hovered(), b.Pressed()
			color := fg
			switch {
			case i == 2 && hot:
				bg := k.closeHover
				if pressed {
					bg = bg.Alpha(0.85)
				}
				b.Background(bg)
				color = white
			case pressed:
				b.Background(k.pressed)
			case hot:
				b.Background(k.hover)
			}
			var ic Icon
			switch i {
			case 0:
				ic = icMinimize
				b.Label("Minimize")
			case 1:
				ic = icMaximize
				if a.maximized {
					ic = icRestore
					b.Label("Restore")
				} else {
					b.Label("Maximize")
				}
				b.MaximizeButton()
			default:
				ic = icClose
				b.Label("Close")
			}
			if b.Clicked() && a.win != nil {
				switch i {
				case 0:
					a.win.Minimize()
				case 1:
					if a.maximized {
						a.win.Unmaximize()
					} else {
						a.win.Maximize()
					}
				default:
					a.win.Close()
				}
			}
			b.Children(func() { icon(c, ic, 10, color) })
		}
	})
}

func round(v float32) float32 {
	if v < 0 {
		return float32(int(v - 0.5))
	}
	return float32(int(v + 0.5))
}

// navWidth is the navigation pane's width: folded in a narrow window, as
// Task Manager does.
func (a *App) navWidth(windowW float32) float32 {
	if a.set.NavOpen && windowW >= 760 {
		return 200
	}
	return 48
}

var navItems = [...]struct {
	page Page
	icon Icon
}{
	{PageProcesses, icProcesses},
	{PagePerformance, icPerformance},
	{PageAppHistory, icHistory},
	{PageStartup, icStartup},
	{PageUsers, icUsers},
	{PageDetails, icDetails},
	{PageServices, icServices},
}

// navPane is the menu button, then a page per item (icon and name, or the
// icon alone when folded), the chosen one marked with the accent's pill;
// Settings at the bottom.
func (a *App) navPane(c *ui.Context, k *colors) {
	w, _ := c.Size()
	nw := a.navWidth(w)
	wide := nw > 48
	ui.Column(c).Width(nw).FillHeight().Padding(0, 4, 6, 4).Gap(4).Background(k.backdrop).Shrink(0).Children(func() {
		b := ui.ButtonBase(c.Key("hamburger")).Size(40, 36).Radius(4).Tooltip("Toggle navigation").Shrink(0)
		switch {
		case b.Pressed():
			b.Background(k.pressed)
		case b.Hovered():
			b.Background(k.hover)
		}
		if b.Clicked() {
			a.set.NavOpen = !a.set.NavOpen
			a.saveSettings()
		}
		b.Children(func() { icon(c, icMenu, 16, k.text) })
		for _, it := range navItems {
			a.navItem(c, k, it.page, it.icon, wide)
		}
		ui.Spacer(c)
		ui.Box(c).FillWidth().Height(1).Background(k.divider).Shrink(0)
		a.navItem(c, k, PageSettings, icSettings, wide)
	})
}

func (a *App) navItem(c *ui.Context, k *colors, page Page, ic Icon, wide bool) {
	on := a.page == page
	b := ui.ButtonBase(c.Key(page)).FillWidth().Height(36).Justify(ui.Start).Gap(12).Padding(0, 8, 0, 11).Radius(4).Shrink(0)
	switch {
	case on:
		b.Background(k.navSelected)
	case b.Pressed():
		b.Background(k.pressed)
	case b.Hovered():
		b.Background(k.hover)
	}
	if !wide {
		b.Tooltip(page.Title())
	} else {
		b.Label(page.Title())
	}
	if b.Clicked() && !on {
		a.goTo(page)
	}
	target := float32(0)
	if on {
		target = 1
	}
	t := b.Animate("pill", target, 160*time.Millisecond)
	color := k.text
	if on {
		color = k.accent
	}
	b.Children(func() {
		if t > 0.01 {
			// The pill on the item's left edge, grown from its middle.
			ui.Box(c).Absolute().Left(0).Top(18-8*t).Size(3, 16*t).Radius(1.5).Background(k.accent)
		}
		icon(c, ic, 16, color)
		if wide {
			txt := ui.Text(c, page.Title()).FontSize(fontBody).TextColor(k.text).SingleLine()
			if on {
				txt.FontWeight(600)
			}
		}
	})
}
