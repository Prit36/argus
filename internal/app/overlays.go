package app

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// What opens over the page: the right-click menus of each table and of the
// tables' column titles, the View and "…" menus (the system's own menus),
// the Run new task dialog, the confirmations (ending a Windows process,
// efficiency mode, signing a user out, deleting app history) and Set
// affinity.

// processMenu is the Processes page's menu, for the selected row.
func (a *App) processMenu(m *ui.Menu) {
	r := a.selectedRow()
	if r == nil {
		return
	}
	a.copyMenu(m)
	if r.Kind == RowGroup {
		label := "Expand"
		if r.Expanded {
			label = "Collapse"
		}
		if m.Item(label).Chosen() {
			a.model.Toggle(r.Key)
			a.dirty = true
		}
	}
	if r.Category == CatApps && m.Item("Switch to").Chosen() {
		a.switchTo(a.rowPIDs(r))
	}
	end := "End task"
	if a.selectedIsExplorer() {
		end = "Restart"
	}
	if m.Item(end).Shortcut(0, ui.KeyDelete).Chosen() {
		a.endSelected()
	}
	if m.Item("Efficiency mode").Checked(r.Efficiency).Disabled(r.Category == CatWindows).Chosen() {
		a.toggleEfficiency()
	}
	p := &a.sampler.Procs[r.Proc]
	if m.Item("Create memory dump file").Chosen() {
		a.createDump(p.PID, p.Info.Name)
	}
	m.Separator()
	if m.Item("Go to details").Chosen() {
		a.goToDetails(p.PID)
	}
	if m.Item("Open file location").Chosen() {
		a.openLocation(p.Info.Path())
	}
	if m.Item("Search online").Chosen() {
		searchOnline(p.Info.Name, r.Title)
	}
	if m.Item("Properties").Chosen() {
		a.properties(p.Info.Path())
	}
}

// detailsMenu is the Details page's menu, for the selected process.
func (a *App) detailsMenu(m *ui.Menu) {
	p := a.detailsSelected()
	if p == nil {
		return
	}
	a.copyMenu(m)
	pid, name, path, title := p.PID, p.Info.Name, p.Info.Path(), p.Info.Title()
	if m.Item("End task").Shortcut(0, ui.KeyDelete).Chosen() {
		a.endDetails(false)
	}
	if m.Item("End process tree").Chosen() {
		a.endDetails(true)
	}
	if m.Item("Efficiency mode").Checked(p.Info.Efficiency).Chosen() {
		a.toggleDetailsEfficiency()
	}
	m.Separator()
	m.Submenu("Set priority", func(m *ui.Menu) {
		cur := priorityOf(p.BasePriority)
		for _, pr := range priorities {
			if m.Item(pr.Title()).Checked(cur == pr).Chosen() {
				a.setPriority(pid, pr)
			}
		}
	})
	if m.Item("Set affinity").Chosen() {
		a.openAffinity()
	}
	m.Separator()
	if m.Item("Create memory dump file").Chosen() {
		a.createDump(pid, name)
	}
	if m.Item("Open file location").Chosen() {
		a.openLocation(path)
	}
	if m.Item("Search online").Chosen() {
		searchOnline(name, title)
	}
	if m.Item("Properties").Chosen() {
		a.properties(path)
	}
	if m.Item("Go to service(s)").Chosen() {
		a.goToServices()
	}
}

func (a *App) serviceMenu(m *ui.Menu) {
	s := a.serviceSelected()
	if s == nil {
		return
	}
	a.copyMenu(m)
	running := s.State == sys.ServiceRunning
	if m.Item("Start").Disabled(running).Chosen() {
		a.controlService(sys.ServiceStart)
	}
	if m.Item("Stop").Disabled(!running).Chosen() {
		a.controlService(sys.ServiceStop)
	}
	if m.Item("Restart").Disabled(!running).Chosen() {
		a.controlService(sys.ServiceRestart)
	}
	m.Separator()
	if m.Item("Open Services").Chosen() {
		open("services.msc")
	}
	if m.Item("Search online").Chosen() {
		searchOnline(s.Name, s.Display)
	}
	if m.Item("Go to details").Disabled(s.PID == 0).Chosen() {
		a.goToDetails(s.PID)
	}
}

