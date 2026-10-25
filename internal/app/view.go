package app

import (
	"github.com/egoist/mygo/ui"
)

// The window: the title bar across the top, the navigation pane down the
// left, and the page on a layer of its own (its top-left corner rounded,
// as Windows 11's); menus, dialogs and toasts over it. The view is built
// when something changes (a sample, input) and mygo draws only what
// differs from the last frame.

// isDark reports whether the app is dark now.
func (a *App) isDark() bool {
	switch a.set.Theme {
	case ThemeLight:
		return false
	case ThemeDark:
		return true
	}
	return a.darkSystem
}

// syncTheme makes the palette and the themes again when the mode or the
// accent changed.
func (a *App) syncTheme() {
	dark := a.isDark()
	if a.uiTheme != nil && a.k.dark == dark && a.themeAccent == a.accent {
		return
	}
	a.k = palette(dark, a.accent)
	a.themeAccent = a.accent
	a.uiTheme = uiTheme(&a.k)
	lt := *a.uiTheme
	lt.Accent, lt.AccentText, lt.SurfaceHover, lt.Radius = a.k.rowSelected, a.k.text, a.k.rowHover, 0
	a.listTheme = &lt
}

func (a *App) view(c *ui.Context) {
	a.syncTheme()
	k := &a.k
	c.SetTheme(a.uiTheme)
	a.icons.take()
	if a.sampler.ApplyResolved() {
		a.dirty = true
	}
	for _, t := range a.toasts {
		c.AddToast(t)
	}
	a.toasts = a.toasts[:0]
	a.refreshRows()
	a.keys(c)
	summary := a.page == PagePerformance && a.perfSummary
	root := ui.Column(c).Fill().Background(k.backdrop)
	root.Draw(a.trackScale)
	root.Children(func() {
		a.titleBar(c, k)
		ui.Row(c).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			if !summary {
				a.navPane(c, k)
			}
			// Navigation and search can change the page during this frame.
			a.refreshRows()
			page := ui.Column(c).Grow(1).MinWidth(0).Background(k.page).Clip()
			if !summary {
				page.BorderWidth(1, 0, 0, 1).BorderColor(k.pageBorder).Radius(8, 0, 0, 0)
			}
			page.Children(func() {
				if summary {
					a.perfSummaryView(c, k)
					return
				}
				switch a.page {
				case PageProcesses:
					a.processesPage(c, k)
				case PagePerformance:
					a.performancePage(c, k)
				case PageDetails:
					a.detailsPage(c, k)
				case PageServices:
					a.servicesPage(c, k)
				case PageStartup:
					a.startupPage(c, k)
				case PageUsers:
					a.usersPage(c, k)
				case PageSettings:
					a.settingsPage(c, k)
				case PageAppHistory:
					a.historyPage(c, k)
				}
			})
		})
	})
	a.dialogs(c, k)
}

// trackScale notes the screen's density as the window paints: icons are
// made at it.
func (a *App) trackScale(p *ui.Painter, _ ui.Rect) {
	if s := p.Scale(); s != a.scale && s > 0 {
		a.scale = s
		if a.win != nil {
			a.win.Invalidate()
		}
	}
}

// goTo shows page.
func (a *App) goTo(page Page) {
	if a.page == page {
		return
	}
	a.queries[a.page] = a.query
	a.query = a.queries[page]
	a.page = page
	a.dirty = true
	switch page {
	case PageStartup:
		if !a.startupRead {
			a.readStartup()
		}
	case PageServices:
		a.refreshServices(0)
	case PageUsers:
		a.refreshSessions(0)
	case PagePerformance:
		if a.perfTab.Kind == PerfNet {
			a.requestAddresses()
		}
	}
	a.syncPerformance(page == PagePerformance)
}

// typing reports whether a text field has the keys.
func (a *App) typing(c *ui.Context) bool { return a.search.Focused(c) || a.modalOpen() }

func (a *App) modalOpen() bool {
	return a.runOpen || a.confirmEnd || a.confirmEfficiency || a.confirmUser != userNone || a.historyConfirm || a.affinityOpen
}

