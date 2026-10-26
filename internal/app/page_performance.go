package app

import (
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// The Performance page: down the left, a small graph and the figures of
// each part of the machine (CPU, memory, each disk, each network adapter,
// each GPU); on the right, the chosen one's graphs and details.

func (a *App) performancePage(c *ui.Context, k *colors) {
	pageHeader(c, k, "Performance", func() {
		compact := a.narrowerThan(c, 820)
		if a.perfTab.Kind == PerfCPU {
			commandButton(c, k, command{icon: icView, label: "CPU view", enabled: true, compact: compact, menu: a.cpuViewMenu})
		}
		if commandButton(c, k, command{icon: icRun, label: "Run new task", enabled: true, compact: compact}) {
			a.openRun()
		}
		commandButton(c, k, command{icon: icMore, enabled: true, menu: a.perfMenu})
	})
	ui.Box(c).FillWidth().Height(1).Background(k.divider).Shrink(0)
	ui.Row(c).FillWidth().Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		a.perfSidebar(c, k, false)
		ui.Box(c).Width(1).Background(k.divider).Shrink(0)
		right := ui.Scroll(c.Key("perf-detail")).Grow(1).MinWidth(0).FillHeight().ContextMenu(a.perfMenu)
		// A double click on the graphs: the summary view.
		if right.DoubleClicked() {
			a.perfSummary = true
		}
		right.Children(func() {
			ui.Column(c).FillWidth().Padding(8, pagePad, 24, pagePad).Gap(6).Children(func() {
				t := a.perfTab
				switch {
				case t.Kind == PerfDisk && t.Index < len(a.disks.List):
					a.diskDetail(c, k, a.disks.List[t.Index])
				case t.Kind == PerfNet && t.Index < len(a.net.List):
					a.netDetail(c, k, a.net.List[t.Index])
				case t.Kind == PerfGPU && t.Index < len(a.gpus):
					a.gpuDetail(c, k, t.Index)
				case t.Kind == PerfMemory:
					a.memoryDetail(c, k)
				default:
					a.perfTab = PerfTab{}
					a.cpuDetail(c, k)
				}
			})
		})
	})
}

// perfSummaryView is the summary view: the parts' list alone, over the
// whole window; a double click (or Esc) goes back.
func (a *App) perfSummaryView(c *ui.Context, k *colors) {
	ui.Column(c).FillWidth().Grow(1).MinHeight(0).ContextMenu(a.perfMenu).Children(func() {
		a.perfSidebar(c, k, true)
	})
}

func (a *App) perfSidebar(c *ui.Context, k *colors, whole bool) {
	w, _ := c.Size()
	room := w - a.navWidth(w)
	col := ui.Scroll(c.Key("perf-sidebar")).FillHeight().Shrink(0)
	if whole {
		col.FillWidth()
	} else {
		col.Width(float32(math.Round(float64(min(max(room*0.27, 192), 260)))))
	}
	narrow := room < 720
	col.Children(func() {
		ui.Column(c).FillWidth().Padding(8, 10, 12, pagePad-12).Gap(4).Children(func() {
			// CPU.
			ghz := a.cpuGHz()
			sub := fmtPercent(a.sampler.CPU * 100)
			if ghz > 0 {
				sub += "  " + fmtGHz(ghz)
			}
			a.perfItem(c, k, PerfTab{Kind: PerfCPU}, "CPU", sub, []series{{ring: &a.histCPU}}, 100, k.cpu, narrow)
			// Memory.
			memSub := trimGB(a.mem.Used()) + "/" + trimGB(a.mem.Total) + " (" + fmtPercent(a.mem.Load()*100) + ")"
			a.perfItem(c, k, PerfTab{Kind: PerfMemory}, "Memory", memSub, []series{{ring: &a.histMem}}, float32(max(1, a.mem.Total)), k.memory, narrow)
			for i, d := range a.disks.List {
				a.perfItem(c, k, PerfTab{Kind: PerfDisk, Index: i}, d.Title(), d.KindTitle()+"\n"+fmtPercent(d.Active*100), []series{{ring: &d.HistActive}}, 100, k.disk, narrow)
			}
			for i, n := range a.net.List {
				// The adapter's name under its kind, unless they're the same.
				name := ""
				if n.Name != n.Kind() {
					name = n.Name + "\n"
				}
				sub := name + "Not connected"
				if n.Connected {
					sub = name + "S: " + fmtBits(n.Send) + " R: " + fmtBits(n.Recv)
				}
				peak := max(n.HistSend.Max(), n.HistRecv.Max())
				a.perfItem(c, k, PerfTab{Kind: PerfNet, Index: i}, n.Kind(), sub, []series{{ring: &n.HistRecv}, {ring: &n.HistSend, dashed: true}}, niceMax(peak, 100e3), k.network, narrow)
			}
			for i, g := range a.gpus {
				a.perfItem(c, k, PerfTab{Kind: PerfGPU, Index: i}, "GPU "+itoa(i), g.Name+"\n"+fmtPercent(g.Use), []series{{ring: &g.Hist}}, 100, k.gpu, narrow)
			}
		})
	})
}

