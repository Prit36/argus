package sys

import (
	"net"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNetworkEnergyDeltas(t *testing.T) {
	m := NetMonitor{previous: make(map[procKey]uint64), current: make(map[procKey]uint64)}
	p := []Proc{{PID: 12, Info: &Info{CreateTime: 1}}}
	value, accessible := uint64(100), true
	read := func(*Proc) (uint64, bool) { return value, accessible }
	m.sampleEnergy(p, 2, read)
	if !p[0].NetKnown || p[0].NetDelta != 0 {
		t.Fatalf("first sample: %+v", p[0])
	}
	value = 500
	m.sampleEnergy(p, 2, read)
	if p[0].NetDelta != 400 || p[0].Net != 200 {
		t.Fatalf("delta: %+v", p[0])
	}
	accessible = false
	m.sampleEnergy(p, 2, read)
	if p[0].NetKnown || p[0].NetDelta != 0 {
		t.Fatalf("inaccessible process: %+v", p[0])
	}
	accessible, value = true, 800
	m.sampleEnergy(p, 2, read)
	if p[0].NetDelta != 0 {
		t.Fatalf("recovered process must establish a new baseline: %+v", p[0])
	}
	p[0].Info.CreateTime = 2
	value = 1000
	m.sampleEnergy(p, 2, read)
	if p[0].NetDelta != 0 {
		t.Fatalf("reused PID must establish a new baseline: %+v", p[0])
	}
	value = 1
	m.sampleEnergy(p, 2, read)
	if p[0].NetDelta != 0 {
		t.Fatalf("counter reset: %+v", p[0])
	}
}

// Opt-in because it sends a small number of DNS requests over the network.
func TestNetworkEnergyLiveUDP(t *testing.T) {
	if os.Getenv("TASKMAN_TEST_NETWORK") != "1" {
		t.Skip("set TASKMAN_TEST_NETWORK=1 for the live UDP test")
	}
	if !nduRunning() {
		t.Skip("Windows Network Data Usage driver is not running")
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	creation := int64(uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime))
	p := []Proc{{PID: windows.GetCurrentProcessId(), Info: &Info{CreateTime: creation}}}
	var m NetMonitor
	m.Start()
	defer m.Stop()
	if !m.Available {
		t.Fatal("no per-process network source was available")
	}
	if !m.UsesEnergy() {
		t.Skip("the trace source was selected instead of the energy fallback")
	}
	m.SampleEnergy(p, 1)
	if !p[0].NetKnown {
		t.Fatal("cannot query the current process through the fallback")
	}
	conn, err := net.Dial("udp4", "1.1.1.1:53")
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	request := []byte{0xab, 0xcd, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 7, 'e', 'x', 'a', 'm', 'p', 'l', 'e', 3, 'c', 'o', 'm', 0, 0, 1, 0, 1}
	for range 20 {
		if _, err := conn.Write(request); err != nil {
			t.Fatal(err)
		}
	}
	for range 10 {
		time.Sleep(time.Second)
		m.SampleEnergy(p, 1)
		if p[0].NetKnown && p[0].NetDelta > 0 {
			t.Logf("unelevated UDP sample: %d bytes", p[0].NetDelta)
			return
		}
	}
	t.Fatal("the process network counter did not increase")
}

func TestCurrentProcessNetworkCounter(t *testing.T) {
	if !nduRunning() {
		t.Skip("Windows Network Data Usage driver is not running or cannot be queried")
	}
	if _, ok := processNetworkBytes(windows.CurrentProcess()); !ok {
		t.Fatal("cannot read current process network counter")
	}
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		t.Fatal(err)
	}
	creation := int64(uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime))
	p := Proc{PID: windows.GetCurrentProcessId(), Info: &Info{CreateTime: creation}}
	if _, ok := readProcessNetwork(&p); !ok {
		t.Fatal("cannot read the current process with a matching creation time")
	}
	p.Info.CreateTime++
	if _, ok := readProcessNetwork(&p); ok {
		t.Fatal("accepted a process with a mismatched creation time")
	}
}

func TestStopNetworkEnergyReleasesCounters(t *testing.T) {
	m := NetMonitor{Available: true, energy: true, previous: make(map[procKey]uint64), current: make(map[procKey]uint64)}
	m.Stop()
	if m.Available || m.energy || m.previous != nil || m.current != nil {
		t.Fatal("stopping the fallback did not release its counters")
	}
}
