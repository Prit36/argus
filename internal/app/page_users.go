package app

import (
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// The Users page: each signed-in user, their session's status, and what
// their processes use together; a user opens to their programs. A user can
// be disconnected or signed out, and a program of theirs ended.

var userCols = [...]column{
	{title: "User"},
	{title: "Status", width: 136},
	{title: "CPU", width: 80, right: true},
	{title: "Memory", width: 104, right: true},
	{title: "Disk", width: 88, right: true},
	{title: "Network", width: 88, right: true},
}

type userSum struct {
	cpu        float32
	memory     uint64
	disk       float32
	net        float32
	netUnknown bool
	count      int
}

func (a *App) usersPage(c *ui.Context, k *colors) {
	pageHeader(c, k, "Users", func() {
		compact := a.narrowerThan(c, 820)
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		separator(c, k)
		if commandButton(c, k, command{icon: icEndTask, label: "End task", enabled: a.userProc() != nil, compact: compact}) {
			a.endUserProc()
		}
		if commandButton(c, k, command{icon: icDismiss, label: "Disconnect", enabled: a.hasUserSel, compact: compact}) {
			a.confirmUser = userDisconnect
		}
		if commandButton(c, k, command{icon: icUser, label: "Sign out", enabled: a.hasUserSel, compact: compact}) {
			a.confirmUser = userSignOut
		}
		if commandButton(c, k, command{icon: icSettings, label: "Manage user accounts", enabled: true, compact: compact}) {
			open("ms-settings:otherusers")
		}
	})
	cols := userCols
	_, header := tableHeader(c, k, cols[:], -1, true, compactHeaderH, 760, false, nil)
	totalMem := float32(max(1, a.mem.Total))
	if len(a.userRows) == 0 && a.query != "" {
		empty(c, k, noResults(a.query))
		return
	}
	a.usersList.Key = a.userRowKey
	a.usersList.KeepOffset = true
	a.tableList(c, "users-table", &a.usersList, len(a.userDisplay), 760, header, func(lr ui.ListRow) {
		i := a.userDisplay[lr.Index]
		if i < 0 {
			s := &a.sessions[-1-i]
			sum := a.userSums[s.ID]
			a.userRow(lr.Context, k, s, &sum, cols[:], totalMem)
		} else {
			a.userProgram(lr.Context, k, &a.sampler.Procs[i], cols[:], totalMem)
		}
	})
}

type userRowKey struct {
	session uint32
	process uint64
	child   bool
}

func (a *App) userRowKey(i int) any {
	r := a.userDisplay[i]
	if r < 0 {
		return userRowKey{session: a.sessions[-1-r].ID}
	}
	p := &a.sampler.Procs[r]
	return userRowKey{session: p.Session, process: procKey(p), child: true}
}

func accountMatches(s *sys.Session, q string) bool {
	return containsFold(s.User, q) || containsFold(s.Full, q) || containsFold(s.Domain, q)
}

// Build aggregates and the program order once per data/search change.
func (a *App) buildUsers() {
	if a.userSums == nil {
		a.userSums = make(map[uint32]userSum)
	}
	clear(a.userSums)
	a.userOrder = a.userOrder[:0]
	q := strings.TrimSpace(a.query)
	needOrder := q != ""
	if !needOrder {
		for i := range a.sessions {
			if a.usersOpen[a.sessions[i].ID] {
				needOrder = true
				break
			}
		}
	}
	for i := range a.sampler.Procs {
		p := &a.sampler.Procs[i]
		sum := a.userSums[p.Session]
		sum.cpu += p.CPU
		sum.memory += p.PrivateWS
		sum.disk += p.IO()
		sum.net += p.Net
		sum.netUnknown = sum.netUnknown || !p.NetKnown
		sum.count++
		a.userSums[p.Session] = sum
		if needOrder {
			a.userOrder = append(a.userOrder, int32(i))
		}
	}
	slices.SortFunc(a.userOrder, func(x, y int32) int {
		p, q := &a.sampler.Procs[x], &a.sampler.Procs[y]
		if p.Session != q.Session {
			return cmp3(p.Session, q.Session)
		}
		if p.CPU != q.CPU {
			return -cmp3(p.CPU, q.CPU)
		}
		if p.PrivateWS != q.PrivateWS {
			return -cmp3(p.PrivateWS, q.PrivateWS)
		}
		return cmp3(p.PID, q.PID)
	})
	a.userRows = a.userRows[:0]
	a.userDisplay = a.userDisplay[:0]
	for i := range a.sessions {
		s := &a.sessions[i]
		accountMatch := accountMatches(s, q)
		match := accountMatch
		start := 0
		if needOrder {
			start, _ = slices.BinarySearchFunc(a.userOrder, s.ID, func(pi int32, id uint32) int {
				return cmp3(a.sampler.Procs[pi].Session, id)
			})
		}
		if !match {
			for _, pi := range a.userOrder[start:] {
				p := &a.sampler.Procs[pi]
				if p.Session != s.ID {
					break
				}
				if detailMatches(p, q) {
					match = true
					break
				}
			}
		}
		if match {
			a.userRows = append(a.userRows, int32(i))
			a.userDisplay = append(a.userDisplay, -1-int32(i))
			if !a.usersOpen[s.ID] && q == "" {
				continue
			}
			// Processes are already ordered by session, then activity.
			for _, pi := range a.userOrder[start:] {
				p := &a.sampler.Procs[pi]
				if p.Session != s.ID {
					break
				}
				if accountMatch || detailMatches(p, q) {
					a.userDisplay = append(a.userDisplay, pi)
				}
			}
		}
	}
}

func (a *App) toggleUser(id uint32) {
	a.usersOpen[id] = !a.usersOpen[id]
	a.dirty = true
}

func (a *App) userRow(c *ui.Context, k *colors, s *sys.Session, sum *userSum, cols []column, totalMem float32) {
	sel := a.hasUserSel && a.userSel == s.ID && a.userProcSel == 0
	id := s.ID
	row := ui.ButtonBase(c).FillWidth().Height(rowH+6).Justify(ui.Start).AlignItems(ui.Stretch).Padding(0, 14, 0, pagePad-10).Dividers(1, k.cellDivider).FocusRing(false)
	selectionOutline(row, k, sel)
	hovered := row.Hovered()
	switch {
	case sel:
		row.Background(k.rowSelected)
	case hovered:
		row.Background(k.rowHover)
	}
	if row.Clicked() {
		a.userSel, a.hasUserSel, a.userProcSel = id, true, 0
		if row.DoubleClicked() {
			a.toggleUser(id)
		}
	}
	row.ContextMenu(func(m *ui.Menu) {
		a.userSel, a.hasUserSel, a.userProcSel = id, true, 0
		a.userMenu(m)
	})
	row.Children(func() {
		cell(c, &cols[0]).Gap(8).Children(func() {
			o := ui.ButtonBase(c.Key("open")).Size(20, 20).Radius(4).Shrink(0).FocusRing(false)
			if o.Hovered() {
				o.Background(k.hover)
			}
			if o.Clicked() {
				a.toggleUser(id)
			}
			ic := icChevronRight
			if a.usersOpen[id] || strings.TrimSpace(a.query) != "" {
				ic = icChevronDown
			}
			o.Children(func() { icon(c, ic, 11, k.secondary) })
			ui.Row(c).Size(24, 24).Center().Radius(12).Background(k.accent.Alpha(0.18)).Shrink(0).Children(func() {
				icon(c, icUser, 14, k.accent)
			})
			name := s.User
			if a.set.FullAccountName && s.Full != "" {
				name = s.Full + " (" + s.User + ")"
			}
			ui.Text(c, name+" ("+itoa(sum.count)+")").FontSize(fontCaption).FontWeight(600).TextColor(k.text).SingleLine()
		})
		status := ""
		if !s.Active {
			status = "Disconnected"
		}
		textCell(c, &cols[1], status, k.secondary)
		heatCell(c, k, &cols[2], cpuCell(sum.cpu), sum.cpu/50, sel, hovered)
		heatCell(c, k, &cols[3], mbCell(sum.memory), float32(sum.memory)/totalMem*2, sel, hovered)
		heatCell(c, k, &cols[4], diskCell(sum.disk), sum.disk/(50<<20), sel, hovered)
		net := ""
		if a.netmon.Available {
			net = netCell(sum.net)
			if sum.netUnknown {
				net = "-"
			}
		}
		heatCell(c, k, &cols[5], net, sum.net*8/50e6, sel, hovered)
	})
}

// userProgram draws one visible process under its account.
func (a *App) userProgram(c *ui.Context, k *colors, p *sys.Proc, cols []column, totalMem float32) {
	session := p.Session
	key := procKey(p)
	sel := a.userProcSel == key
	row := ui.ButtonBase(c.Key(key)).FillWidth().Height(rowH).Justify(ui.Start).AlignItems(ui.Stretch).Padding(0, 14, 0, pagePad-10).Dividers(1, k.cellDivider).FocusRing(false)
	selectionOutline(row, k, sel)
	hovered := row.Hovered()
	switch {
	case sel:
		row.Background(k.rowSelected)
	case hovered:
		row.Background(k.rowHover)
	}
	if row.Clicked() {
		a.userSel, a.hasUserSel, a.userProcSel = session, true, key
	}
	row.ContextMenu(func(m *ui.Menu) {
		a.userSel, a.hasUserSel, a.userProcSel = session, true, key
		a.userProcMenu(m)
	})
	row.Children(func() {
		// Under the user's name, past the chevron and the avatar.
		cell(c, &cols[0]).Gap(8).PaddingStart(10 + 20 + 8).Children(func() {
			a.exeIcon(c, p.Info.Exe)
			ui.Text(c, p.Info.Title()).FontSize(fontCaption).TextColor(k.text).SingleLine()
		})
		switch {
		case p.Hung:
			textCell(c, &cols[1], "Not responding", k.danger)
		case p.Suspended:
			textCell(c, &cols[1], "Suspended", k.secondary)
		default:
			textCell(c, &cols[1], "", k.secondary)
		}
		heatCell(c, k, &cols[2], cpuCell(p.CPU), p.CPU/50, sel, hovered)
		heatCell(c, k, &cols[3], mbCell(p.PrivateWS), float32(p.PrivateWS)/totalMem*4, sel, hovered)
		heatCell(c, k, &cols[4], diskCell(p.IO()), p.IO()/(50<<20), sel, hovered)
		net := ""
		if a.netmon.Available {
			net = netCell(p.Net)
			if !p.NetKnown {
				net = "-"
			}
		}
		heatCell(c, k, &cols[5], net, p.Net*8/50e6, sel, hovered)
	})
}

// userProc is the selected program, under its user.
func (a *App) userProc() *sys.Proc {
	if a.userProcSel == 0 {
		return nil
	}
	return a.procByKey(a.userProcSel)
}

func (a *App) endUserProc() {
	p := a.userProc()
	if p == nil {
		return
	}
	pids := []uint32{p.PID}
	if a.set.AskEndTask || a.critical(p.PID) || p.Session == 0 {
		a.confirmProcesses(pids, p.Info.Title())
		return
	}
	a.endProcesses(pids, p.Info.Title())
}