func trimGB(n uint64) string { return fmtGB(n) }

func (a *App) perfItem(c *ui.Context, k *colors, tab PerfTab, title, sub string, ss []series, top float32, color ui.Color, narrow bool) {
	on := a.perfTab == tab
	b := ui.ButtonBase(c.Key(tab)).FillWidth().MinHeight(64).Justify(ui.Start).Gap(12).Padding(8, 8, 8, 10).Radius(6).FocusRing(false)
	switch {
	case on:
		b.Background(k.navSelected)
	case b.Pressed():
		b.Background(k.pressed)
	case b.Hovered():
		b.Background(k.hover)
	}
	if b.Clicked() {
		if a.perfTab != tab {
			a.perfTab = tab
			if tab.Kind == PerfNet {
				a.requestAddresses()
			}
		}
		if b.DoubleClicked() {
			a.perfSummary = !a.perfSummary
		}
	}
	b.Tooltip(title + "\n" + sub)
	b.Children(func() {
		if on {
			ui.Box(c).Absolute().Left(0).Top(20).Size(3, 24).Radius(1.5).Background(k.accent)
		}
		gw := float32(64)
		if narrow {
			gw = 52
		}
		graph(c, k, ss, graphOptions{width: gw, height: 46, color: color, max: top, mini: true})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Clip().Children(func() {
			ui.Text(c, title).FontSize(fontBody).FontWeight(600).TextColor(k.text).SingleLine()
			for line := range strings.SplitSeq(sub, "\n") {
				ui.Text(c, line).FontSize(fontSmall).TextColor(k.secondary).SingleLine()
			}
		})
	})
}

// detailTitle is the part's name, and its model on the right.
func detailTitle(c *ui.Context, k *colors, name, model string) {
	ui.Row(c).FillWidth().AlignItems(ui.End).Gap(16).Padding(2, 0, 6, 0).Shrink(0).Children(func() {
		ui.Text(c, name).FontSize(fontTitle).FontWeight(600).TextColor(k.text).NoWrap()
		ui.Spacer(c)
		ui.Row(c).MinWidth(0).Shrink(1).Justify(ui.End).Clip().Children(func() {
			ui.Text(c, model).FontSize(fontBody).FontWeight(600).TextColor(k.text).SingleLine()
		})
	})
}

// mainHeight is the main graph's height: half the room, within limits.
func mainHeight(c *ui.Context) float32 {
	_, h := c.Size()
	return float32(math.Round(float64(min(max((h-300)*0.55, 140), 420))))
}

// figures lays out the figures under the graphs: big ones on the left, a
// list on the right, stacked when the pane is narrow.
func (a *App) figures(c *ui.Context, left, right func()) {
	w, _ := c.Size()
	pane := w - a.navWidth(w) - 280
	r := ui.Row(c).FillWidth().Gap(24).Padding(12, 0, 0, 0)
	if pane < 520 {
		r.Column()
	}
	r.Children(func() {
		ui.Column(c).Gap(4).Shrink(0).Children(left)
		ui.Column(c).Grow(1).MinWidth(0).Children(right)
	})
}

func statRow(c *ui.Context, fn func()) {
	ui.Row(c).Children(fn)
}

