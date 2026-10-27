package app

import (
	"errors"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"

	"taskmgr/internal/sys"
)

// The Services, Startup apps and App history pages: simple tables, each
// sorted by the column clicked and searched.

var serviceCols = [...]column{
	{title: "Name", width: 180},
	{title: "PID", width: 72, right: true},
	{title: "Description"},
	{title: "Status", width: 104},
	{title: "Group", width: 176},
}

// The Services page: each Windows service's name, the process it runs in,
// its description, status and group; started, stopped and restarted from
// the command bar or its menu (most need administrator rights).
func (a *App) servicesPage(c *ui.Context, k *colors) {
	pageHeader(c, k, "Services", func() {
		compact := a.narrowerThan(c, 960)
		sv := a.serviceSelected()
		running := sv != nil && sv.State == sys.ServiceRunning
		available := !a.serviceActionBusy
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icPlay, label: "Start", enabled: available && sv != nil && !running, compact: compact}) {
			a.controlService(sys.ServiceStart)
		}
		if commandButton(c, k, command{icon: icStop, label: "Stop", enabled: available && running, compact: compact}) {
			a.controlService(sys.ServiceStop)
		}
		if commandButton(c, k, command{icon: icRestart, label: "Restart", enabled: available && running, compact: compact}) {
			a.controlService(sys.ServiceRestart)
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icServices, label: "Open services", enabled: true, compact: compact}) {
			open("services.msc")
		}
	})
	cols := serviceCols
	minW, flexMin := a.applyColumnWidths(cols[:], 820)
	clicked, header := a.tableHeader(c, k, cols[:], int(a.servicesSort), a.servicesAsc, compactHeaderH, &minW, flexMin, true, nil)
	if clicked >= 0 {
		a.servicesSortBy(uint8(clicked))
		a.refreshRows()
	}
	if len(a.servicesRows) == 0 {
		msg := "There are no services to display"
		if a.query != "" {
			msg = noResults(a.query)
		}
		empty(c, k, msg)
		return
	}
	list := a.services.List
	a.serviceIdx = -1
	for i, si := range a.servicesRows {
		if list[si].Name == a.serviceSel {
			a.serviceIdx = i
		}
	}
	s := &a.servicesList
	s.Selected = &a.serviceIdx
	s.Key = a.serviceRowKey
	s.Label = a.serviceRowKey2
	if a.revealSvc && a.serviceIdx >= 0 {
		s.ScrollTo(a.serviceIdx, ui.Center)
	}
	a.revealSvc = false
	l := a.tableList(c, "services-table", s, len(a.servicesRows), minW, header, func(lr ui.ListRow) {
		a.serviceRow(lr.Context, k, &list[a.servicesRows[lr.Index]], cols[:])
	})
	l.ContextMenu(a.serviceMenu)
	if l.Changed() && a.serviceIdx >= 0 && a.serviceIdx < len(a.servicesRows) {
		a.servicesPID = 0
		a.serviceSel = list[a.servicesRows[a.serviceIdx]].Name
	}
}

func (a *App) serviceRowKey(i int) any {
	if a.services != nil && i < len(a.servicesRows) {
		return a.services.List[a.servicesRows[i]].Name
	}
	return i
}

func (a *App) serviceRowKey2(i int) string {
	if a.services != nil && i < len(a.servicesRows) {
		return a.services.List[a.servicesRows[i]].Name
	}
	return ""
}

func (a *App) serviceRow(c *ui.Context, k *colors, s *sys.Service, cols []column) {
	marked := a.servicesPID != 0 && s.PID == a.servicesPID
	row := ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).Padding(0, 14, 0, pagePad-10).Gap(1).Dividers(1, k.cellDivider)
	if marked {
		row.Background(k.rowSelected)
	}
	name := s.Name
	row.ContextMenu(func(m *ui.Menu) {
		if !marked {
			a.servicesPID = 0
		}
		a.serviceSel = name
		a.serviceMenu(m)
	})
	row.Children(func() {
		textCell(c, &cols[0], s.Name, k.text)
		pid := ""
		if s.PID != 0 {
			pid = itoa(s.PID)
		}
		textCell(c, &cols[1], pid, k.secondary)
		textCell(c, &cols[2], s.Display, k.text)
		state := k.secondary
		if s.State == sys.ServiceRunning {
			state = k.text
		}
		textCell(c, &cols[3], s.State.String(), state)
		textCell(c, &cols[4], s.Group, k.secondary)
	})
}

