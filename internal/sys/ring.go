// Package sys reads what the task manager shows from Windows: processes,
// processors, memory, disks, network adapters, GPUs, services, startup
// items, signed-in users and each process's network use. Each sample is a
// handful of system calls; what doesn't change is read once and kept.
package sys

// HistoryLen is how many samples a graph shows: the last minute, at one a
// second.
const HistoryLen = 60

// Ring is the last minute of a figure, a sample a slot, as Task Manager's
// graphs show it: oldest first, the newest at the right edge.
type Ring struct {
	v     [HistoryLen]float32
	head  int
	count int
}

// Push adds the newest sample.
func (r *Ring) Push(x float32) {
	r.v[r.head] = x
	r.head = (r.head + 1) % HistoryLen
	if r.count < HistoryLen {
		r.count++
	}
}

// Has reports whether slot i (0 is a minute ago) holds a sample.
func (r *Ring) Has(i int) bool { return i >= HistoryLen-r.count }

// At is the value i slots from the left edge; 0 where there's nothing yet.
func (r *Ring) At(i int) float32 {
	if !r.Has(i) {
		return 0
	}
	return r.v[(r.head+i)%HistoryLen]
}

// Last is the newest sample.
func (r *Ring) Last() float32 {
	if r.count == 0 {
		return 0
	}
	return r.v[(r.head+HistoryLen-1)%HistoryLen]
}

// Max is the largest sample.
func (r *Ring) Max() float32 {
	var m float32
	for _, x := range r.v {
		m = max(m, x)
	}
	return m
}