func (a *App) cpuDetail(c *ui.Context, k *colors) {
	info := &a.cpuInfo
	detailTitle(c, k, "CPU", info.Name)
	label := "% Utilization"
	if a.set.CPULogical {
		label = "% Utilization over 60 seconds"
	}
	graphLabels(c, k, label, "100%")
	gh := mainHeight(c)
	if a.set.CPULogical && len(a.cores) > 1 {
		// A graph per logical processor, in a grid.
		n := len(a.cores)
		cols := logicalColumns(n)
		rows := (n + cols - 1) / cols
		cellH := float32(math.Round(float64(max(40, (gh-float32(rows-1)*4)/float32(rows)))))
		ui.Column(c).FillWidth().Gap(4).Children(func() {
			for i := 0; i < n; i += cols {
				ui.Row(c).FillWidth().Gap(4).Children(func() {
					for j := i; j < i+cols; j++ {
						ui.Column(c.Key(j)).Grow(1).Basis(0).MinWidth(0).Label("Logical processor " + itoa(j)).Children(func() {
							if j < n {
								graph(c, k, []series{{ring: &a.cores[j]}}, graphOptions{height: cellH, color: k.cpu, max: 100, grid: true, samples: a.samples})
							}
						})
					}
				})
			}
		})
	} else {
		ss := []series{{ring: &a.histCPU}}
		if a.set.KernelTimes {
			ss = append(ss, series{ring: &a.histKernel, kernel: true})
		}
		graph(c, k, ss, graphOptions{height: gh, color: k.cpu, max: 100, grid: true, samples: a.samples})
	}
	graphLabels(c, k, "60 seconds", "0")
	a.figures(c, func() {
		statRow(c, func() {
			stat(c, k, "Utilization", fmtPercent(a.sampler.CPU*100))
			stat(c, k, "Speed", fmtGHz(a.cpuGHz()))
		})
		statRow(c, func() {
			stat(c, k, "Processes", itoa(len(a.sampler.Procs)))
			stat(c, k, "Threads", itoa(a.sampler.Threads))
			stat(c, k, "Handles", itoa(a.sampler.Handles))
		})
		statRow(c, func() {
			stat(c, k, "Up time", fmtUptime(sys.Uptime()))
		})
	}, func() {
		kv(c, k, "Base speed:", fmtGHz(float32(info.BaseMHz)/1000))
		kv(c, k, "Sockets:", itoa(info.Sockets))
		kv(c, k, "Cores:", itoa(info.Cores))
		kv(c, k, "Logical processors:", itoa(info.Logical))
		virt := "Disabled"
		if info.Virtualization {
			virt = "Enabled"
		}
		kv(c, k, "Virtualization:", virt)
		kv(c, k, "L1 cache:", fmtCache(info.L1))
		kv(c, k, "L2 cache:", fmtCache(info.L2))
		kv(c, k, "L3 cache:", fmtCache(info.L3))
	})
}

// Prefer a complete balanced rectangle (12 processors: 4 by 3), allowing
// a partial last row for processor counts with no suitable factors.
func logicalColumns(n int) int {
	best := 1
	target := math.Sqrt(float64(n) * 4 / 3)
	for cols := 1; cols <= n; cols++ {
		if n%cols == 0 && math.Abs(float64(cols)-target) < math.Abs(float64(best)-target) {
			best = cols
		}
	}
	if n/best > best*2 {
		return int(math.Ceil(target))
	}
	return best
}

// compressedBytes is what the memory compression store holds: the
// "Memory Compression" process's working set.
func (a *App) compressedBytes() uint64 {
	for i := range a.sampler.Procs {
		if a.sampler.Procs[i].Info.Name == "Memory Compression" {
			return a.sampler.Procs[i].WorkingSet
		}
	}
	return 0
}