func (a *App) serviceSelected() *sys.Service {
	if a.serviceSel == "" || a.services == nil {
		return nil
	}
	for i := range a.services.List {
		if a.services.List[i].Name == a.serviceSel {
			return &a.services.List[i]
		}
	}
	return nil
}

// controlService starts, stops or restarts the selected service, on a
// goroutine: a restart waits for it to stop.
func (a *App) controlService(action sys.ServiceAction) {
	s := a.serviceSelected()
	if s == nil || a.serviceActionBusy {
		return
	}
	name := s.Name
	a.serviceActionBusy = true
	a.toast("info", "Sending request to "+action.String()+" "+name+"…")
	go func() {
		err := sys.ControlService(name, action)
		a.post(func() {
			a.serviceActionBusy = false
			if err != nil {
				a.toast("error", "Couldn't "+action.String()+" "+name+": "+actionError(err))
			} else {
				a.toast("success", "Request to "+action.String()+" "+name+" accepted")
			}
			if a.servicesBusy {
				a.serviceReloadPending = true
			} else {
				a.refreshServices(0)
			}
		})
	}()
}

func actionError(err error) string {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return "access denied; try running as administrator"
	}
	return strings.TrimSpace(err.Error())
}

var startupCols = [...]column{
	{title: "Name"},
	{title: "Publisher", width: 220},
	{title: "Status", width: 110},
	{title: "Startup impact", width: 136},
}

// The Startup apps page: what starts at sign-in, its publisher, and
// whether it's enabled; enabled and disabled as Task Manager does it.
func (a *App) startupPage(c *ui.Context, k *colors) {
	if !a.startupRead {
		a.readStartup()
	}
	item := a.startupItem()
	pageHeader(c, k, "Startup apps", func() {
		compact := a.narrowerThan(c, 800)
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icEnable, label: "Enable", enabled: !a.startupActionBusy && item != nil && !item.Enabled, compact: compact}) {
			a.setStartupEnabled(true)
		}
		if commandButton(c, k, command{icon: icDisable, label: "Disable", enabled: !a.startupActionBusy && item != nil && item.Enabled, compact: compact}) {
			a.setStartupEnabled(false)
		}
		if commandButton(c, k, command{icon: icProperties, label: "Properties", enabled: item != nil, compact: compact}) {
			a.properties(item.Path)
		}
	})
	ui.Row(c).FillWidth().Padding(0, pagePad, 8, pagePad).Shrink(0).Children(func() {
		ui.Spacer(c)
		if bios := a.biosSeconds(); bios > 0 {
			ui.Text(c, "Last BIOS time:  "+string(fixed(nil, float64(bios), 1))+" seconds").FontSize(fontSmall).TextColor(k.secondary).NoWrap()
		}
	})
	items := a.startup
	q := strings.TrimSpace(a.query)
	cols := startupCols
	minW, flexMin := a.applyColumnWidths(cols[:], 760)
	clicked, header := a.tableHeader(c, k, cols[:], int(a.startupSort), a.startupAsc, compactHeaderH, &minW, flexMin, true, nil)
	if clicked >= 0 {
		if int(a.startupSort) == clicked {
			a.startupAsc = !a.startupAsc
		} else {
			a.startupSort, a.startupAsc = uint8(clicked), true
		}
		a.dirty = true
		a.refreshRows()
		a.saveSettings()
	}
	if len(a.startupOrder) == 0 {
		msg := "There are no startup items to display"
		if q != "" {
			msg = noResults(q)
		} else if !a.startupRead {
			msg = ""
		}
		empty(c, k, msg)
		return
	}
	a.startupIdx = -1
	for i, si := range a.startupOrder {
		if int(si) == a.startupSel {
			a.startupIdx = i
		}
	}
	s := &a.startupList
	s.Selected = &a.startupIdx
	s.Key = a.startupRowKey
	l := a.tableList(c, "startup-table", s, len(a.startupOrder), minW, header, func(lr ui.ListRow) {
		i := int(a.startupOrder[lr.Index])
		a.startupRow(lr.Context, k, &items[i], i, cols[:])
	})
	l.ContextMenu(a.startupMenu)
	if l.Changed() && a.startupIdx >= 0 && a.startupIdx < len(a.startupOrder) {
		a.startupSel = int(a.startupOrder[a.startupIdx])
	}
}

