package app

import (
	"math"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// Task Manager's graphs: the last minute of a figure, newest at the right,
// as a thin line over a lightly filled area. Neutral grids and borders
// keep the chart's structure separate from its series color.

type series struct {
	ring *sys.Ring
	// dashed: a dashed line, no fill (the second series: writes, sends).
	dashed bool
	// kernel: a deeper area under a thin line (the CPU's kernel time).
	kernel bool
}

type graphOptions struct {
	// width in DIPs; 0 fills the row.
	width, height float32
	color         ui.Color
	// max is the value at the top.
	max float32
	// grid: lines, columns scrolling with samples (where samples are).
	grid    bool
	samples uint64
	// mini: a small graph (the sidebar's): thinner lines, no grid.
	mini bool
}

// graph draws series in a box.
func graph(c *ui.Context, k *colors, ss []series, o graphOptions) {
	h := float32(math.Round(float64(o.height)))
	e := ui.Box(c).Height(h).Shrink(0)
	if o.width > 0 {
		e.Width(o.width)
	} else {
		e.FillWidth()
	}
	bg := k.graphBG
	gridColor := ui.Hex("#e7e7e7")
	fill := o.color.Alpha(0.14)
	deep := o.color.Alpha(0.34)
	edge := ui.Hex("#a6a6a6")
	if k.dark {
		gridColor, fill, deep, edge = ui.Hex("#2c2c2c"), o.color.Alpha(0.18), o.color.Alpha(0.4), ui.Hex("#757575")
	}
	if o.mini {
		edge = o.color.Alpha(0.7)
	}
	// The series are copied: the drawing may run after the view returns.
	var ringsBuf [2]series
	rings := append(ringsBuf[:0], ss...)
	e.Draw(func(p *ui.Painter, r ui.Rect) {
		w := float32(math.Round(float64(r.W)))
		if w <= 2 {
			return
		}
		p.Fill(r, bg, 0)
		if o.grid && !o.mini {
			// Ten utilization bands, including the logical-processor tiles;
			// columns six samples apart, moving with time.
			n := 10
			for i := 1; i < n; i++ {
				y := r.Y + float32(math.Round(float64(h*float32(i)/float32(n)))) + 0.5
				p.Line(r.X, y, r.X+w, y, 1, gridColor)
			}
			step := w / 10
			shift := float32(o.samples%6) / 6 * step
			for x := w - shift; x > 0; x -= step {
				xr := r.X + float32(math.Round(float64(x))) + 0.5
				p.Line(xr, r.Y, xr, r.Y+h, 1, gridColor)
			}
		}
		top := o.max
		if top <= 0 {
			top = 1
		}
		dx := w / float32(sys.HistoryLen-1)
		lineW := float32(1)
		var pts [sys.HistoryLen][2]float32
		for si, s := range rings {
			n := 0
			for i := range sys.HistoryLen {
				if !s.ring.Has(i) {
					continue
				}
				v := min(max(s.ring.At(i)/top, 0), 1)
				pts[n] = [2]float32{r.X + float32(i)*dx, r.Y + h - v*(h-1) - 0.5}
				n++
			}
			if n < 2 {
				continue
			}
			line := pts[:n]
			switch {
			case s.kernel:
				p.FillPath(area(line, r.Y+h), deep)
				p.StrokePolyline(line, 1, o.color.Alpha(0.8))
			case s.dashed:
				dashedPolyline(p, line, lineW, o.color)
			default:
				if si == 0 {
					p.FillPath(area(line, r.Y+h), fill)
				}
				p.StrokePolyline(line, lineW, o.color)
			}
		}
		// The edge, over all.
		p.Stroke(ui.Rect{X: r.X, Y: r.Y, W: w, H: h}, edge, 0, 1)
	})
}

// area is the region under a line, down to bottom.
func area(line [][2]float32, bottom float32) *ui.Path {
	var path ui.Path
	path.MoveTo(line[0][0], bottom)
	for _, pt := range line {
		path.LineTo(pt[0], pt[1])
	}
	path.LineTo(line[len(line)-1][0], bottom)
	path.Close()
	return &path
}

// dashedPolyline strokes line in dashes of 4 DIPs, 3 apart.
func dashedPolyline(p *ui.Painter, line [][2]float32, width float32, color ui.Color) {
	const dash, gap = 4, 3
	on, left := true, float32(dash)
	for i := 1; i < len(line); i++ {
		x0, y0 := line[i-1][0], line[i-1][1]
		x1, y1 := line[i][0], line[i][1]
		seg := float32(math.Hypot(float64(x1-x0), float64(y1-y0)))
		at := float32(0)
		for at < seg {
			step := min(left, seg-at)
			if on {
				t0, t1 := at/seg, (at+step)/seg
				p.Line(x0+(x1-x0)*t0, y0+(y1-y0)*t0, x0+(x1-x0)*t1, y0+(y1-y0)*t1, width, color)
			}
			at += step
			left -= step
			if left <= 0 {
				on = !on
				left = gap
				if on {
					left = dash
				}
			}
		}
	}
}

// niceMax is a scale that fits peak as Task Manager's rate graphs do: the
// next of 1, 2, 5 times a power of ten (never below floor).
func niceMax(peak, floor float32) float32 {
	m, base := floor, floor
	for m < peak*1.1 {
		for _, s := range [...]float32{1, 2, 5} {
			m = base * s
			if m >= peak*1.1 {
				return m
			}
		}
		base *= 10
	}
	return m
}

// graphLabels are the labels above or below a graph: what it is and its
// scale, or the span and zero.
func graphLabels(c *ui.Context, k *colors, left, right string) {
	ui.Row(c).FillWidth().Height(20).AlignItems(ui.End).Shrink(0).Children(func() {
		ui.Text(c, left).FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		ui.Spacer(c)
		ui.Text(c, right).FontSize(fontSmall).TextColor(k.secondary).NoWrap()
	})
}

// legend is a legend's line: a short stroke in a series' style, and its
// name.
func legend(c *ui.Context, k *colors, color ui.Color, dashed bool, name string) {
	ui.Row(c).Gap(6).AlignItems(ui.Center).PaddingEnd(12).Shrink(0).Children(func() {
		ui.Box(c).Size(18, 8).Draw(func(p *ui.Painter, r ui.Rect) {
			y := r.Y + 4
			if dashed {
				dashedPolyline(p, [][2]float32{{r.X, y}, {r.X + 18, y}}, 1.5, color)
			} else {
				p.Line(r.X, y, r.X+18, y, 1.5, color)
			}
		})
		ui.Text(c, name).FontSize(fontSmall).TextColor(k.secondary).NoWrap()
	})
}
