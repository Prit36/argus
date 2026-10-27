package sys

import (
	"testing"
	"time"
	"unsafe"
)

func TestWiFiLayoutsAndNames(t *testing.T) {
	if unsafe.Sizeof(wlanInterface{}) != 532 {
		t.Fatal("native WLAN structure layout does not match Windows")
	}
	for phy, want := range map[uint32]string{4: "802.11a", 7: "802.11n", 8: "802.11ac", 10: "802.11ax", 11: "802.11be", 99: ""} {
		if got := wifiStandard(phy); got != want {
			t.Errorf("PHY %d: got %q, want %q", phy, got, want)
		}
	}

}

func TestAdapterConnectionChangeClearsDetails(t *testing.T) {
	a := Adapter{Connected: true, IPv4: "192.0.2.1", WiFiDetails: true, Signal: 80, Send: 10, Recv: 20, prevTime: time.Now()}
	a.setConnected(false)
	if a.Connected || a.IPv4 != "" || a.WiFiDetails || a.Signal != 0 || a.Send != 0 || a.Recv != 0 || !a.prevTime.IsZero() {
		t.Fatalf("stale data after disconnect: %+v", a)
	}
	a.setConnected(true)
	if !a.Connected || !a.prevTime.IsZero() {
		t.Fatal("reconnect should start a fresh rate baseline")
	}
}

func TestNetworkRequestsCoalesceWiFi(t *testing.T) {
	w := &PerfWorker{wake: make(chan struct{}, 1)}
	w.RequestAddresses(false)
	if !w.addrPending || w.wifiPending {
		t.Fatal("address-only request scheduled Wi-Fi")
	}
	w.RequestAddresses(true)
	w.RequestAddresses(false)
	if !w.addrPending || !w.wifiPending || len(w.wake) != 1 {
		t.Fatal("coalescing lost Wi-Fi request or queued extra wakes")
	}
}

func BenchmarkReadNetworkDetails(b *testing.B) {
	for _, wifi := range []bool{false, true} {
		name := "addresses"
		if wifi {
			name = "with_wifi"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				readNetworkDetails(wifi)
			}
		})
	}
}
