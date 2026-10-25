package app

import "testing"

func TestFigures(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{grouped(1234567, ""), "1,234,567"},
		{grouped(12, " K"), "12 K"},
		{fmtUptime(86400 + 2*3600 + 3*60 + 4), "1:02:03:04"},
		{fmtBits(10), "0 Kbps"},
		{fmtBits(1.5e6), "1.5 Mbps"},
		{fmtMB(12 << 20), "12.0 MB"},
		{mbCell(1902<<20 + 220<<10), "1,902.2 MB"},
		{cpuCell(0.01), "0%"},
		{cpuCell(0.8), "0.8%"},
		{diskCell(1000), "0 MB/s"},
		{netCell(12500), "0.1 Mbps"},
		{fmtCPUTime(3723 * 10_000_000), "1:02:03"},
		{fmtCache(480 << 10), "480 KB"},
		{fmtCache(18 << 20), "18.0 MB"},
		{fmtRate(30 << 10), "30 KB/s"},
		{fmtRate(600 << 10), "0.6 MB/s"},
		{fmtBytes(512), "512 B"},
		{fmtGHz(2.496), "2.50 GHz"},
		{mbTotal(3 << 30), "3.0 GB"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
	if !containsFold("Microsoft Edge", "EDGE") || containsFold("Edge", "edges") {
		t.Error("containsFold")
	}
	if cmpFold("abc", "ABD") >= 0 || cmpFold("b", "A") <= 0 || cmpFold("Ab", "aB") != 0 {
		t.Error("cmpFold")
	}
	if !pidHasPrefix(1234, "12") || pidHasPrefix(1234, "23") {
		t.Error("pidHasPrefix")
	}
}

func TestNiceMax(t *testing.T) {
	for _, c := range []struct{ peak, want float32 }{{10, 100}, {150, 200}, {300, 500}, {600, 1000}} {
		if got := niceMax(c.peak, 100); got != c.want {
			t.Errorf("niceMax(%v) = %v, want %v", c.peak, got, c.want)
		}
	}
}
