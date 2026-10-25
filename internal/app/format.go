package app

import (
	"strconv"
)

// Figures as Task Manager writes them: memory in MB and GB, rates in MB/s
// and Kbps, times as d:hh:mm:ss. Each builds its text in a buffer on the
// stack and allocates only the string it returns.

func fixed(b []byte, v float64, prec int) []byte { return strconv.AppendFloat(b, v, 'f', prec, 64) }

// fmtBytes is "12.3 MB", "1.2 GB": one decimal place.
func fmtBytes(n uint64) string {
	units := [...]string{"B", "KB", "MB", "GB", "TB"}
	v, u := float64(n), 0
	for v >= 1024 && u+1 < len(units) {
		v /= 1024
		u++
	}
	var buf [32]byte
	if u == 0 {
		return string(append(strconv.AppendUint(buf[:0], n, 10), " B"...))
	}
	return string(append(append(fixed(buf[:0], v, 1), ' '), units[u]...))
}

// fmtMB is "123.4 MB".
func fmtMB(n uint64) string {
	var buf [32]byte
	return string(append(fixed(buf[:0], float64(n)/(1<<20), 1), " MB"...))
}

// fmtKB is the Details page's memory: "12,345 K".
func fmtKB(n uint64) string { return grouped(n/1024, " K") }

// appendGrouped writes n with its thousands separated: "1,234,567".
func appendGrouped(b []byte, n uint64) []byte {
	var digits [24]byte
	d := strconv.AppendUint(digits[:0], n, 10)
	for i, ch := range d {
		if i > 0 && (len(d)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, ch)
	}
	return b
}

func grouped(n uint64, suffix string) string {
	var buf [32]byte
	return string(append(appendGrouped(buf[:0], n), suffix...))
}

// fmtGB is "15.8 GB".
func fmtGB(n uint64) string {
	var buf [32]byte
	return string(append(fixed(buf[:0], float64(n)/(1<<30), 1), " GB"...))
}

// fmtRate is a disk's rate: "0 MB/s", "512 KB/s", "1.2 MB/s".
func fmtRate(bytesPerS float32) string {
	v := float64(bytesPerS)
	var buf [32]byte
	switch {
	case v < 1024:
		return "0 MB/s"
	case v < 0.05*1024*1024:
		return string(append(fixed(buf[:0], v/1024, 0), " KB/s"...))
	}
	return string(append(fixed(buf[:0], v/(1<<20), 1), " MB/s"...))
}

// fmtBits is a network rate: "0 Kbps", "8.0 Kbps", "1.2 Mbps".
func fmtBits(bitsPerS float32) string {
	v := float64(bitsPerS)
	var buf [32]byte
	switch {
	case v >= 1e9:
		return string(append(fixed(buf[:0], v/1e9, 1), " Gbps"...))
	case v >= 1e6:
		return string(append(fixed(buf[:0], v/1e6, 1), " Mbps"...))
	case v < 50:
		return "0 Kbps"
	}
	return string(append(fixed(buf[:0], v/1e3, 1), " Kbps"...))
}

func pad2(b []byte, n uint64) []byte {
	if n < 10 {
		b = append(b, '0')
	}
	return strconv.AppendUint(b, n, 10)
}

// fmtUptime is seconds as "d:hh:mm:ss".
func fmtUptime(secs uint64) string {
	var buf [32]byte
	b := strconv.AppendUint(buf[:0], secs/86400, 10)
	b = pad2(append(b, ':'), secs/3600%24)
	b = pad2(append(b, ':'), secs/60%60)
	b = pad2(append(b, ':'), secs%60)
	return string(b)
}

// fmtCPUTime is processor time (100 ns units) as "h:mm:ss".
func fmtCPUTime(t uint64) string {
	s := t / 10_000_000
	var buf [32]byte
	b := strconv.AppendUint(buf[:0], s/3600, 10)
	b = pad2(append(b, ':'), s/60%60)
	b = pad2(append(b, ':'), s%60)
	return string(b)
}

