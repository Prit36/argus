package sys

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// The Microsoft-Windows-Kernel-Network provider.
var kernelNetwork = windows.GUID{Data1: 0x7dd42a49, Data2: 0x5329, Data3: 0x4832, Data4: [8]byte{0x8d, 0xfd, 0x43, 0xd9, 0x79, 0x15, 0x3a, 0x88}}

var traceName = windows.StringToUTF16("nanotask network")

// traceProps are a session's settings, its name after them.
type traceProps struct {
	p    winapi.EventTraceProperties
	name [64]uint16
}

func newTraceProps() *traceProps {
	x := &traceProps{}
	x.p.WnodeBufferSize = uint32(unsafe.Sizeof(*x))
	// WNODE_FLAG_TRACED_GUID; timestamps from the performance counter.
	x.p.WnodeFlags = 0x00020000
	x.p.WnodeClientContext = 1
	// EVENT_TRACE_REAL_TIME_MODE; buffers handed over every second.
	x.p.LogFileMode = 0x100
	x.p.FlushTimer = 1
	x.p.BufferSize = 64
	x.p.LoggerNameOffset = uint32(unsafe.Sizeof(x.p))
	return x
}

// netEntry is a process's bytes since the sampler last took them.
type netEntry struct {
	pid   uint32
	bytes uint64
}

// netCounters sum bytes by process: dense entries keep draining cheap, and
// open addressing keeps each event's work small (they come per packet).
type netCounters struct {
	entries [1024]netEntry
	slots   [2048]uint16
	used    int
}

func (c *netCounters) add(pid uint32, bytes uint64) {
	slot := int((pid * 0x9e3779b1) >> 21)
	for c.slots[slot] != 0 {
		e := &c.entries[c.slots[slot]-1]
		if e.pid == pid {
			e.bytes += bytes
			return
		}
		slot = (slot + 1) & (len(c.slots) - 1)
	}
	if c.used == len(c.entries) {
		return
	}
	c.entries[c.used] = netEntry{pid, bytes}
	c.used++
	c.slots[slot] = uint16(c.used)
}

func (c *netCounters) reset() {
	c.used = 0
	clear(c.slots[:])
}

// NetMonitor counts each process's network use, as Task Manager does: the
// kernel's network provider reports every TCP and UDP send and receive
// with its process and size, through a real-time trace session read on a
// goroutine of its own. Starting a session needs administrator rights;
// without them Available is false.
type NetMonitor struct {
	session   uint64
	trace     uint64
	Available bool
	mu        sync.Mutex
	counters  netCounters
	copy      []netEntry
}

var (
	activeMonitor *NetMonitor
	eventCallback = windows.NewCallback(onNetEvent)
)

// Start starts the trace session; Available says whether it did.
func (m *NetMonitor) Start() {
	props := newTraceProps()
	st := winapi.StartTrace(&m.session, &traceName[0], unsafe.Pointer(props))
	if st == uint32(windows.ERROR_ALREADY_EXISTS) {
		// Left by a run that didn't stop it.
		winapi.ControlTrace(0, &traceName[0], unsafe.Pointer(newTraceProps()), 1)
		props = newTraceProps()
		st = winapi.StartTrace(&m.session, &traceName[0], unsafe.Pointer(props))
	}
	if st != 0 {
		return
	}
	// Level 4 (information); keywords IPv4 and IPv6.
	if winapi.EnableTraceEx2(m.session, &kernelNetwork, 1, 4, 0x10|0x20) != 0 {
		m.stopSession()
		return
	}
	activeMonitor = m
	log := winapi.EventTraceLogfile{
		LoggerName: &traceName[0],
		// PROCESS_TRACE_MODE_REAL_TIME | PROCESS_TRACE_MODE_EVENT_RECORD.
		ProcessTraceMode:    0x100 | 0x10000000,
		EventRecordCallback: eventCallback,
	}
	m.trace = winapi.OpenTrace(&log)
	if m.trace == winapi.InvalidTrace {
		m.stopSession()
		return
	}
	m.Available = true
	go func(h uint64) {
		// Returns once the session stops.
		winapi.ProcessTrace(&h)
	}(m.trace)
}

func (m *NetMonitor) stopSession() {
	winapi.ControlTrace(m.session, nil, unsafe.Pointer(newTraceProps()), 1)
	m.session = 0
}

// Stop stops the session.
func (m *NetMonitor) Stop() {
	if !m.Available {
		return
	}
	m.Available = false
	m.stopSession()
	winapi.CloseTrace(m.trace)
}

// Take calls f with each process's bytes since the last call, and starts
// counting again.
func (m *NetMonitor) Take(f func(pid uint32, bytes uint64)) {
	if !m.Available {
		return
	}
	m.mu.Lock()
	m.copy = append(m.copy[:0], m.counters.entries[:m.counters.used]...)
	m.counters.reset()
	m.mu.Unlock()
	for _, e := range m.copy {
		f(e.pid, e.bytes)
	}
}

func onNetEvent(e *winapi.EventRecord) uintptr {
	// TCP and UDP, IPv4 and IPv6, sent and received: their payloads start
	// with the process and the size.
	switch e.Header.ID {
	case 10, 11, 26, 27, 42, 43, 58, 59:
	default:
		return 0
	}
	m := activeMonitor
	if m == nil || e.UserDataLength < 8 || e.UserData == nil {
		return 0
	}
	d := (*[2]uint32)(e.UserData)
	pid, size := d[0], d[1]
	if pid == 0 {
		return 0
	}
	m.mu.Lock()
	m.counters.add(pid, uint64(size))
	m.mu.Unlock()
	return 0
}
