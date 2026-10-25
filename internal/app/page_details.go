package app

import (
	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// The Details page: every process on its own row (the idle process too),
// with the columns chosen (right-click a column's title): PID, status,
// user, CPU, memory, architecture and description by default; session,
// CPU time, working set, commit size, threads, handles, base priority,
// elevation, image path, command line and isolation too. Sorted by any
// column.

func (a *App) detailsPage(c *ui.Context, k *colors) {
	pageHeader(c, k, "Details", func() {
		compact := a.narrowerThan(c, 880)
		sel := a.detailsSelected() != nil
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icEndTask, label: "End task", enabled: sel, compact: compact}) {
			a.endDetails(false)
		}
		if commandButton(c, k, command{icon: icProperties, label: "Properties", enabled: sel, compact: compact}) {
			if p := a.detailsSelected(); p != nil {
				a.properties(p.Info.Path())
			}
		}
		if commandButton(c, k, command{icon: icEfficiency, label: "Efficiency mode", enabled: sel, compact: compact}) {
			a.toggleDetailsEfficiency()
		}
	})
	var shownCols [detailColumnCount]DetailCol
	shownList := shownCols[:0]
	for col := DetailCol(0); col < detailColumnCount; col++ {
		if shown(a.set.DetailColumns, col) {
			shownList = append(shownList, col)
		}
	}
	var colsBuf [detailColumnCount + 1]column
	var keysBuf [detailColumnCount + 1]Sort
	cols, keys := colsBuf[:len(shownList)+1], keysBuf[:len(shownList)+1]
	cols[0], keys[0] = column{title: "Name"}, SortName
	minW := float32(240 + pagePad + 24)
	for i, col := range shownList {
		info := detailColumns[col]
		cols[i+1] = column{title: info.title, width: info.width, right: info.right}
		keys[i+1] = info.sort
		minW += info.width + 1
	}
	sorted := -1
	for i, key := range keys {
		if key == a.detailsSort {
			sorted = i
		}
	}
	clicked, header := tableHeader(c, k, cols, sorted, a.detailsAsc, compactHeaderH, minW, true, a.detailColumnsMenu)
	if clicked >= 0 {
		a.detailsSortBy(keys[clicked])
		a.refreshRows()
	}
	if len(a.details) == 0 && a.query != "" {
		empty(c, k, noResults(a.query))
		return
	}
	list := a.sampler.Procs
	a.detailsIdx = -1
	for i, pi := range a.details {
		if procKey(&list[pi]) == a.detailsSel {
			a.detailsIdx = i
		}
	}
	s := &a.detailsList
	s.Selected = &a.detailsIdx
	s.Key = a.detailsRowKey
	s.KeepOffset = true
	s.Label = a.detailsRowLabel
	if a.revealDet && a.detailsIdx >= 0 {
		s.ScrollIntoView(a.detailsIdx)
	}
	a.revealDet = false
	l := a.tableList(c, "details-table", s, len(a.details), minW, header, func(lr ui.ListRow) {
		a.detailsRow(lr.Context, k, &list[a.details[lr.Index]], shownList, cols)
	})
	l.ContextMenu(a.detailsMenu)
	if l.Changed() && a.detailsIdx >= 0 && a.detailsIdx < len(a.details) {
		a.detailsSel = procKey(&list[a.details[a.detailsIdx]])
	}
}

func (a *App) detailsRowKey(i int) any {
	if i < len(a.details) {
		return procKey(&a.sampler.Procs[a.details[i]])
	}
	return i
}

func (a *App) detailsRowLabel(i int) string {
	if i < len(a.details) {
		return a.sampler.Procs[a.details[i]].Info.Name
	}
	return ""
}

func (a *App) detailsSelected() *sys.Proc {
	if a.detailsSel == 0 {
		return nil
	}
	return a.procByKey(a.detailsSel)
}

