package app

import (
	"github.com/egoist/mygo/ui"
	"testing"
)

func TestPerformanceFiguresStayStable(t *testing.T) {
	for _, kind := range []PerfKind{PerfCPU, PerfMemory, PerfDisk, PerfNet, PerfGPU} {
		t.Run(itoa(int(kind)), func(t *testing.T) {
			a := &App{perfTab: PerfTab{Kind: kind}}
			k := palette(true, defaultAccent)
			value := "0"
			tt := ui.NewTester(func(c *ui.Context) {
				a.figures(c, func() {
					if kind == PerfNet {
						networkStat(c, &k, "Send", value, true)
						return
					}
					statRow(c, func() { stat(c, &k, "First", value); stat(c, &k, "Second", value) })
				}, func() { kv(c, &k, "Anchor:", "detail") })
			}, 1200, 720)
			tt.Frame()
			before, ok := tt.Find("Anchor:")
			if !ok {
				t.Fatal("missing detail label")
			}
			value = "9999.9 Mbps"
			tt.Frame()
			after, ok := tt.Find("Anchor:")
			if !ok || before.X != after.X || before.Y != after.Y {
				t.Fatalf("detail moved: %+v -> %+v", before, after)
			}
		})
	}
}

func TestPerformanceFiguresWrapOnNarrowWindows(t *testing.T) {
	a := &App{perfTab: PerfTab{Kind: PerfCPU}}
	k := palette(true, defaultAccent)
	tt := ui.NewTester(func(c *ui.Context) {
		a.figures(c, func() {
			statRow(c, func() { stat(c, &k, "First", "1"); stat(c, &k, "Second", "2"); stat(c, &k, "Third", "3") })
		}, func() { kv(c, &k, "Anchor:", "detail") })
	}, 600, 720)
	tt.Frame()
	first, ok1 := tt.Find("First")
	third, ok3 := tt.Find("Third")
	if !ok1 || !ok3 || third.Y <= first.Y {
		t.Fatalf("CPU figures did not wrap: %+v %+v", first, third)
	}
}

func TestDiskFiguresUseAvailableWidth(t *testing.T) {
	a := &App{perfTab: PerfTab{Kind: PerfDisk}}
	k := palette(true, defaultAccent)
	tt := ui.NewTester(func(c *ui.Context) {
		a.figures(c, func() { statRow(c, func() { stat(c, &k, "Active time", "0%") }) },
			func() { kv(c, &k, "Capacity:", "931 GB") })
	}, 900, 720)
	tt.Frame()
	left, ok1 := tt.Find("Active time")
	right, ok2 := tt.Find("Capacity:")
	if !ok1 || !ok2 || right.X <= left.X || right.Y > left.Y+24 {
		t.Fatalf("disk details should be alongside figures: %+v %+v", left, right)
	}
}

func TestCPUDetailsAlongsideFigures(t *testing.T) {
	a, tt := testApp(t)
	a.page = PagePerformance
	a.perfTab = PerfTab{Kind: PerfCPU}
	tt.SetSize(1000, 900)
	tt.Frame()
	left, ok1 := tt.Find("Utilization")
	right, ok2 := tt.Find("Base speed:")
	if !ok1 || !ok2 || right.X <= left.X || right.Y > left.Y+24 {
		t.Fatalf("CPU details should sit alongside figures: %+v %+v", left, right)
	}
}