func (a *App) startupMenu(m *ui.Menu) {
	it := a.startupItem()
	if it == nil {
		return
	}
	a.copyMenu(m)
	if it.Enabled {
		if m.Item("Disable").Chosen() {
			a.setStartupEnabled(false)
		}
	} else if m.Item("Enable").Chosen() {
		a.setStartupEnabled(true)
	}
	m.Separator()
	if m.Item("Open file location").Chosen() {
		a.openLocation(it.Path)
	}
	if m.Item("Search online").Chosen() {
		searchOnline(it.Name, it.Publisher)
	}
	if m.Item("Properties").Chosen() {
		a.properties(it.Path)
	}
}

func (a *App) userMenu(m *ui.Menu) {
	if !a.hasUserSel {
		return
	}
	id := a.userSel
	label := "Expand"
	if a.usersOpen[id] {
		label = "Collapse"
	}
	if m.Item(label).Chosen() {
		a.usersOpen[id] = !a.usersOpen[id]
	}
	if m.Item("Disconnect").Chosen() {
		a.confirmUser = userDisconnect
	}
	if m.Item("Sign out").Chosen() {
		a.confirmUser = userSignOut
	}
	m.Separator()
	if m.Item("Manage user accounts").Chosen() {
		open("ms-settings:otherusers")
	}
}

func (a *App) userProcMenu(m *ui.Menu) {
	p := a.userProc()
	if p == nil {
		return
	}
	a.copyMenu(m)
	pid, name, path, title := p.PID, p.Info.Name, p.Info.Path(), p.Info.Title()
	if m.Item("End task").Chosen() {
		a.endUserProc()
	}
	m.Separator()
	if m.Item("Go to details").Chosen() {
		a.goToDetails(pid)
	}
	if m.Item("Open file location").Chosen() {
		a.openLocation(path)
	}
	if m.Item("Search online").Chosen() {
		searchOnline(name, title)
	}
	if m.Item("Properties").Chosen() {
		a.properties(path)
	}
}

// viewMenu is the Processes page's View menu.
func (a *App) viewMenu(m *ui.Menu) {
	if m.Item("Group by type").Checked(a.set.GroupByType).Chosen() {
		a.set.GroupByType = !a.set.GroupByType
		a.dirty = true
		a.saveSettings()
	}
	m.Submenu("Select columns", a.procColumnsMenu)
	m.Separator()
	for _, s := range [...]Speed{SpeedHigh, SpeedNormal, SpeedLow, SpeedPaused} {
		if m.Item("Update speed: " + s.Title()).Checked(a.set.Speed == s).Chosen() {
			a.setSpeed(s)
		}
	}
	m.Separator()
	if m.Item("Expand all").Chosen() {
		for i := range a.model.Rows {
			if r := &a.model.Rows[i]; r.Kind == RowGroup {
				a.model.expanded[r.Key] = struct{}{}
			}
		}
		a.dirty = true
	}
	if m.Item("Collapse all").Chosen() {
		clear(a.model.expanded)
		a.dirty = true
	}
	if !a.netmon.Available {
		m.Separator()
		if m.Item("Restart as administrator").Chosen() && relaunchAsAdmin() {
			mygo.App.Quit()
		}
	}
}

// procColumnsMenu chooses the Processes page's columns.
func (a *App) procColumnsMenu(m *ui.Menu) {
	for col := ProcCol(0); col < procColumnCount; col++ {
		if m.Item(procColumns[col].title).Checked(shown(a.set.ProcColumns, col)).Chosen() {
			a.set.ProcColumns ^= 1 << col
			a.dirty = true
			a.saveSettings()
			a.syncPerformance(true)
		}
	}
}

// detailColumnsMenu chooses the Details page's columns.
func (a *App) detailColumnsMenu(m *ui.Menu) {
	for col := DetailCol(0); col < detailColumnCount; col++ {
		if m.Item(detailColumns[col].title).Checked(shown(a.set.DetailColumns, col)).Chosen() {
			a.set.DetailColumns ^= 1 << col
			a.saveSettings()
		}
	}
}

// perfMenu is the Performance page's "…" and right-click menu.
func (a *App) perfMenu(m *ui.Menu) {
	a.cpuViewMenu(m)
	m.Separator()
	if m.Item("Summary view").Checked(a.perfSummary).Chosen() {
		a.perfSummary = !a.perfSummary
	}
	if m.Item("Copy").Shortcut(ui.Ctrl, ui.KeyC).Chosen() {
		a.copySummary()
	}
	m.Separator()
	if m.Item("Open Resource Monitor").Chosen() {
		open("resmon.exe")
	}
}