func (a *App) memoryDetail(c *ui.Context, k *colors) {
	m := a.mem
	installed := a.memHW.Installed
	if installed == 0 {
		installed = m.Total
	}
	detailTitle(c, k, "Memory", fmtGB(installed))
	graphLabels(c, k, "Memory usage", fmtGB(m.Total))
	graph(c, k, []series{{ring: &a.histMem}}, graphOptions{height: float32(math.Round(float64(mainHeight(c) * 0.78))), color: k.memory, max: float32(max(1, m.Total)), grid: true, samples: a.samples})
	graphLabels(c, k, "60 seconds", "0")
	ui.Row(c).FillWidth().Height(24).Padding(4, 0, 0, 0).AlignItems(ui.End).Children(func() {
		ui.Text(c, "Memory composition").FontSize(fontSmall).TextColor(k.secondary).NoWrap()
	})
	composition(c, k, m)
	a.figures(c, func() {
		statRow(c, func() {
			if cb := a.compressedBytes(); cb > 0 {
				stat(c, k, "In use (Compressed)", fmtGB(m.Used())+" ("+fmtMBShort(cb)+")")
			} else {
				stat(c, k, "In use", fmtGB(m.Used()))
			}
			stat(c, k, "Available", fmtGB(m.Available))
		})
		statRow(c, func() {
			stat(c, k, "Committed", fmtGB(m.Commit)+"/"+fmtGB(m.CommitLimit))
			stat(c, k, "Cached", fmtGB(m.Cached))
		})
		statRow(c, func() {
			stat(c, k, "Paged pool", fmtBytes(m.PagedPool))
			stat(c, k, "Non-paged pool", fmtBytes(m.NonpagedPool))
		})
	}, func() {
		hw := a.memHW
		if hw.Speed > 0 {
			kv(c, k, "Speed:", itoa(hw.Speed)+" MT/s")
		}
		if hw.Slots > 0 {
			kv(c, k, "Slots used:", itoa(hw.SlotsUsed)+" of "+itoa(hw.Slots))
		}
		if hw.Form != "" {
			kv(c, k, "Form factor:", hw.Form)
		}
		if installed > m.Total {
			kv(c, k, "Hardware reserved:", itoa((installed-m.Total)>>20)+" MB")
		}
	})
}

// composition is the memory's parts as a bar: in use, cached, free.
func composition(c *ui.Context, k *colors, m sys.Memory) {
	total := float32(max(1, m.Total))
	used := float32(m.Used()) / total
	cached := min(float32(m.Cached)/total, 1-used)
	usedFill := k.memory.Alpha(0.3)
	if k.dark {
		usedFill = k.memory.Alpha(0.45)
	}
	ui.Row(c).FillWidth().Height(40).Border(1, k.memory).Background(k.graphBG).Clip().Shrink(0).Children(func() {
		ui.Box(c).Grow(used).Basis(0).FillHeight().Background(usedFill)
		ui.Box(c).Width(1).FillHeight().Background(k.memory).Shrink(0)
		ui.Box(c).Grow(cached).Basis(0).FillHeight().Background(k.memory.Alpha(0.14))
		ui.Box(c).Width(1).FillHeight().Background(k.memory.Alpha(0.6)).Shrink(0)
		ui.Box(c).Grow(max(0.001, 1-used-cached)).Basis(0).FillHeight()
	})
}