func (a *App) detailsRow(c *ui.Context, k *colors, p *sys.Proc, shownList []DetailCol, cols []column) {
	key := procKey(p)
	row := ui.Row(c).FillWidth().Height(rowH).AlignItems(ui.Center).Padding(0, 14, 0, pagePad-10).Dividers(1, k.cellDivider)
	row.ContextMenu(func(m *ui.Menu) {
		a.detailsSel = key
		a.detailsMenu(m)
	})
	row.Children(func() {
		cell(c, &cols[0]).Gap(8).Children(func() {
			a.exeIcon(c, p.Info.Exe)
			name := p.Info.Name
			if p.PID == 0 {
				name = "System Idle Process"
			}
			ui.Text(c, name).FontSize(fontCaption).TextColor(k.text).SingleLine()
		})
		muted := k.secondary
		for i, col := range shownList {
			cc := &cols[i+1]
			switch col {
			case dcPID:
				textCell(c, cc, itoa(p.PID), k.text)
			case dcStatus:
				switch {
				case p.Hung:
					textCell(c, cc, "Not responding", k.danger)
				case p.Suspended:
					textCell(c, cc, "Suspended", muted)
				default:
					textCell(c, cc, "Running", muted)
				}
			case dcUser:
				textCell(c, cc, p.Info.User, muted)
			case dcSession:
				textCell(c, cc, itoa(p.Session), muted)
			case dcCPU:
				v := int(min(99, p.CPU+0.5))
				text := itoa(v)
				if v < 10 {
					text = "0" + text
				}
				textCell(c, cc, text, k.text)
			case dcCPUTime:
				textCell(c, cc, fmtCPUTime(p.CPUTime), k.text)
			case dcMemory:
				textCell(c, cc, fmtKB(p.PrivateWS), k.text)
			case dcWorkingSet:
				textCell(c, cc, fmtKB(p.WorkingSet), k.text)
			case dcCommit:
				textCell(c, cc, fmtKB(p.Commit), k.text)
			case dcThreads:
				textCell(c, cc, itoa(p.Threads), k.text)
			case dcHandles:
				textCell(c, cc, grouped(uint64(p.Handles), ""), k.text)
			case dcBasePriority:
				text := "N/A"
				if p.PID != 0 {
					text = priorityOf(p.BasePriority).Title()
				}
				textCell(c, cc, text, muted)
			case dcArch:
				textCell(c, cc, p.Info.Arch.String(), muted)
			case dcElevated:
				text := ""
				if p.Info.Resolved && p.Info.User != "" {
					text = "No"
					if p.Info.Elevated {
						text = "Yes"
					}
				}
				textCell(c, cc, text, muted)
			case dcImagePath:
				textCell(c, cc, p.Info.Path(), muted)
			case dcCommandLine:
				textCell(c, cc, p.Info.Cmdline, muted)
			case dcDescription:
				desc := ""
				if p.Info.Exe != nil {
					desc = p.Info.Exe.Description
				}
				textCell(c, cc, desc, muted)
			case dcIsolation:
				textCell(c, cc, isolation(p), muted)
			}
		}
	})
}

// endDetails ends the selected process, or (with tree) it and every
// process it started, and theirs. It asks first: ending a process can lose
// its data or bring Windows down.
func (a *App) endDetails(tree bool) {
	p := a.detailsSelected()
	if p == nil {
		return
	}
	pids := []uint32{p.PID}
	if tree {
		// Children are found by parent and must be younger than it.
		for i := 0; i < len(pids); i++ {
			parent := pids[i]
			var ptime int64
			if q := a.sampler.ByPID(parent); q != nil {
				ptime = q.Info.CreateTime
			}
			for j := range a.sampler.Procs {
				q := &a.sampler.Procs[j]
				if q.PPID == parent && q.PID != parent && q.Info.CreateTime >= ptime {
					pids = append(pids, q.PID)
				}
			}
		}
	}
	a.confirmPIDs, a.confirmTitle, a.confirmEnd = pids, p.Info.Name, true
}

func (a *App) toggleDetailsEfficiency() {
	p := a.detailsSelected()
	if p == nil {
		return
	}
	a.confirmPIDs = []uint32{p.PID}
	if !p.Info.Efficiency && a.set.AskEfficiency {
		a.confirmEfficiency = true
		return
	}
	a.setEfficiency(a.confirmPIDs, !p.Info.Efficiency)
}

// openAffinity opens the Set affinity dialog for the selection.
func (a *App) openAffinity() {
	p := a.detailsSelected()
	if p == nil {
		return
	}
	mask, _, ok := affinity(p.PID)
	if !ok {
		a.toast("error", "Couldn't read the affinity: access denied")
		return
	}
	a.affinityPID, a.affinityMask, a.affinityOpen, a.confirmTitle = p.PID, mask, true, p.Info.Name
}

// goToServices shows the Services page with the selection's services
// marked.
func (a *App) goToServices() {
	p := a.detailsSelected()
	if p == nil {
		return
	}
	a.servicesPID, a.serviceSel, a.revealSvc = p.PID, "", true
	if a.services != nil {
		for _, s := range a.services.List {
			if s.PID == p.PID {
				a.serviceSel = s.Name
				break
			}
		}
	}
	if a.serviceSel == "" {
		a.toast("info", p.Info.Name+" runs no services")
	}
	a.goTo(PageServices)
	a.query = ""
	a.dirty = true
}
