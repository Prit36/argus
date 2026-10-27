package app

import (
	"testing"

	"taskmgr/internal/sys"
)

func TestWiFiDetailsView(t *testing.T) {
	a, tt := testApp(t)
	n := &sys.Adapter{Name: "Wi-Fi", Description: "Wireless adapter", WiFi: true, Connected: true, WiFiStandard: "802.11ac", Signal: 80, WiFiDetails: true, IPv4: "192.0.2.1", IPv6: "fe80::1%4"}
	a.net.List = []*sys.Adapter{n}
	a.page = PagePerformance
	a.perfTab = PerfTab{Kind: PerfNet}
	tt.Frame()
	for _, want := range []string{"802.11ac", "80%", "192.0.2.1", "fe80::1%4", "60 samples"} {
		if !tt.HasText(want) {
			t.Errorf("missing %q in %q", want, tt.Texts())
		}
	}
	snap(t, tt, "wifi-details")
	tt.SetSize(700, 720)
	tt.SetDark(false)
	a.darkSystem = false
	tt.Frame()
	snap(t, tt, "wifi-details-narrow-light")
	tt.SetSize(1080, 720)
	n.Connected = false
	tt.Frame()
	if tt.HasText("fe80::1%4") || tt.HasText("Signal strength:") {
		t.Error("disconnected adapter shows stale details")
	}
}

func TestNetworkDetailRequestSelection(t *testing.T) {
	a := &App{net: &sys.Net{List: []*sys.Adapter{{WiFi: true, Connected: true}, {Connected: true}, {WiFi: true}}}, perfTab: PerfTab{Kind: PerfNet}}
	if !a.selectedWiFiConnected() {
		t.Fatal("connected Wi-Fi needs wireless details")
	}
	for _, tab := range []PerfTab{{Kind: PerfNet, Index: 1}, {Kind: PerfNet, Index: 2}, {Kind: PerfNet, Index: 9}, {Kind: PerfCPU}} {
		a.perfTab = tab
		if a.selectedWiFiConnected() {
			t.Fatalf("unnecessary Wi-Fi request for %+v", tab)
		}
	}
}

func TestNetworkDetailsDoNotInvalidateTableRows(t *testing.T) {
	a := fixtureApp(t)
	a.dirty = false
	a.deliverPerformance(nil, []sys.AdapterAddress{})
	if len(a.early) != 1 {
		t.Fatal("network update was not posted")
	}
	a.early[0]()
	if a.dirty {
		t.Fatal("network details unnecessarily invalidated table rows")
	}
}
