package app

import (
	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// The Processes page: apps, background processes and Windows processes,
// each group a row that opens to its processes, with the columns chosen
// (right-click a column's title): status, CPU, memory, disk and network by
// default; type, publisher, PID, process name, command line, GPU, GPU
// engine and isolation too. The busier a figure, the deeper its cell's
// accent tint. Rows are built only as they scroll into view.

const (
	openerW = 20
	nameMin = 300
)

func (a *App) processesPage(c *ui.Context, k *colors) {
	a.processCommands(c, k)
	var shownCols [procColumnCount]ProcCol
	shownList := shownCols[:0]
	for col := ProcCol(0); col < procColumnCount; col++ {
		if shown(a.set.ProcColumns, col) {
			shownList = append(shownList, col)
		}
	}
	minW := float32(nameMin + pagePad + 24)
	for _, col := range shownList {
		minW += procColumns[col].width + 1
	}
	cpu := a.sampler.CPU * 100
	mem := a.mem.Load() * 100
	disk := a.disks.Busiest()
	net := a.net.Load() * 100
	var gpu float32
	for _, g := range a.gpus {
		gpu = max(gpu, g.Use)
	}
	var colsBuf [procColumnCount + 1]column
	var keysBuf [procColumnCount + 1]Sort
	cols, keys := colsBuf[:len(shownList)+1], keysBuf[:len(shownList)+1]
	cols[0], keys[0] = column{title: "Name"}, SortName
	for i, col := range shownList {
		info := procColumns[col]
		cc := &cols[i+1]
		*cc = column{title: info.title, width: info.width, right: info.right}
		keys[i+1] = info.sort
		switch col {
		case pcCPU:
			cc.total, cc.totalHeat = fmtPercent(cpu), cpu/100
		case pcMemory:
			cc.total, cc.totalHeat = fmtPercent(mem), mem/100
		case pcDisk:
			cc.total, cc.totalHeat = fmtPercent(disk*100), disk
		case pcNetwork:
			cc.total, cc.totalHeat = fmtPercent(net), net/100
		case pcGPU:
			cc.total, cc.totalHeat = fmtPercent(gpu), gpu/100
		}
	}
	sorted := -1
	for i, key := range keys {
		if key == a.sort {
			sorted = i
		}
	}
	clicked, header := tableHeader(c, k, cols, sorted, a.ascending, headerH, minW, true, a.procColumnsMenu)
	if clicked >= 0 {
		a.sortBy(keys[clicked])
		a.refreshRows()
	}
	if shown(a.set.ProcColumns, pcNetwork) && !a.netmon.Available {
		header.Tooltip("Per-process network usage is unavailable. Restart as administrator to try tracing.")
	} else if shown(a.set.ProcColumns, pcNetwork) && a.netmon.UsesEnergy() {
		header.Tooltip("Network usage uses Windows counters. A dash means one or more processes could not be read.")
	}
	rows := a.model.Rows
	// The list's choice is the selected key's row, wherever it is now.
	a.selIdx = -1
	for i := range rows {
		if rows[i].Key == a.selected && rows[i].Kind != RowHeader {
			a.selIdx = i
		}
	}
	prev := a.selIdx
	s := &a.procList
	s.Selected = &a.selIdx
	s.Key = a.procRowKey
	s.Header = a.procRowHeader
	s.KeepOffset = true
	s.Label = a.procRowLabel
	if a.revealSel && a.selIdx >= 0 {
		s.ScrollIntoView(a.selIdx)
	}
	a.revealSel = false
	totalMem := float32(max(1, a.mem.Total))
	list := a.tableList(c, "process-table", s, len(rows), minW, header, func(lr ui.ListRow) {
		a.processRow(lr.Context, k, &a.model.Rows[lr.Index], shownList, cols, lr.Selected(), totalMem)
	})
	list.ContextMenu(a.processMenu)
	if list.Changed() && a.selIdx != prev && a.selIdx >= 0 && a.selIdx < len(rows) {
		if rows[a.selIdx].Kind == RowHeader {
			// The choice landed on a group's title: on past it, the way it
			// was going.
			dir := 1
			if prev > a.selIdx {
				dir = -1
			}
			i := a.selIdx
			for i >= 0 && i < len(rows) && rows[i].Kind == RowHeader {
				i += dir
			}
			if i < 0 || i >= len(rows) {
				for i = a.selIdx; i < len(rows) && rows[i].Kind == RowHeader; i++ {
				}
			}
			if i < len(rows) {
				a.selected = rows[i].Key
				a.revealSel = true
			}
		} else {
			a.selected = rows[a.selIdx].Key
		}
	}
	if list.Submitted() {
		if r := a.selectedRow(); r != nil && r.Kind == RowGroup {
			a.model.Toggle(r.Key)
			a.dirty = true
		}
	}
	if len(rows) == 0 && a.query != "" {
		ui.Box(c).Absolute().Top(pageHeaderH + headerH).Left(0).Right(0).Children(func() {
			empty(c, k, noResults(a.query))
		})
	}
}

func (a *App) procRowKey(i int) any {
	if i < len(a.model.Rows) {
		return a.model.Rows[i].Key
	}
	return i
}

func (a *App) procRowHeader(i int) bool {
	return i < len(a.model.Rows) && a.model.Rows[i].Kind == RowHeader
}

func (a *App) procRowLabel(i int) string {
	if i < len(a.model.Rows) {
		return a.model.Rows[i].Title
	}
	return ""
}

func (a *App) processCommands(c *ui.Context, k *colors) {
	pageHeader(c, k, "Processes", func() {
		compact := a.narrowerThan(c, 880)
		sel := a.selectedRow()
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		explorer := a.selectedIsExplorer()
		end := command{icon: icEndTask, label: "End task", enabled: sel != nil, compact: compact}
		if explorer {
			end.icon, end.label = icRestart, "Restart"
		}
		if commandButton(c, k, end) {
			a.endSelected()
		}
		if commandButton(c, k, command{icon: icEfficiency, label: "Efficiency mode", enabled: sel != nil && sel.Category != CatWindows, compact: compact}) {
			a.toggleEfficiency()
		}
		commandButton(c, k, command{icon: icView, label: "View", enabled: true, compact: compact, menu: a.viewMenu})
	})
}

func (a *App) processRow(c *ui.Context, k *colors, r *Row, shownList []ProcCol, cols []column, sel bool, totalMem float32) {
	if r.Kind == RowHeader {
		ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).PaddingStart(pagePad).Background(k.page).Children(func() {
			ui.Text(c, r.Category.Title()+" ("+itoa(a.model.Counts[r.Category])+")").FontSize(fontBody).FontWeight(600).TextColor(k.text).NoWrap()
		})
		return
	}
	if !r.cells {
		r.cells = true
		r.cpuText, r.memText, r.diskText = cpuCell(r.CPU), mbCell(r.Memory), diskCell(r.Disk)
		r.netText, r.gpuText = netCell(r.Net), cpuCell(r.GPU)
		r.titleText = r.Title
		if r.Kind == RowGroup {
			r.titleText = r.Title + " (" + itoa(r.Len) + ")"
		}
	}
	p := &a.sampler.Procs[r.Proc]
	row := ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).Padding(0, 14, 0, pagePad-10).Dividers(1, k.cellDivider)
	hovered := row.Hovered()
	if sel {
		row.Background(k.rowSelected)
	} else if hovered {
		row.Background(k.rowHover)
	}
	row.ContextMenu(func(m *ui.Menu) {
		a.selected = r.Key
		a.processMenu(m)
	})
	group := r.Kind == RowGroup
	row.Children(func() {
		// Name: the opener, the icon, the title.
		cell(c, &cols[0]).Gap(8).Children(func() {
			if r.Depth > 0 {
				ui.Box(c).Width(openerW + 8).Shrink(0)
			}
			if group {
				o := ui.ButtonBase(c.Key("open")).Size(openerW, openerW).Radius(4).Shrink(0).FocusRing(false)
				if o.Hovered() {
					o.Background(k.hover)
				}
				if o.Clicked() {
					a.model.Toggle(r.Key)
					a.dirty = true
				}
				ic := icChevronRight
				if r.Expanded {
					ic = icChevronDown
				}
				o.Children(func() { icon(c, ic, 11, k.secondary) })
			} else if r.Depth == 0 {
				ui.Box(c).Width(openerW).Shrink(0)
			}
			a.exeIcon(c, p.Info.Exe)
			ui.Text(c, r.titleText).FontSize(fontCaption).TextColor(k.text).SingleLine()
		})
		for i, col := range shownList {
			cc := &cols[i+1]
			switch col {
			case pcType:
				textCell(c, cc, r.Category.Title(), k.secondary)
			case pcStatus:
				statusCell(c, k, cc, r)
			case pcPublisher:
				textCell(c, cc, p.Info.Company(), k.secondary)
			case pcPID:
				text := ""
				if !group {
					text = itoa(p.PID)
				}
				textCell(c, cc, text, k.secondary)
			case pcProcessName:
				text := ""
				if !group {
					text = p.Info.Name
				}
				textCell(c, cc, text, k.secondary)
			case pcCommandLine:
				text := ""
				if !group {
					text = p.Info.Cmdline
				}
				textCell(c, cc, text, k.secondary)
			case pcCPU:
				heatCell(c, k, cc, r.cpuText, r.CPU/50, sel, hovered)
			case pcMemory:
				heatCell(c, k, cc, r.memText, float32(r.Memory)/totalMem*4, sel, hovered)
			case pcDisk:
				heatCell(c, k, cc, r.diskText, r.Disk/(50<<20), sel, hovered)
			case pcNetwork:
				text := ""
				if a.netmon.Available {
					text = r.netText
					if r.NetUnknown {
						text = "-"
					}
				}
				heatCell(c, k, cc, text, r.Net*8/50e6, sel, hovered)
			case pcGPU:
				heatCell(c, k, cc, r.gpuText, r.GPU/50, sel, hovered)
			case pcGPUEngine:
				text := ""
				if !group && p.GPU > 0 {
					text = "GPU " + itoa(p.GPUAdapter) + " - " + sys.KindTitle(p.GPUKind)
				}
				textCell(c, cc, text, k.secondary)
			case pcIsolation:
				text := ""
				if !group {
					text = isolation(p)
				}
				textCell(c, cc, text, k.secondary)
			}
		}
	})
}

