package app

import (
	"fmt"
	"image/color"
	"math"
	"testing"

	"github.com/egoist/mygo/ui"
)

func colorContrast(a, b ui.Color) float64 {
	luminance := func(c ui.Color) float64 {
		linear := func(v uint8) float64 {
			s := float64(v) / 255
			if s <= 0.04045 {
				return s / 12.92
			}
			return math.Pow((s+0.055)/1.055, 2.4)
		}
		return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B)
	}
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}

func TestNeutralPaletteContrast(t *testing.T) {
	for _, dark := range []bool{false, true} {
		k := palette(dark, defaultAccent)
		for _, bg := range []ui.Color{k.page, k.backdrop, k.card, k.field, k.rowHover, k.rowSelected} {
			if bg.R != bg.G || bg.G != bg.B {
				t.Errorf("dark=%v: surface has an unwanted color tint: %v", dark, bg)
			}
			for _, fg := range []ui.Color{k.text, k.secondary} {
				if ratio := colorContrast(fg, bg); ratio < 4.5 {
					t.Errorf("dark=%v: text contrast %.2f < 4.5", dark, ratio)
				}
			}
		}
		for _, bg := range []ui.Color{k.page, k.backdrop, k.card, k.field} {
			if ratio := colorContrast(k.tertiary, bg); ratio < 4.5 {
				t.Errorf("dark=%v: tertiary text contrast %.2f < 4.5", dark, ratio)
			}
		}
		for i := range 101 {
			bg := k.heat(float32(i) / 100)
			if bg.B <= bg.R {
				t.Errorf("dark=%v: resource shading lost the blue usage tint", dark)
			}
			if ratio := colorContrast(k.heatText, bg); ratio < 4.5 {
				t.Errorf("dark=%v: resource text contrast %.2f < 4.5", dark, ratio)
			}
		}
		if k.heat(0) == k.heat(1) {
			t.Errorf("dark=%v: resource shading should distinguish idle from busy", dark)
		}
	}
}

// Rendered pixels catch missed resource cells and interaction highlights
// that erase the blue heat map.
func TestResourceRowsHighlightAcrossAllColumns(t *testing.T) {
	for _, dark := range []bool{false, true} {
		for _, page := range []Page{PageProcesses, PageUsers} {
			t.Run(fmt.Sprintf("dark=%v/page=%v", dark, page), func(t *testing.T) {
				a := fixtureApp(t)
				a.darkSystem, a.accent, a.page = dark, defaultAccent, page
				tt := ui.NewTester(a.view, 1080, 720)
				label := "Alpha"
				if page == PageUsers {
					label = "Ada (3)"
				}
				r, ok := tt.Find(label)
				if !ok {
					t.Fatalf("missing row %q among %v", label, tt.Texts())
				}
				y := r.Y + r.H/2
				// Sample below the text, away from dividers and icons.
				sampleY := int(y) + 12
				xs := []float32{500}
				for _, name := range []string{"CPU", "Memory", "Disk", "Network"} {
					h, ok := tt.Find(name)
					if !ok {
						t.Fatal("missing header", name)
					}
					xs = append(xs, h.X+h.W/2)
				}
				idle := make([]color.RGBA, len(xs))
				for i, x := range xs {
					idle[i] = tt.Image().RGBAAt(int(x), sampleY)
				}
				assertHighlighted := func(state string) {
					t.Helper()
					img := tt.Image()
					for i, x := range xs {
						got := img.RGBAAt(int(x), sampleY)
						if got == idle[i] {
							t.Errorf("%s: column at %.0f was not highlighted", state, x)
						}
						if i > 0 && got.B <= got.R {
							t.Errorf("%s: usage cell at %.0f lost its blue shading: %v", state, x, got)
						}
						brightness := func(c color.RGBA) int { return int(c.R) + int(c.G) + int(c.B) }
						if dark && brightness(got) <= brightness(idle[i]) || !dark && brightness(got) >= brightness(idle[i]) {
							t.Errorf("%s: column at %.0f has the wrong highlight direction", state, x)
						}
					}
					if idle[1] != idle[2] && img.RGBAAt(int(xs[1]), sampleY) == img.RGBAAt(int(xs[2]), sampleY) {
						t.Errorf("%s flattened distinct usage levels into the same color", state)
					}
				}
				for _, x := range xs {
					tt.Move(x, y)
					tt.Frame()
					assertHighlighted("hover")
				}
				snap(t, tt, fmt.Sprintf("neutral-%v-%v-hover", dark, page))
				tt.ClickAt(xs[0], y)
				tt.Move(600, 70)
				tt.Frame()
				assertHighlighted("selected")
				snap(t, tt, fmt.Sprintf("neutral-%v-%v-selected", dark, page))
			})
		}
	}
}