func (a *App) diskDetail(c *ui.Context, k *colors, d *sys.Disk) {
	detailTitle(c, k, d.Title(), d.Model)
	graphLabels(c, k, "Active time", "100%")
	graph(c, k, []series{{ring: &d.HistActive}}, graphOptions{height: float32(math.Round(float64(mainHeight(c) * 0.62))), color: k.disk, max: 100, grid: true, samples: a.samples})
	graphLabels(c, k, "60 seconds", "0")
	top := niceMax(max(d.HistRead.Max(), d.HistWrite.Max()), 1<<20)
	graphLabels(c, k, "Disk transfer rate", fmtRate(top))
	graph(c, k, []series{{ring: &d.HistRead}, {ring: &d.HistWrite, dashed: true}}, graphOptions{height: float32(math.Round(float64(mainHeight(c) * 0.4))), color: k.disk, max: top, grid: true, samples: a.samples})
	spanLegend(c, k, k.disk, "Read speed", "Write speed")
	a.figures(c, func() {
		statRow(c, func() {
			stat(c, k, "Active time", fmtPercent(d.Active*100))
			stat(c, k, "Average response time", string(fixed(nil, float64(d.ResponseMS), 1))+" ms")
		})
		statRow(c, func() {
			stat(c, k, "Read speed", fmtRate(d.ReadRate))
			stat(c, k, "Write speed", fmtRate(d.WriteRate))
		})
	}, func() {
		kv(c, k, "Capacity:", fmtBytes(d.Capacity))
		if d.Formatted > 0 {
			kv(c, k, "Formatted:", fmtBytes(d.Formatted))
		}
		kv(c, k, "System disk:", yesNo(d.System))
		kv(c, k, "Page file:", yesNo(d.Pagefile))
		if d.Kind != "" {
			kv(c, k, "Type:", d.KindTitle())
		}
	})
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// spanLegend is "60 seconds", the legend, and "0", under a two-series graph.
func spanLegend(c *ui.Context, k *colors, color ui.Color, first, second string) {
	ui.Row(c).FillWidth().Height(20).AlignItems(ui.Center).Shrink(0).Children(func() {
		ui.Text(c, "60 seconds").FontSize(fontSmall).TextColor(k.secondary).NoWrap()
		ui.Spacer(c)
		legend(c, k, color, false, first)
		legend(c, k, color, true, second)
		ui.Text(c, "0").FontSize(fontSmall).TextColor(k.secondary).NoWrap()
	})
}

func (a *App) netDetail(c *ui.Context, k *colors, n *sys.Adapter) {
	detailTitle(c, k, n.Kind(), n.Description)
	top := niceMax(max(n.HistSend.Max(), n.HistRecv.Max()), 100e3)
	graphLabels(c, k, "Throughput", fmtBits(top))
	graph(c, k, []series{{ring: &n.HistRecv}, {ring: &n.HistSend, dashed: true}}, graphOptions{height: mainHeight(c), color: k.network, max: top, grid: true, samples: a.samples})
	spanLegend(c, k, k.network, "Receive", "Send")
	a.figures(c, func() {
		statRow(c, func() {
			stat(c, k, "Send", fmtBits(n.Send))
			stat(c, k, "Receive", fmtBits(n.Recv))
		})
	}, func() {
		kv(c, k, "Adapter name:", n.Name)
		kv(c, k, "Connection type:", n.Kind())
		if n.LinkSpeed > 0 {
			kv(c, k, "Link speed:", fmtBits(float32(n.LinkSpeed)))
		}
		status := "Not connected"
		if n.Connected {
			status = "Connected"
		}
		kv(c, k, "Status:", status)
		if n.IPv4 != "" {
			kv(c, k, "IPv4 address:", n.IPv4)
		}
		if n.IPv6 != "" {
			kv(c, k, "IPv6 address:", n.IPv6)
		}
	})
}

var engineNames = [sys.EngineKinds]string{"3D", "Copy", "Video Encode", "Video Decode"}

func (a *App) gpuDetail(c *ui.Context, k *colors, index int) {
	g := a.gpus[index]
	detailTitle(c, k, "GPU "+itoa(index), g.Name)
	eh := float32(math.Round(float64(min(max(mainHeight(c)*0.45, 90), 200))))
	for row := range 2 {
		ui.Row(c).FillWidth().Gap(16).Children(func() {
			for col := range 2 {
				i := row*2 + col
				ui.Column(c.Key(engineNames[i])).Grow(1).Basis(0).MinWidth(0).Gap(4).Padding(0, 0, 8, 0).Children(func() {
					graphLabels(c, k, engineNames[i], fmtPercent(g.Engines[i]))
					graph(c, k, []series{{ring: &g.HistEngines[i]}}, graphOptions{height: eh, color: k.gpu, max: 100, grid: true, samples: a.samples})
				})
			}
		})
	}
	graphLabels(c, k, "Dedicated GPU memory usage", fmtGB(g.DedicatedTotal))
	graph(c, k, []series{{ring: &g.HistDedicated}}, graphOptions{height: 60, color: k.gpu, max: float32(max(1, g.DedicatedTotal)), grid: true, samples: a.samples})
	graphLabels(c, k, "Shared GPU memory usage", fmtGB(g.SharedTotal))
	graph(c, k, []series{{ring: &g.HistShared}}, graphOptions{height: 60, color: k.gpu, max: float32(max(1, g.SharedTotal)), grid: true, samples: a.samples})
	a.figures(c, func() {
		statRow(c, func() {
			stat(c, k, "Utilization", fmtPercent(g.Use))
			stat(c, k, "Dedicated GPU memory", fmtGB(g.Dedicated)+"/"+fmtGB(g.DedicatedTotal))
		})
		statRow(c, func() {
			stat(c, k, "Shared GPU memory", fmtGB(g.Shared)+"/"+fmtGB(g.SharedTotal))
			stat(c, k, "GPU Memory", fmtGB(g.Dedicated+g.Shared)+"/"+fmtGB(g.DedicatedTotal+g.SharedTotal))
		})
	}, func() {
		if g.Driver[0] != 0 {
			kv(c, k, "Driver version:", itoa(g.Driver[0])+"."+itoa(g.Driver[1])+"."+itoa(g.Driver[2])+"."+itoa(g.Driver[3]))
		}
		kv(c, k, "Dedicated memory:", fmtGB(g.DedicatedTotal))
		if !a.gpuWarm {
			kv(c, k, "Status:", "Collecting…")
		}
	})
}
