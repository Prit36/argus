package app

import (
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

func TestGraphGridIsNeutral(t *testing.T) {
	for _, dark := range []bool{false, true} {
		k := palette(dark, defaultAccent)
		for _, hue := range []ui.Color{k.cpu, k.memory, k.disk, k.network, k.gpu} {
			var history sys.Ring
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(uiTheme(&k))
				graph(c, &k, []series{{ring: &history}}, graphOptions{width: 240, height: 120, color: hue, max: 100, grid: true})
			}, 240, 120)
			grid := tt.Image().RGBAAt(24, 6)
			bg := tt.Image().RGBAAt(28, 6)
			if grid == bg || grid.R != grid.G || grid.G != grid.B {
				t.Errorf("dark=%v: graph grid is missing or tinted: %v (background %v)", dark, grid, bg)
			}
		}
	}
}

func TestLogicalProcessorView(t *testing.T) {
	a := fixtureApp(t)
	a.darkSystem, a.accent, a.page = true, defaultAccent, PagePerformance
	a.cpuInfo = sys.CPU{Name: "12 logical processors", Logical: 12, Cores: 6, Sockets: 1}
	a.cores = make([]sys.Ring, 12)
	for i := range 60 {
		for j := range a.cores {
			v := 9 + 6*math.Sin(float64(i+j*4)*0.45)
			if (i+j*7)%23 == 0 {
				v += 36
			}
			a.cores[j].Push(float32(v))
		}
	}
	tt := ui.NewTester(a.view, 1440, 900)
	click(t, tt, "CPU view")
	if err := tt.ChooseMenuItem("Logical processors"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !a.set.CPULogical || !loadSettings().CPULogical {
		t.Fatal("logical processor choice was not applied and saved")
	}
	var tiles [12]ui.Rect
	for i := range tiles {
		r, ok := tt.Find("Logical processor " + itoa(i))
		if !ok || r.W <= 0 || r.H <= 0 {
			t.Fatalf("missing logical processor %d", i)
		}
		tiles[i] = r
	}
	if tiles[0].Y != tiles[3].Y || tiles[4].Y <= tiles[0].Y || tiles[8].Y <= tiles[4].Y {
		t.Fatal("12 logical processors must form a complete 4 by 3 grid")
	}
	// Confirm the per-processor tile contains actual neutral grid pixels.
	r, pixels := tiles[0], 0
	img := tt.Image()
	for y := int(r.Y) + 2; y < int(r.Y+r.H)-2; y++ {
		for x := int(r.X) + 2; x < int(r.X+r.W)-2; x++ {
			if img.RGBAAt(x, y) == (color.RGBA{R: 44, G: 44, B: 44, A: 255}) {
				pixels++
			}
		}
	}
	if pixels < 50 {
		t.Fatal("logical processor tile has no visible neutral grid")
	}
	snap(t, tt, "logical-processors-grid")
	click(t, tt, "CPU view")
	if err := tt.ChooseMenuItem("Overall utilization"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.set.CPULogical || loadSettings().CPULogical {
		t.Fatal("overall utilization choice was not applied and saved")
	}
}