func (a *App) cpuViewMenu(m *ui.Menu) {
	cpu := a.perfTab.Kind == PerfCPU
	if m.Item("Overall utilization").Checked(!a.set.CPULogical).Disabled(!cpu).Chosen() {
		a.set.CPULogical = false
		a.saveSettings()
	}
	if m.Item("Logical processors").Checked(a.set.CPULogical).Disabled(!cpu).Chosen() {
		a.set.CPULogical = true
		a.saveSettings()
	}
	if m.Item("Show kernel times").Checked(a.set.KernelTimes).Disabled(!cpu || a.set.CPULogical).Chosen() {
		a.set.KernelTimes = !a.set.KernelTimes
		a.saveSettings()
	}
}

// goToDetails shows the Details page with pid selected.
func (a *App) goToDetails(pid uint32) {
	if p := a.sampler.ByPID(pid); p != nil {
		a.detailsSel = procKey(p)
		a.revealDet = true
	}
	a.goTo(PageDetails)
	a.query = ""
	a.dirty = true
}

func (a *App) copyMenu(m *ui.Menu) {
	if m.Item("Copy row").Shortcut(ui.Ctrl, ui.KeyC).Chosen() {
		a.copySelection()
	}
	m.Separator()
}

// copySummary puts the Performance page's figures on the clipboard, as its
// "Copy" does.
func (a *App) copySummary() {
	var b strings.Builder
	b.WriteString("CPU\r\n\t" + a.cpuInfo.Name + "\r\n")
	b.WriteString("\tUtilization\t" + fmtPercent(a.sampler.CPU*100) + "\r\n")
	b.WriteString("\tSpeed\t" + fmtGHz(a.cpuGHz()) + "\r\n")
	b.WriteString("\tProcesses\t" + itoa(len(a.sampler.Procs)) + "\r\n")
	b.WriteString("\tThreads\t" + itoa(a.sampler.Threads) + "\r\n")
	b.WriteString("\tHandles\t" + itoa(a.sampler.Handles) + "\r\n")
	b.WriteString("\tUp time\t" + fmtUptime(sys.Uptime()) + "\r\n")
	b.WriteString("Memory\r\n\tIn use\t" + fmtGB(a.mem.Used()) + "\r\n\tAvailable\t" + fmtGB(a.mem.Available) + "\r\n")
	b.WriteString("\tCommitted\t" + fmtGB(a.mem.Commit) + "/" + fmtGB(a.mem.CommitLimit) + "\r\n")
	for _, d := range a.disks.List {
		b.WriteString(d.Title() + "\r\n\tActive time\t" + fmtPercent(d.Active*100) + "\r\n\tRead speed\t" + fmtRate(d.ReadRate) + "\r\n\tWrite speed\t" + fmtRate(d.WriteRate) + "\r\n")
	}
	for _, n := range a.net.List {
		b.WriteString(n.Name + "\r\n\tSend\t" + fmtBits(n.Send) + "\r\n\tReceive\t" + fmtBits(n.Recv) + "\r\n")
	}
	for i, g := range a.gpus {
		b.WriteString("GPU " + itoa(i) + "\r\n\t" + g.Name + "\r\n\tUtilization\t" + fmtPercent(g.Use) + "\r\n")
	}
	text := b.String()
	go mygo.Clipboard.WriteText(text)
	a.toast("success", "Copied")
}

func (a *App) openRun() {
	a.runOpen, a.runFocus, a.runText = true, true, ""
}

// dialogs draws the dialogs that are open.
func (a *App) dialogs(c *ui.Context, k *colors) {
	a.runDialog(c, k)
	if a.confirmEnd {
		switch ui.AlertDialog(c, &a.confirmEnd, "Do you want to end "+a.confirmTitle+"?",
			"If an open program is associated with this process, it will close and you will lose any unsaved data. If you end a system process, it might result in system instability.",
			"Cancel", "End process") {
		case 1:
			a.endProcesses(a.confirmPIDs, a.confirmTitle)
		}
	}
	a.efficiencyDialog(c, k)
	if a.confirmUser != userNone {
		open := true
		signOut := a.confirmUser == userSignOut
		title, msg, ok := "Do you want to disconnect the selected user?", "The user's programs keep running; they can sign in again to reconnect.", "Disconnect user"
		if signOut {
			title, msg, ok = "Do you want to sign out the selected user?", "Any unsaved data of this user's may be lost.", "Sign out user"
		}
		if ui.AlertDialog(c, &open, title, msg, "Cancel", ok) == 1 && a.hasUserSel {
			a.userSession(a.userSel, signOut)
		}
		if !open {
			a.confirmUser = userNone
		}
	}
	if a.historyConfirm {
		if ui.AlertDialog(c, &a.historyConfirm, "Delete usage history?",
			"The processor time and network use counted for each app are forgotten, and counting starts again from now.",
			"Cancel", "Delete") == 1 {
			a.history.Clear()
			a.dirty = true
		}
	}
	a.affinityDialog(c, k)
}