func (a *App) startupRowKey(i int) any {
	if i < len(a.startupOrder) {
		return a.startupOrder[i]
	}
	return -1 - i
}

var bios float32 = -1

func (a *App) biosSeconds() float32 {
	if bios < 0 {
		bios = sys.BiosSeconds()
	}
	return bios
}

func (a *App) startupItem() *sys.StartupItem {
	if a.startupSel < 0 || a.startupSel >= len(a.startup) {
		return nil
	}
	return &a.startup[a.startupSel]
}

func (a *App) startupRow(c *ui.Context, k *colors, it *sys.StartupItem, i int, cols []column) {
	row := ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).Padding(0, 14, 0, pagePad-10).Gap(1).Dividers(1, k.cellDivider)
	row.ContextMenu(func(m *ui.Menu) {
		a.startupSel = i
		a.startupMenu(m)
	})
	row.Children(func() {
		cell(c, &cols[0]).Gap(8).Children(func() {
			a.exeIcon(c, it.Exe)
			ui.Text(c, it.Name).FontSize(fontCaption).TextColor(k.text).SingleLine()
		})
		textCell(c, &cols[1], it.Publisher, k.secondary)
		status, impact := "Disabled", "None"
		if it.Enabled {
			status, impact = "Enabled", "Not measured"
		}
		textCell(c, &cols[2], status, k.text)
		textCell(c, &cols[3], impact, k.secondary)
	})
}

func (a *App) setStartupEnabled(on bool) {
	it := a.startupItem()
	if it == nil || a.startupActionBusy {
		return
	}
	item := *it
	verb := "disable"
	if on {
		verb = "enable"
	}
	a.startupActionBusy = true
	a.toast("info", "Working to "+verb+" "+item.Name+"…")
	go func() {
		err := item.SetEnabled(on)
		a.post(func() {
			a.startupActionBusy = false
			if err != nil {
				a.toast("error", "Couldn't "+verb+" "+item.Name+": "+actionError(err))
			} else {
				state := "Disabled"
				if on {
					state = "Enabled"
				}
				a.toast("success", state+" "+item.Name)
			}
			if a.startupBusy {
				a.startupReloadPending = true
			} else {
				a.readStartup()
			}
		})
	}()
}

var historyCols = [...]column{
	{title: "Name"},
	{title: "CPU time", width: 120, right: true},
	{title: "Network", width: 120, right: true},
}

