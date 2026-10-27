package app

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestColumnResizeDrag(t *testing.T) {
	a := &App{page: PageServices}
	k := colors{}
	clicks := 0
	view := func(c *ui.Context) {
		cols := []column{{title: "First", width: 180}, {title: "Second", width: 100}, {title: "Flexible"}}
		minW, flexMin := a.applyColumnWidths(cols, 500)
		clicked, _ := a.tableHeader(c, &k, cols, -1, true, compactHeaderH, &minW, flexMin, true, nil)
		if clicked >= 0 {
			clicks++
		}
	}
	tt := ui.NewTester(view, 800, 200)
	tt.Move(191, 20)
	if tt.Cursor() != ui.CursorResizeEW {
		t.Fatalf("resize cursor missing: %v", tt.Cursor())
	}
	tt.Press(191, 20)
	tt.Move(251, 20)
	tt.Release(251, 20)
	if got := a.columnWidths[PageServices]["First"]; got != 240 {
		t.Fatalf("width after drag = %v, want 240", got)
	}
	if clicks != 0 {
		t.Fatalf("drag triggered sorting %d times", clicks)
	}
	tt.ClickAt(50, 20)
	if clicks != 1 {
		t.Fatalf("header click did not sort: %d", clicks)
	}
}

func TestColumnWidthsAreScopedAndAdjustMinimum(t *testing.T) {
	a := &App{page: PageServices}
	a.columnWidths[PageServices] = map[string]float32{"First": 240, "Flexible": 250}
	cols := []column{{title: "First", width: 180}, {title: "Flexible"}}
	minW, _ := a.applyColumnWidths(cols, 500)
	if cols[0].width != 240 || cols[1].width != 250 || minW != 519 {
		t.Fatalf("widths = %v, %v, minimum = %v", cols[0].width, cols[1].width, minW)
	}
	a.page = PageUsers
	cols = []column{{title: "First", width: 180}, {title: "Flexible"}}
	minW, _ = a.applyColumnWidths(cols, 500)
	if cols[0].width != 180 || cols[1].width != 0 || minW != 500 {
		t.Fatalf("widths leaked across pages: %+v, %v", cols, minW)
	}
}

func TestResizeAfterFlexibleColumnAndClamp(t *testing.T) {
	a := &App{page: PageUsers}
	k := colors{}
	view := func(c *ui.Context) {
		cols := []column{{title: "Flexible"}, {title: "Fixed", width: 100}, {title: "Last", width: 100}}
		minW, flexMin := a.applyColumnWidths(cols, 500)
		a.tableHeader(c, &k, cols, -1, true, compactHeaderH, &minW, flexMin, false, nil)
	}
	tt := ui.NewTester(view, 800, 200)
	// Fixed ends at 800 minus the padding and last column.
	tt.Move(684, 20)
	if tt.Cursor() != ui.CursorResizeEW {
		t.Fatalf("resize cursor missing: %v", tt.Cursor())
	}
	tt.Press(684, 20)
	tt.Move(724, 20)
	tt.Release(724, 20)
	if got := a.columnWidths[PageUsers]["Fixed"]; got != 140 {
		t.Fatalf("fixed width = %v, want 140", got)
	}
	if got := a.columnWidths[PageUsers]["Flexible"]; got < 400 {
		t.Fatalf("flexible column was not kept in place: %v", got)
	}
	tt.Move(724, 20)
	tt.Press(724, 20)
	tt.Move(524, 20)
	tt.Release(524, 20)
	if got := a.columnWidths[PageUsers]["Fixed"]; got != minColumnWidth {
		t.Fatalf("minimum width = %v, want %v", got, minColumnWidth)
	}
}

func TestHeaderCellsAlignWithRows(t *testing.T) {
	a := &App{page: PageProcesses}
	k := colors{}
	var state ui.ListState
	view := func(c *ui.Context) {
		cols := []column{{title: "Name"}, {title: "CPU", width: 80, right: true}, {title: "Memory", width: 104, right: true}, {title: "Network", width: 88, right: true}}
		minW, flexMin := a.applyColumnWidths(cols, 600)
		ui.Column(c).Fill().Children(func() {
			_, header := a.tableHeader(c, &k, cols, -1, true, compactHeaderH, &minW, flexMin, true, nil)
			list := ui.List(c, &state, 100).FillWidth().Grow(1).RowWidth(minW)
			list.Rows(func(lr ui.ListRow) {
				c := lr.Context
				ui.Box(c).FillWidth().Children(func() {
					ui.Row(c).FillWidth().Height(rowH).Padding(0, 14, 0, pagePad-10).Gap(1).Dividers(1, k.cellDivider).Children(func() {
						textCell(c, &cols[0], "A", k.text)
						textCell(c, &cols[1], "B", k.text)
						textCell(c, &cols[2], "C", k.text)
						textCell(c, &cols[3], "D", k.text)
					})
				})
			})
			header.ScrollWith(list)
		})
	}
	tt := ui.NewTester(view, 800, 250)
	assertAligned := func() {
		t.Helper()
		for _, p := range [][2]string{{"CPU", "B"}, {"Memory", "C"}, {"Network", "D"}} {
			h, ok := tt.Find(p[0])
			if !ok {
				t.Fatal("missing", p[0])
			}
			d, ok := tt.Find(p[1])
			if !ok {
				t.Fatal("missing", p[1])
			}
			if delta := (h.X + h.W) - (d.X + d.W); delta < -1 || delta > 1 {
				t.Errorf("%s and %s right edges differ by %v, header=%+v row=%+v", p[0], p[1], delta, h, d)
			}
		}
	}
	assertAligned()
	// A narrow column must still share its right edge with its data.
	a.columnWidths[PageProcesses] = map[string]float32{"Network": 40, "CPU": 150}
	tt.Frame()
	assertAligned()
	// The header follows its rows when the table is wider than the viewport.
	tt.SetSize(350, 250)
	tt.Scroll(200, 100, 250, 0)
	assertAligned()
}

func TestSelectionStaysCorrectAfterSortAndFilter(t *testing.T) {
	a := fixtureApp(t)
	a.set.GroupByType = false
	a.page = PageProcesses
	a.refreshRows()
	a.selected = a.model.Rows[len(a.model.Rows)-1].Key
	tt := ui.NewTester(a.view, 1080, 720)
	click(t, tt, "CPU")
	if a.selIdx < 0 || a.model.Rows[a.selIdx].Key != a.selected {
		t.Fatal("process selection moved after sorting")
	}

	a.page, a.dirty = PageDetails, true
	a.detailsSel = procKey(&a.sampler.Procs[1])
	tt.Frame()
	click(t, tt, "CPU")
	if a.detailsIdx < 0 || procKey(&a.sampler.Procs[a.details[a.detailsIdx]]) != a.detailsSel {
		t.Fatal("details selection moved after sorting")
	}
	a.query = "Alpha"
	tt.Frame()
	if a.detailsIdx != -1 || a.detailsSelected() == nil || a.detailsSelected().PID != 20 {
		t.Fatal("filtered selection lost")
	}
	a.query = ""
	tt.Frame()
	if a.detailsIdx < 0 || procKey(&a.sampler.Procs[a.details[a.detailsIdx]]) != a.detailsSel {
		t.Fatal("selection did not reappear after clearing filter")
	}
}
