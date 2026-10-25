package sys

import (
	"testing"
	"time"
)

// These tests sample the machine they run on and report what a sample
// costs.

func TestSampleProcesses(t *testing.T) {
	exes := NewExeCache()
	s := NewSampler(exes, NewResolver(exes, 4))
	s.Sample(time.Now())
	if len(s.Procs) < 10 {
		t.Fatalf("only %d processes", len(s.Procs))
	}
	start := time.Now()
	s.WaitResolved(2 * time.Second)
	t.Logf("resolved %d processes in %v", len(s.Procs), time.Since(start))
	time.Sleep(200 * time.Millisecond)
	start = time.Now()
	s.Sample(time.Now())
	t.Logf("a sample of %d processes, %d threads, %d handles: %v; CPU %.1f%%, %d cores", len(s.Procs), s.Threads, s.Handles, time.Since(start), s.CPU*100, s.Cores())
	allocs := testing.AllocsPerRun(20, func() { s.Sample(time.Now()) })
	t.Logf("allocations per steady sample: %.1f", allocs)
	var self *Proc
	named, windowed := 0, 0
	for i := range s.Procs {
		p := &s.Procs[i]
		if p.PID == s.SelfPID {
			self = p
		}
		if p.Info.Exe != nil {
			named++
		}
		if p.Windowed {
			windowed++
		}
	}
	if self == nil {
		t.Fatal("this process isn't in the sample")
	}
	t.Logf("self: %q user %q arch %v path %q cmdline %q ws %d", self.Info.Name, self.Info.User, self.Info.Arch, self.Info.Path(), self.Info.Cmdline, self.WorkingSet)
	t.Logf("%d of %d with a program file, %d windowed", named, len(s.Procs), windowed)
	if self.Info.User == "" || self.Info.Path() == "" || self.WorkingSet == 0 {
		t.Fatal("this process wasn't resolved")
	}
}

func TestMachine(t *testing.T) {
	c := ReadCPU()
	t.Logf("CPU %q %d MHz, %d sockets, %d cores, %d logical, L1 %d L2 %d L3 %d", c.Name, c.BaseMHz, c.Sockets, c.Cores, c.Logical, c.L1, c.L2, c.L3)
	m := ReadMemoryHardware()
	t.Logf("memory %+v", m)
	mem := ReadMemory()
	t.Logf("memory use %+v", mem)
	if c.Logical == 0 || mem.Total == 0 {
		t.Fatal("nothing read")
	}
	ds := FindDisks()
	defer ds.Close()
	ds.Sample()
	for _, d := range ds.List {
		t.Logf("%s %q %s capacity %d formatted %d system %v pagefile %v", d.Title(), d.Model, d.KindTitle(), d.Capacity, d.Formatted, d.System, d.Pagefile)
	}
	n := FindNet()
	n.Sample(time.Now())
	for _, a := range n.List {
		t.Logf("%s %q %q connected %v link %d", a.Kind(), a.Name, a.Description, a.Connected, a.LinkSpeed)
	}
	for _, a := range ReadAddresses() {
		if a.IPv4 != "" || a.IPv6 != "" {
			t.Logf("addresses %x: %s %s", a.LUID, a.IPv4, a.IPv6)
		}
	}
	for _, g := range FindGPUs() {
		t.Logf("GPU %q dedicated %d shared %d driver %v", g.Name, g.DedicatedTotal, g.SharedTotal, g.Driver)
	}
}

func TestGPUCounters(t *testing.T) {
	gpus := FindGPUs()
	got := make(chan *GPUReading, 4)
	w := NewPerfWorker(gpus, func(r *GPUReading, _ []AdapterAddress) {
		if r != nil {
			got <- r
		}
	})
	defer w.Close()
	start := time.Now()
	w.RequestGPU(true, true)
	r := <-got
	t.Logf("first collection in %v: warm %v cpu perf %.0f", time.Since(start), r.Warm, r.CPUPerf)
	time.Sleep(500 * time.Millisecond)
	w.RequestGPU(true, true)
	r = <-got
	t.Logf("second: warm %v cpu perf %.0f, %d processes on GPUs", r.Warm, r.CPUPerf, len(r.PerPID))
	for i, g := range r.GPUs {
		t.Logf("GPU %d: use %.1f engines %v dedicated %d shared %d", i, g.Use, g.Engines, g.Dedicated, g.Shared)
	}
}

func TestServicesStartupUsers(t *testing.T) {
	start := time.Now()
	sv := ReadServices(ServiceGroups{})
	if sv == nil || len(sv.List) == 0 {
		t.Fatal("no services")
	}
	t.Logf("%d services in %v", len(sv.List), time.Since(start))
	for _, s := range sv.List[:3] {
		t.Logf("  %+v", s)
	}
	items := ReadStartup(NewExeCache())
	for _, it := range items {
		t.Logf("startup %q %q enabled %v", it.Name, it.Publisher, it.Enabled)
	}
	for _, s := range ReadSessions() {
		t.Logf("session %+v", s)
	}
	t.Logf("BIOS %.1f s, uptime %d s", BiosSeconds(), Uptime())
}

func TestIcons(t *testing.T) {
	exes := NewExeCache()
	s := NewSampler(exes, NewResolver(exes, 2))
	s.Sample(time.Now())
	s.WaitResolved(2 * time.Second)
	for i := range s.Procs {
		if p := s.Procs[i].Info.Path(); p != "" {
			img := FileIcon(p, 24)
			if img != nil {
				t.Logf("icon of %s: %v", p, img.Bounds())
				break
			}
		}
	}
	if GenericIcon(16) == nil {
		t.Fatal("no generic icon")
	}
}

func TestDates(t *testing.T) {
	// 2026-10-25 00:00 UTC.
	if got := FiletimeDate(134_373_600_000_000_000); got != "10/25/2026" {
		t.Fatalf("got %q", got)
	}
}
