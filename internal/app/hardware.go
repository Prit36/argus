package app

import (
	"sync"
	"time"

	"taskmgr/internal/sys"
)

// Hardware descriptions can wait on storage and graphics drivers. They are
// independent of the first process sample and arrive after the window opens.
type hardwareData struct {
	cpu    sys.CPU
	memory sys.MemoryHardware
	disks  *sys.Disks
	net    *sys.Net
	gpus   []*sys.GPU
}

func readHardware() *hardwareData {
	h := &hardwareData{cpu: sys.ReadCPU(), memory: sys.ReadMemoryHardware(),
		disks: sys.FindDisks(), net: sys.FindNet(), gpus: sys.FindGPUs()}
	h.disks.Sample()
	h.net.Sample(time.Now())
	return h
}

// hardwareLoad owns a result until the UI takes it. Closing during discovery
// releases its disk handles even when the window can no longer accept posts.
type hardwareLoad struct {
	mu     sync.Mutex
	data   *hardwareData
	closed bool
}

func (l *hardwareLoad) publish(h *hardwareData) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		h.disks.Close()
		return false
	}
	l.data = h
	return true
}

func (l *hardwareLoad) take() *hardwareData {
	l.mu.Lock()
	defer l.mu.Unlock()
	h := l.data
	l.data = nil
	return h
}

func (l *hardwareLoad) close() {
	l.mu.Lock()
	l.closed = true
	h := l.data
	l.data = nil
	l.mu.Unlock()
	if h != nil {
		h.disks.Close()
	}
}

func (a *App) applyHardware(h *hardwareData) {
	a.cpuInfo, a.memHW = h.cpu, h.memory
	a.disks, a.net, a.gpus = h.disks, h.net, h.gpus
	a.hardwareReady = true
	a.dirty = true
}

func (a *App) takeHardware() {
	if h := a.hardware.take(); h != nil {
		a.applyHardware(h)
		a.syncPerformance(true)
		if a.page == PagePerformance && a.perfTab.Kind == PerfNet {
			a.requestAddresses()
		}
	}
}