// dialogFrame is a dialog's title and description over its body, and its
// buttons: Cancel and ok. Whether ok was chosen.
func dialogFrame(c *ui.Context, k *colors, open *bool, width float32, title, desc, okLabel string, danger bool, body func()) bool {
	chosen := false
	ui.Modal(c, open, func() {
		ui.Column(c).Width(width).Gap(12).Children(func() {
			ui.Text(c, title).FontSize(fontSubtitle).FontWeight(600).TextColor(k.text)
			if desc != "" {
				ui.Text(c, desc).FontSize(fontBody).TextColor(k.secondary)
			}
			if body != nil {
				body()
			}
			ui.Row(c).FillWidth().Gap(8).Justify(ui.End).Padding(8, 0, 0, 0).Children(func() {
				if ui.Button(c, "Cancel").MinWidth(96).Clicked() {
					*open = false
				}
				ok := ui.PrimaryButton(c, okLabel).MinWidth(96)
				if danger {
					ok.Background(k.closeHover).TextColor(white)
				}
				if ok.Clicked() {
					chosen, *open = true, false
				}
			})
		})
	})
	return chosen
}

func (a *App) runDialog(c *ui.Context, k *colors) {
	if !a.runOpen {
		return
	}
	focus := a.runFocus
	a.runFocus = false
	submitted := false
	ok := dialogFrame(c, k, &a.runOpen, 480, "Create new task", "Type the name of a program, folder, document, or Internet resource, and Windows will open it for you.", "OK", false, func() {
		ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Open:").FontSize(fontBody).TextColor(k.text).NoWrap()
			in := ui.TextInput(c, &a.runText).Grow(1).MinWidth(0).Placeholder("notepad").Label("Open")
			if focus {
				in.AutoFocus()
			}
			if in.Submitted() {
				submitted = true
			}
			if ui.Button(c, "Browse").Clicked() {
				a.browse()
			}
		})
		ui.Checkbox(c, &a.runAdmin, "Create this task with administrative privileges")
	})
	if ok || submitted {
		a.runTask(a.runText, a.runAdmin)
		a.runOpen = false
	}
}

func (a *App) efficiencyDialog(c *ui.Context, k *colors) {
	if !a.confirmEfficiency {
		return
	}
	if dialogFrame(c, k, &a.confirmEfficiency, 460, "Turn on Efficiency mode?",
		"Efficiency mode will lower process priority and improve power efficiency. This might cause instability for some processes.",
		"Turn on Efficiency mode", false, func() {
			ask := !a.set.AskEfficiency
			if ui.Checkbox(c, &ask, "Don't show again").Changed() {
				a.set.AskEfficiency = !ask
				a.saveSettings()
			}
		}) {
		a.setEfficiency(a.confirmPIDs, true)
	}
}

// affinityDialog is Set affinity: which processors the process may run on.
func (a *App) affinityDialog(c *ui.Context, k *colors) {
	if !a.affinityOpen {
		return
	}
	n := min(64, int(a.cpuInfo.Logical))
	all := ^uint64(0)
	if n < 64 {
		all = 1<<n - 1
	}
	if dialogFrame(c, k, &a.affinityOpen, 480, "Processor affinity", "Which processors is "+a.confirmTitle+" allowed to run on?", "OK", false, func() {
		every := a.affinityMask&all == all
		if ui.Checkbox(c, &every, "<All Processors>").Changed() {
			if every {
				a.affinityMask = all
			} else {
				a.affinityMask = 0
			}
		}
		ui.Column(c).FillWidth().Gap(4).PaddingStart(24).Children(func() {
			for i := 0; i < n; i += 4 {
				ui.Row(c).FillWidth().Gap(8).Children(func() {
					for j := i; j < i+4; j++ {
						ui.Row(c.Key(j)).Grow(1).Basis(0).Children(func() {
							if j < n {
								bit := uint64(1) << j
								on := a.affinityMask&bit != 0
								if ui.Checkbox(c, &on, "CPU "+itoa(j)).Changed() {
									a.affinityMask ^= bit
								}
							}
						})
					}
				})
			}
		})
	}) {
		a.setAffinity(a.affinityPID, a.affinityMask)
	}
}
