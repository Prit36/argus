package sys

import "sync"

// PerfWorker collects the costly counters (PDH can load providers for
// hundreds of milliseconds on its first use) and reads the adapters'
// addresses, off the UI thread. It sleeps between requests, and requests
// coalesce rather than queue. Each result is handed to Deliver, from the
// worker's goroutine: the app posts it to its window.
type PerfWorker struct {
	mu          sync.Mutex
	enabled     bool
	gpuPending  bool
	addrPending bool
	quitting    bool
	wake        chan struct{}
	done        chan struct{}
	counters    gpuCounters
	deliver     func(*GPUReading, []AdapterAddress)
}

// NewPerfWorker starts a worker for gpus.
func NewPerfWorker(gpus []*GPU, deliver func(*GPUReading, []AdapterAddress)) *PerfWorker {
	w := &PerfWorker{wake: make(chan struct{}, 1), done: make(chan struct{}), deliver: deliver}
	for _, g := range gpus {
		w.counters.gpus = append(w.counters.gpus, gpuIdentity{g.luidLow, g.luidHigh})
	}
	go w.run()
	return w
}

func (w *PerfWorker) signal() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// RequestGPU turns the counters on or off; with sample, it asks for a
// fresh collection. Disabling wins at once.
func (w *PerfWorker) RequestGPU(enabled, sample bool) {
	w.mu.Lock()
	changed := w.enabled != enabled
	w.enabled = enabled
	w.gpuPending = w.gpuPending || changed || (enabled && sample)
	pending := w.gpuPending
	w.mu.Unlock()
	if pending {
		w.signal()
	}
}

// RequestAddresses asks for the adapters' addresses.
func (w *PerfWorker) RequestAddresses() {
	w.mu.Lock()
	w.addrPending = true
	w.mu.Unlock()
	w.signal()
}

// Close stops the worker and waits for it: it owns the queries until it
// exits.
func (w *PerfWorker) Close() {
	w.mu.Lock()
	w.quitting = true
	w.mu.Unlock()
	w.signal()
	<-w.done
}

func (w *PerfWorker) run() {
	defer close(w.done)
	defer w.counters.stop()
	for range w.wake {
		w.mu.Lock()
		quitting, enabled := w.quitting, w.enabled
		sample, addrs := w.gpuPending, w.addrPending
		w.gpuPending, w.addrPending = false, false
		w.mu.Unlock()
		if quitting {
			return
		}
		var reading *GPUReading
		if !enabled {
			w.counters.stop()
		} else if sample {
			reading = w.counters.sample()
		}
		var addresses []AdapterAddress
		if addrs {
			addresses = ReadAddresses()
		}
		// Discard a reading if the page was left while PDH was busy.
		w.mu.Lock()
		if !w.enabled {
			reading = nil
		}
		w.mu.Unlock()
		if reading != nil || addresses != nil {
			w.deliver(reading, addresses)
		}
	}
}
