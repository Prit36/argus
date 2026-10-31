package sys

import (
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// The GPU engine kinds Task Manager graphs, by their PDH names.
var engineKinds = [...]string{"3D", "Copy", "VideoEncode", "VideoDecode"}

// EngineKinds is the number of engine kinds graphed.
const EngineKinds = len(engineKinds)

// GPUKindOther is an engine of another kind (Compute, …).
const GPUKindOther = 0xff

// KindTitle is an engine kind as Task Manager writes it ("Video Decode").
func KindTitle(k uint8) string {
	switch k {
	case 0:
		return "3D"
	case 1:
		return "Copy"
	case 2:
		return "Video Encode"
	case 3:
		return "Video Decode"
	}
	return "Compute"
}

// ProcGPU is a process's use of the GPUs: its busiest engine.
type ProcGPU struct {
	Use     float32
	Adapter uint8
	Kind    uint8
}

// GPU is a graphics adapter: its name and memory from DXGI, once, and its
// engines' use and memory from PDH while the Performance page shows.
type GPU struct {
	Name           string
	luidLow        uint32
	luidHigh       int32
	DedicatedTotal uint64
	SharedTotal    uint64
	Dedicated      uint64
	Shared         uint64
	// Driver is the user-mode driver's version ("32.0.15.6094"), zero
	// if unknown.
	Driver [4]uint16
	// Use is the busiest engine's, 0–100.
	Use           float32
	Engines       [EngineKinds]float32
	Hist          Ring
	HistEngines   [EngineKinds]Ring
	HistDedicated Ring
	HistShared    Ring
}

// FindGPUs lists the machine's GPUs (not WARP or the basic display driver,
// each once).
func FindGPUs() []*GPU {
	var out []*GPU
	for _, a := range winapi.EnumAdapters() {
		d := &a.Desc
		// DXGI_ADAPTER_FLAG_SOFTWARE, or Microsoft's basic display driver.
		if d.Flags&2 != 0 || d.VendorID == 0x1414 {
			continue
		}
		dup := false
		for _, g := range out {
			if g.luidLow == d.LuidLow && g.luidHigh == d.LuidHigh {
				dup = true
			}
		}
		if dup {
			continue
		}
		out = append(out, &GPU{
			Name:           windows.UTF16ToString(d.Description[:]),
			luidLow:        d.LuidLow,
			luidHigh:       d.LuidHigh,
			DedicatedTotal: uint64(d.DedicatedVideoMemory),
			SharedTotal:    uint64(d.SharedSystemMemory),
			Driver:         a.Driver,
		})
	}
	return out
}

// gpuCounters are the Performance Data Helper's counters: the GPUs'
// engines and memory, and the processor's performance against its base
// speed. Collecting them costs more than the rest of a sample, so a
// worker does it only while they show (PerfWorker).
type gpuCounters struct {
	gpus     []gpuIdentity
	query    windows.Handle
	cPerf    windows.Handle
	cEngine  windows.Handle
	cDed     windows.Handle
	cShared  windows.Handle
	buf      []uint64
	baseline bool
	reading  GPUReading
}

type gpuIdentity struct {
	low  uint32
	high int32
}

// GPUReading is one collection of the counters.
type GPUReading struct {
	// Warm: collected at least twice, so the rates mean something.
	Warm bool
	// CPUPerf is the processor's performance against its base speed, in
	// percent.
	CPUPerf float32
	GPUs    []GPUFigures
	PerPID  map[uint32]ProcGPU
}

// GPUFigures are one GPU's figures in a reading.
type GPUFigures struct {
	Use               float32
	Engines           [EngineKinds]float32
	Dedicated, Shared uint64
}

func (g *gpuCounters) start() {
	if g.query != 0 {
		return
	}
	q, ok := winapi.PdhOpenQuery()
	if !ok {
		return
	}
	g.query = q
	g.cPerf, _ = winapi.PdhAddEnglishCounter(q, `\Processor Information(_Total)\% Processor Performance`)
	if len(g.gpus) > 0 {
		g.cEngine, _ = winapi.PdhAddEnglishCounter(q, `\GPU Engine(*)\Utilization Percentage`)
		g.cDed, _ = winapi.PdhAddEnglishCounter(q, `\GPU Adapter Memory(*)\Dedicated Usage`)
		g.cShared, _ = winapi.PdhAddEnglishCounter(q, `\GPU Adapter Memory(*)\Shared Usage`)
	}
	g.baseline = false
}

func (g *gpuCounters) stop() {
	if g.query != 0 {
		winapi.PdhCloseQuery(g.query)
	}
	g.query, g.cPerf, g.cEngine, g.cDed, g.cShared = 0, 0, 0, 0, 0
	g.baseline = false
	g.buf = nil // Large wildcard-instance buffers are unused while disabled.
}

// sample collects once: a fresh reading, its map and slice its own.
func (g *gpuCounters) sample() *GPUReading {
	g.start()
	if g.query == 0 || !winapi.PdhCollectQueryData(g.query) {
		return nil
	}
	// The first collection supplies the baseline; rates need a later one.
	r := &GPUReading{Warm: g.baseline, GPUs: make([]GPUFigures, len(g.gpus)), PerPID: make(map[uint32]ProcGPU, 32)}
	g.baseline = true
	if g.cPerf != 0 {
		var v winapi.PdhFmtCounterValue
		if winapi.PdhGetFormattedCounterValue(g.cPerf, &v) && v.Status <= 1 {
			r.CPUPerf = float32(v.Value)
		}
	}
	if g.cEngine != 0 {
		g.engines(r)
	}
	if g.cDed != 0 {
		g.memory(r, g.cDed, true)
	}
	if g.cShared != 0 {
		g.memory(r, g.cShared, false)
	}
	return r
}

// items reads a counter's instances.
func (g *gpuCounters) items(c windows.Handle) []winapi.PdhFmtCounterValueItem {
	for {
		bytes := unsafe.Slice((*byte)(unsafe.Pointer(unsafe.SliceData(g.buf))), len(g.buf)*8)
		st, size, count := winapi.PdhGetFormattedCounterArray(c, bytes)
		if st == winapi.PdhMoreData {
			g.buf = make([]uint64, (size+4096)/8+1)
			continue
		}
		if st != 0 || count == 0 {
			return nil
		}
		return unsafe.Slice((*winapi.PdhFmtCounterValueItem)(unsafe.Pointer(&g.buf[0])), count)
	}
}

// engines sums each engine's use over processes; an engine kind's use is
// its busiest engine's, the GPU's the busiest of all.
func (g *gpuCounters) engines(r *GPUReading) {
	// Engines by adapter and number: up to 64 an adapter.
	var sums [8][64]float32
	var kinds [8][64]uint8
	for a := range kinds {
		for e := range kinds[a] {
			kinds[a][e] = GPUKindOther
		}
	}
	var nameBuf [160]byte
	for _, it := range g.items(g.cEngine) {
		if it.Value.Status > 1 {
			continue
		}
		name := asciiName(nameBuf[:], it.Name)
		ai := g.adapterOf(name)
		eng := numberAfter(name, "_eng_")
		if ai < 0 || ai >= 8 || eng < 0 || eng >= 64 {
			continue
		}
		v := float32(it.Value.Value)
		sums[ai][eng] += v
		kind := uint8(GPUKindOther)
		if _, after, ok := strings.Cut(name, "_engtype_"); ok {
			for ki, ek := range engineKinds {
				if after == ek {
					kind = uint8(ki)
				}
			}
		}
		kinds[ai][eng] = kind
		// "pid_1234_luid_…": the process's busiest engine.
		if pid := numberAfter(name, "pid_"); pid >= 0 && v > 0 {
			if cur, ok := r.PerPID[uint32(pid)]; !ok || v > cur.Use {
				r.PerPID[uint32(pid)] = ProcGPU{Use: min(100, v), Adapter: uint8(ai), Kind: kind}
			}
		}
	}
	for ai := range r.GPUs {
		if ai >= 8 {
			break
		}
		f := &r.GPUs[ai]
		for e, v := range sums[ai] {
			u := min(100, v)
			f.Use = max(f.Use, u)
			if k := kinds[ai][e]; k != GPUKindOther {
				f.Engines[k] = max(f.Engines[k], u)
			}
		}
	}
}

func (g *gpuCounters) memory(r *GPUReading, c windows.Handle, dedicated bool) {
	var nameBuf [160]byte
	for _, it := range g.items(c) {
		if it.Value.Status > 1 {
			continue
		}
		ai := g.adapterOf(asciiName(nameBuf[:], it.Name))
		if ai < 0 {
			continue
		}
		v := uint64(max(0, it.Value.Value))
		if dedicated {
			r.GPUs[ai].Dedicated += v
		} else {
			r.GPUs[ai].Shared += v
		}
	}
}

// asciiName copies a counter instance's name into buf (its characters
// are ASCII): no allocation per instance.
func asciiName(buf []byte, p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for s := unsafe.Pointer(p); n < len(buf); n++ {
		c := *(*uint16)(unsafe.Add(s, n*2))
		if c == 0 {
			break
		}
		buf[n] = byte(c)
	}
	return unsafe.String(&buf[0], n)
}

// adapterOf is the adapter an instance ("…luid_0x00000000_0x0000D3B2_phys_0…")
// is of, or -1.
func (g *gpuCounters) adapterOf(name string) int {
	_, after, ok := strings.Cut(name, "luid_0x")
	if !ok {
		return -1
	}
	s := after
	if len(s) < 19 {
		return -1
	}
	high, err1 := strconv.ParseUint(s[0:8], 16, 32)
	low, err2 := strconv.ParseUint(s[11:19], 16, 32)
	if err1 != nil || err2 != nil {
		return -1
	}
	for i, a := range g.gpus {
		if a.low == uint32(low) && uint32(a.high) == uint32(high) {
			return i
		}
	}
	return -1
}

// numberAfter is the number following tag in s, or -1.
func numberAfter(s, tag string) int {
	at := strings.Index(s, tag)
	if at < 0 {
		return -1
	}
	n, i := 0, at+len(tag)
	start := i
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		n = n*10 + int(s[i]-'0')
		i++
	}
	if i == start {
		return -1
	}
	return n
}
