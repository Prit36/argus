package app

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestReusedGraphPathMatchesFreshPath(t *testing.T) {
	for _, line := range [][][2]float32{
		{{0, 10}, {20, 20}, {40, 5}},
		{{2, 2}, {4, 40}, {20, 10}, {40, 40}, {50, 1}},
		{{0, 30}, {63, 10}},
	} {
		view := func(reused bool) func(*ui.Context) {
			return func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, _ ui.Rect) {
					for _, bottom := range []float32{64, 48} {
						color := ui.Hex("#3699dd").Alpha(0.4)
						if reused {
							fillArea(p, line, bottom, color)
						} else {
							var path ui.Path
							path.MoveTo(line[0][0], bottom)
							for _, pt := range line {
								path.LineTo(pt[0], pt[1])
							}
							path.LineTo(line[len(line)-1][0], bottom)
							path.Close()
							p.FillPath(&path, color)
						}
					}
				})
			}
		}
		want := ui.NewTester(view(false), 64, 64)
		got := ui.NewTester(view(true), 64, 64)
		got.Frame()
		if !bytes.Equal(want.Image().Pix, got.Image().Pix) {
			t.Fatal("reused graph scratch changed rendered pixels")
		}
	}
}

func TestTrayMeterPixels(t *testing.T) {
	for _, px := range []int{16, 24, 32, 64} {
		for _, level := range []int{0, 33, 67, 100} {
			img, err := png.Decode(bytes.NewReader(meter(px, level)))
			if err != nil {
				t.Fatal(err)
			}
			inset := px / 8
			fillTop := px - inset - (meterKey(px, level) & 0xff) - 1
			for y := range px {
				for x := range px {
					want := color.NRGBA{}
					if x >= inset && x < px-inset && y >= inset && y < px-inset {
						switch {
						case y == inset || y == px-inset-1 || x == inset || x == px-inset-1:
							want = color.NRGBA{0x8a, 0x8a, 0x8a, 0xff}
						case y >= fillTop:
							want = color.NRGBA{0x2e, 0xb8, 0x2e, 0xff}
						default:
							want = color.NRGBA{0x1c, 0x1c, 0x1c, 0xff}
						}
					}
					if got := color.NRGBAModel.Convert(img.At(x, y)); got != want {
						t.Fatalf("%d px, %d%% at %d,%d: got %v, want %v", px, level, x, y, got, want)
					}
				}
			}
		}
	}
}

func TestTrayCachesPixelLevelsAndDPISizes(t *testing.T) {
	for _, px := range []int{16, 24, 32, 64} {
		keys := map[int]bool{}
		for level := range 101 {
			key := meterKey(px, level)
			keys[key] = true
			got := meter(px, level)
			if key>>8 != px || !bytes.Equal(got, meter(px, level)) {
				t.Fatal("cached tray icon lost its size or image")
			}
			if level > 0 && meterKey(px, level-1) == key && &got[0] != &meter(px, level-1)[0] {
				t.Fatal("identical pixel levels did not share cached PNG data")
			}
		}
		if len(keys) != px-2*(px/8)-1 {
			t.Fatalf("size %d cached %d levels, expected one per distinct filled row", px, len(keys))
		}
	}
}