// fmtCache is a cache's size: "512 KB", "24.0 MB".
func fmtCache(n uint64) string {
	var buf [32]byte
	if n >= 1<<20 {
		return string(append(fixed(buf[:0], float64(n)/(1<<20), 1), " MB"...))
	}
	return string(append(strconv.AppendUint(buf[:0], n/1024, 10), " KB"...))
}

// fmtPercent is a whole percentage: "18%".
func fmtPercent(v float32) string {
	var buf [16]byte
	return string(append(fixed(buf[:0], float64(v), 0), '%'))
}

// cpuCell is the Processes page's CPU: "0%" when idle, else "0.8%".
func cpuCell(pct float32) string {
	if pct < 0.05 {
		return "0%"
	}
	var buf [16]byte
	return string(append(fixed(buf[:0], float64(pct), 1), '%'))
}

// mbCell is the Processes page's memory: "1,902.2 MB".
func mbCell(n uint64) string {
	tenths := (n*10 + 512*1024) / (1 << 20)
	var buf [32]byte
	b := appendGrouped(buf[:0], tenths/10)
	b = strconv.AppendUint(append(b, '.'), tenths%10, 10)
	return string(append(b, " MB"...))
}

// diskCell is the Processes page's disk: "0 MB/s", "0.1 MB/s".
func diskCell(bytesPerS float32) string {
	v := float64(bytesPerS) / (1 << 20)
	if v < 0.05 {
		return "0 MB/s"
	}
	var buf [32]byte
	return string(append(fixed(buf[:0], v, 1), " MB/s"...))
}

// netCell is the Processes page's network: "0 Mbps", "0.1 Mbps".
func netCell(bytesPerS float32) string {
	v := float64(bytesPerS) * 8 / 1e6
	if v < 0.05 {
		return "0 Mbps"
	}
	var buf [32]byte
	return string(append(fixed(buf[:0], v, 1), " Mbps"...))
}

// fmtMBShort is a size in whole MB: "512 MB".
func fmtMBShort(n uint64) string {
	var buf [32]byte
	return string(append(strconv.AppendUint(buf[:0], n/(1<<20), 10), " MB"...))
}

// mbTotal is a total of bytes as App history writes it: "0 MB", "12.3 MB",
// "1.2 GB".
func mbTotal(n uint64) string {
	v := float64(n) / (1 << 20)
	var buf [32]byte
	switch {
	case v < 0.05:
		return "0 MB"
	case v >= 1024:
		return string(append(fixed(buf[:0], v/1024, 1), " GB"...))
	}
	return string(append(fixed(buf[:0], v, 1), " MB"...))
}

// fmtGHz is "2.50 GHz".
func fmtGHz(ghz float32) string {
	var buf [24]byte
	return string(append(fixed(buf[:0], float64(ghz), 2), " GHz"...))
}

// itoa is n in decimal.
func itoa[T ~int | ~int32 | ~int64 | ~uint8 | ~uint16 | ~uint32 | ~uint64](n T) string {
	if n < 0 {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatUint(uint64(n), 10)
}

// lowerASCII folds an ASCII letter.
func lowerASCII(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

// containsFold reports whether needle is in hay, ignoring ASCII case,
// without allocating.
func containsFold(hay, needle string) bool {
	if needle == "" {
		return true
	}
	n := len(needle)
	first := lowerASCII(needle[0])
outer:
	for i := 0; i+n <= len(hay); i++ {
		if lowerASCII(hay[i]) != first {
			continue
		}
		for j := 1; j < n; j++ {
			if lowerASCII(hay[i+j]) != lowerASCII(needle[j]) {
				continue outer
			}
		}
		return true
	}
	return false
}

// cmpFold orders a and b ignoring ASCII case.
func cmpFold(a, b string) int {
	n := min(len(a), len(b))
	for i := range n {
		x, y := lowerASCII(a[i]), lowerASCII(b[i])
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

// pidHasPrefix reports whether pid, in decimal, starts with q.
func pidHasPrefix(pid uint32, q string) bool {
	var buf [12]byte
	d := strconv.AppendUint(buf[:0], uint64(pid), 10)
	if len(q) > len(d) {
		return false
	}
	return string(d[:len(q)]) == q
}