// The App history page: the processor time and network each of the user's
// apps has used since a date, sorted by any column; "Delete usage history"
// starts the count again.
func (a *App) historyPage(c *ui.Context, k *colors) {
	h := a.history
	pageHeader(c, k, "App history", func() {
		compact := a.narrowerThan(c, 760)
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icDismiss, label: "Delete usage history", enabled: len(h.List) > 0, compact: compact}) {
			a.historyConfirm = true
		}
	})
	ui.Row(c).FillWidth().Padding(0, pagePad, 8, pagePad).Gap(6).Shrink(0).Children(func() {
		ui.Text(c, "Resource usage since "+sys.FiletimeDate(h.Since)+" for current user account.").FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		if !a.netmon.Available {
			ui.Text(c, "Network usage is unavailable; restarting as administrator may enable it.").FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		}
	})
	q := strings.TrimSpace(a.query)
	cols := historyCols
	minW, flexMin := a.applyColumnWidths(cols[:], 560)
	clicked, header := a.tableHeader(c, k, cols[:], int(a.historySort), a.historyAsc, compactHeaderH, &minW, flexMin, true, nil)
	if clicked >= 0 {
		if int(a.historySort) == clicked {
			a.historyAsc = !a.historyAsc
		} else {
			a.historySort, a.historyAsc = uint8(clicked), clicked == 0
		}
		a.dirty = true
		a.refreshRows()
		a.saveSettings()
	}
	if len(a.historyRows) == 0 {
		msg := "No app history yet: it's counted while Task Manager runs"
		if q != "" {
			msg = noResults(q)
		}
		empty(c, k, msg)
		return
	}
	s := &a.historyList
	s.Key = a.historyRowKey
	a.tableList(c, "history-table", s, len(a.historyRows), minW, header, func(lr ui.ListRow) {
		c := lr.Context
		e := &h.List[a.historyRows[lr.Index]]
		ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).Padding(0, 14, 0, pagePad-10).Gap(1).Dividers(1, k.cellDivider).Tooltip(e.Path).Children(func() {
			cell(c, &cols[0]).Gap(8).Children(func() {
				a.exeIcon(c, e.Exe)
				ui.Text(c, e.Title).FontSize(fontCaption).TextColor(k.text).SingleLine()
			})
			textCell(c, &cols[1], fmtCPUTime(e.CPU), k.text)
			textCell(c, &cols[2], mbTotal(e.Net), k.text)
		})
	})
}

func (a *App) historyRowKey(i int) any {
	if i < len(a.historyRows) {
		return a.history.List[a.historyRows[i]].Key
	}
	return i
}

func (a *App) buildStartup() {
	// Sorted by the column clicked; searched.
	items := a.startup
	q := strings.TrimSpace(a.query)
	a.startupOrder = a.startupOrder[:0]
	for i := range items {
		status := "Disabled"
		if items[i].Enabled {
			status = "Enabled"
		}
		if q != "" && !containsFold(items[i].Name, q) && !containsFold(items[i].Publisher, q) &&
			!containsFold(items[i].Path, q) && !containsFold(status, q) {
			continue
		}
		a.startupOrder = append(a.startupOrder, int32(i))
	}
	by, asc := a.startupSort, a.startupAsc
	slices.SortFunc(a.startupOrder, func(x, y int32) int {
		p, r := &items[x], &items[y]
		var c int
		switch by {
		case 1:
			c = cmpFold(p.Publisher, r.Publisher)
		case 2, 3:
			c = cmp3(b2i(r.Enabled), b2i(p.Enabled))
		default:
			c = cmpFold(p.Name, r.Name)
		}
		if c == 0 {
			return cmpFold(p.Name, r.Name)
		}
		if !asc {
			return -c
		}
		return c
	})
}

func (a *App) buildHistory() {
	h := a.history
	// The rows: apps (or every program, as Settings says), searched.
	q := strings.TrimSpace(a.query)
	a.historyRows = a.historyRows[:0]
	for i := range h.List {
		e := &h.List[i]
		if !e.App && !a.set.HistoryAll {
			continue
		}
		if q != "" && !containsFold(e.Title, q) && !containsFold(e.Path, q) {
			continue
		}
		a.historyRows = append(a.historyRows, int32(i))
	}
	by, asc := a.historySort, a.historyAsc
	slices.SortFunc(a.historyRows, func(x, y int32) int {
		p, r := &h.List[x], &h.List[y]
		var c int
		switch by {
		case 1:
			c = cmp3(p.CPU, r.CPU)
		case 2:
			c = cmp3(p.Net, r.Net)
		default:
			c = cmpFold(p.Title, r.Title)
		}
		if c == 0 {
			return cmpFold(p.Title, r.Title)
		}
		if !asc {
			return -c
		}
		return c
	})
}