// keys are Task Manager's keys: Delete ends the selection, the arrows
// (and Home, End, Page Up and Down) move it, Enter opens a group, Ctrl+F
// finds, F5 refreshes now, Ctrl+P pauses, Ctrl+1–8 and Ctrl+Tab change pages,
// Ctrl+C copies the selection or Performance figures. (The menu key and
// Shift+F10 open the focused table's menu, as mygo's context menus do.)
func (a *App) keys(c *ui.Context) {
	if a.modalOpen() {
		return
	}
	if c.Shortcut(ui.Ctrl, ui.KeyF) || c.Shortcut(ui.Ctrl, ui.KeyE) {
		a.search.Focus(c)
	}
	if c.Shortcut(ui.Ctrl, ui.KeyN) {
		a.openRun()
	}
	for i, p := range navOrder {
		if c.Shortcut(ui.Ctrl, ui.Key1+ui.Key(i)) {
			a.goTo(p)
		}
	}
	back := c.Shortcut(ui.Ctrl|ui.Shift, ui.KeyTab)
	if back || c.Shortcut(ui.Ctrl, ui.KeyTab) {
		at := 0
		for i, p := range navOrder {
			if p == a.page {
				at = i
			}
		}
		n := len(navOrder)
		if back {
			a.goTo(navOrder[(at+n-1)%n])
		} else {
			a.goTo(navOrder[(at+1)%n])
		}
	}
	if c.Shortcut(0, ui.KeyF5) {
		a.refreshNow()
	}
	typing := a.typing(c)
	if !typing && c.Shortcut(ui.Ctrl, ui.KeyP) {
		a.togglePaused()
	}
	if !typing && c.Shortcut(ui.Ctrl, ui.KeyC) {
		if a.page == PagePerformance {
			a.copySummary()
		} else {
			a.copySelection()
		}
	}
	_, h := c.Size()
	pageRows := max(1, int((h-160)/rowH))
	switch a.page {
	case PageProcesses:
		if !typing && !a.procList.Focused(c) {
			a.listKeys(c, pageRows, a.step)
		}
		if !typing {
			if c.Shortcut(0, ui.KeyDelete) {
				a.endSelected()
			}
			if r := a.selectedRow(); r != nil && r.Kind == RowGroup {
				if c.Shortcut(0, ui.KeyRight) && !r.Expanded || c.Shortcut(0, ui.KeyLeft) && r.Expanded || !a.procList.Focused(c) && c.Shortcut(0, ui.KeyEnter) {
					a.model.Toggle(r.Key)
					a.dirty = true
				}
			}
		}
	case PageDetails:
		if !typing && !a.detailsList.Focused(c) {
			a.listKeys(c, pageRows, a.stepDetails)
		}
		if !typing && c.Shortcut(0, ui.KeyDelete) {
			a.endDetails(false)
		}
	}
	if c.Shortcut(0, ui.KeyEscape) {
		switch {
		case a.perfSummary && a.page == PagePerformance:
			a.perfSummary = false
		case a.query != "":
			a.query = ""
		}
	}
}

// listKeys moves a table's selection with the keys while the table hasn't
// the focus (it moves its own when it has).
func (a *App) listKeys(c *ui.Context, pageRows int, step func(int)) {
	switch {
	case c.Shortcut(0, ui.KeyDown):
		step(1)
	case c.Shortcut(0, ui.KeyUp):
		step(-1)
	case c.Shortcut(0, ui.KeyHome):
		step(-1 << 20)
	case c.Shortcut(0, ui.KeyEnd):
		step(1 << 20)
	case c.Shortcut(0, ui.KeyPageUp):
		step(-pageRows)
	case c.Shortcut(0, ui.KeyPageDown):
		step(pageRows)
	}
}

// step moves the Processes page's selection by rows, over the headers.
func (a *App) step(by int) {
	rows := a.model.Rows
	if len(rows) == 0 {
		return
	}
	at := -1
	for i := range rows {
		if rows[i].Key == a.selected && rows[i].Kind != RowHeader {
			at = i
		}
	}
	next := min(max(at+by, 0), len(rows)-1)
	dir := 1
	if by < 0 {
		dir = -1
	}
	for rows[next].Kind == RowHeader {
		next += dir
		if next < 0 || next >= len(rows) {
			// At the top, the first row under its header.
			if dir > 0 {
				return
			}
			dir, next = 1, 0
		}
	}
	a.selected = rows[next].Key
	a.revealSel = true
}

// stepDetails moves the Details page's selection by rows.
func (a *App) stepDetails(by int) {
	rows := a.details
	if len(rows) == 0 {
		return
	}
	at := -1
	for i, pi := range rows {
		if procKey(&a.sampler.Procs[pi]) == a.detailsSel {
			at = i
		}
	}
	next := min(max(at+by, 0), len(rows)-1)
	a.detailsSel = procKey(&a.sampler.Procs[rows[next]])
	a.revealDet = true
}