// statusCell is efficiency mode (a leaf), suspended (a pause), or not
// responding.
func statusCell(c *ui.Context, k *colors, col *column, r *Row) {
	e := cell(c, col).Gap(6)
	if !r.Hung && !r.Efficiency && !r.Suspended {
		return
	}
	e.Children(func() {
		switch {
		case r.Hung:
			ui.Text(c, "Not responding").FontSize(fontSmall).TextColor(k.danger).SingleLine()
		case r.Efficiency:
			icon(c, icEfficiency, 14, k.success)
			ui.Text(c, "Efficiency mode").FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		default:
			icon(c, icPause, 14, k.secondary)
			ui.Text(c, "Suspended").FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		}
	})
}

// selectedIsExplorer reports whether the selected row is Windows Explorer:
// "End task" restarts it.
func (a *App) selectedIsExplorer() bool {
	r := a.selectedRow()
	return r != nil && a.isExplorer(a.sampler.Procs[r.Proc].PID)
}

// endSelected confirms when configured; Windows processes always ask and
// Windows Explorer's command restarts it.
func (a *App) endSelected() {
	r := a.selectedRow()
	if r == nil {
		return
	}
	pids := a.rowPIDs(r)
	if a.selectedIsExplorer() {
		a.restartExplorer(pids)
		return
	}
	critical := false
	for _, pid := range pids {
		if a.critical(pid) {
			critical = true
		}
	}
	if a.set.AskEndTask || r.Category == CatWindows || critical {
		a.confirmProcesses(pids, r.Title)
		return
	}
	a.endProcesses(pids, r.Title)
}

// toggleEfficiency turns efficiency mode on or off for the selection;
// turning it on asks first, unless Settings say not to.
func (a *App) toggleEfficiency() {
	r := a.selectedRow()
	if r == nil {
		return
	}
	a.confirmPIDs = a.rowPIDs(r)
	if !r.Efficiency && a.set.AskEfficiency {
		a.confirmEfficiency = true
		return
	}
	a.setEfficiency(a.confirmPIDs, !r.Efficiency)
}

// selectedPath is the selected row's program file.
func (a *App) selectedPath() string {
	r := a.selectedRow()
	if r == nil {
		return ""
	}
	return a.sampler.Procs[r.Proc].Info.Path()
}
