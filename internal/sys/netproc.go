package sys

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// The Microsoft-Windows-Kernel-Network provider.
var kernelNetwork = windows.GUID{Data1: 0x7dd42a49, Data2: 0x5329, Data3: 0x4832, Data4: [8]byte{0x8d, 0xfd, 0x43, 0xd9, 0x79, 0x15, 0x3a, 0x88}}

var traceName = windows.StringToUTF16("argus network")

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

// NetMonitor uses a real-time network trace when permitted, and otherwise
// samples Windows' per-process network energy counters. The latter needs the
// Windows Network Data Usage driver and access to each process.
type NetMonitor struct {
	session   uint64
	trace     uint64
	Available bool
	mu        sync.Mutex
	counters  netCounters
	copy      []netEntry
	energy    bool
	previous  map[procKey]uint64
	current   map[procKey]uint64
}

var (
	activeMonitor *NetMonitor
	eventCallback = windows.NewCallback(onNetEvent)
)

// Start selects a network source; Available says whether one was found.
func (m *NetMonitor) Start() {
	if m.startTrace() {
		return
	}
	if nduRunning() {
		if _, ok := processNetworkBytes(windows.CurrentProcess()); ok {
			m.energy, m.Available = true, true
			m.previous = make(map[procKey]uint64)
			m.current = make(map[procKey]uint64)
		}
	}
}

func (m *NetMonitor) startTrace() bool {
	props := newTraceProps()
	st := winapi.StartTrace(&m.session, &traceName[0], unsafe.Pointer(props))
	if st == uint32(windows.ERROR_ALREADY_EXISTS) {
		// Left by a run that didn't stop it.
		winapi.ControlTrace(0, &traceName[0], unsafe.Pointer(newTraceProps()), 1)
		props = newTraceProps()
		st = winapi.StartTrace(&m.session, &traceName[0], unsafe.Pointer(props))
	}
	if st != 0 {
		return false
	}
	// Level 4 (information); keywords IPv4 and IPv6.
	if winapi.EnableTraceEx2(m.session, &kernelNetwork, 1, 4, 0x10|0x20) != 0 {
		m.stopSession()
		return false
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
		return false
	}
	m.Available = true
	go func(h uint64) {
		// Returns once the session stops.
		winapi.ProcessTrace(&h)
	}(m.trace)
	return true
}

// ProcessEnergyValues is a Windows native API, available since Windows 10
// 1703. NetworkTxRxBytes is at offset 88 in PROCESS_ENERGY_VALUES; the
// remaining fields are deliberately opaque because Windows has extended
// the structure over time. NetworkTxRxBytes includes both send and receive.
func processNetworkBytes(h windows.Handle) (uint64, bool) {
	var data [432 / 8]uint64
	st := winapi.NtQueryInformationProcess(h, 76, unsafe.Pointer(&data[0]), uint32(unsafe.Sizeof(data)))
	return data[11], st == 0
}

func nduRunning() bool {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(scm)
	h, err := windows.OpenService(scm, windows.StringToUTF16Ptr("Ndu"), windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return false
	}
	defer windows.CloseServiceHandle(h)
	var status windows.SERVICE_STATUS
	return windows.QueryServiceStatus(h, &status) == nil && status.CurrentState == windows.SERVICE_RUNNING
}

// SampleEnergy reads accessible processes and computes deltas using both PID
// and creation time, so a reused PID never inherits another process's bytes.
func (m *NetMonitor) SampleEnergy(procs []Proc, dt float64) {
	if !m.Available || !m.energy {
		return
	}
	m.sampleEnergy(procs, dt, readProcessNetwork)
}

func readProcessNetwork(p *Proc) (uint64, bool) {
	if p.PID == 0 || p.Info == nil {
		return 0, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, p.PID)
	if err != nil {
		return 0, false
	}
	var created, exited, kernel, user windows.Filetime
	err = windows.GetProcessTimes(h, &created, &exited, &kernel, &user)
	creation := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
	if err != nil || creation != uint64(p.Info.CreateTime) {
		windows.CloseHandle(h)
		return 0, false
	}
	value, ok := processNetworkBytes(h)
	windows.CloseHandle(h)
	return value, ok
}

func (m *NetMonitor) sampleEnergy(procs []Proc, dt float64, read func(*Proc) (uint64, bool)) {
	clear(m.current)
	for i := range procs {
		p := &procs[i]
		p.Net, p.NetDelta, p.NetKnown = 0, 0, false
		if p.PID == 0 || p.Info == nil {
			continue
		}
		value, ok := read(p)
		if !ok {
			continue
		}
		key := procKey{p.PID, p.Info.CreateTime}
		m.current[key] = value
		p.NetKnown = true
		if old, found := m.previous[key]; found {
			p.NetDelta = sub(value, old)
			if dt > 0 {
				p.Net = float32(float64(p.NetDelta) / dt)
			}
		}
	}
	m.previous, m.current = m.current, m.previous
}

// UsesEnergy reports whether the fallback is active.
func (m *NetMonitor) UsesEnergy() bool { return m.energy }

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
	if m.energy {
		m.energy = false
		m.previous, m.current = nil, nil
		return
	}
	m.stopSession()
	winapi.CloseTrace(m.trace)
}

// Take calls f with each process's bytes since the last call, and starts
// counting again.
func (m *NetMonitor) Take(f func(pid uint32, bytes uint64)) {
	if !m.Available || m.energy {
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
