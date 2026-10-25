package sys

import (
	"net/netip"
	"sort"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// Adapter is an Ethernet or Wi-Fi interface.
type Adapter struct {
	LUID        uint64
	Name        string // the alias ("Wi-Fi")
	Description string
	WiFi        bool
	Connected   bool
	LinkSpeed   uint64
	prevIn      uint64
	prevOut     uint64
	prevTime    time.Time
	// Send and Recv are bits a second.
	Send, Recv         float32
	HistSend, HistRecv Ring
	IPv4, IPv6         string
	seen               bool
}

// Kind is "Wi-Fi" or "Ethernet".
func (a *Adapter) Kind() string {
	if a.WiFi {
		return "Wi-Fi"
	}
	return "Ethernet"
}

// Net is the machine's network adapters: found with GetIfTable2 now and
// then, each read every sample with GetIfEntry2 (one row, not the whole
// table).
type Net struct {
	List    []*Adapter
	samples uint32
	row     windows.MibIfRow2
}

// FindNet finds the adapters.
func FindNet() *Net {
	n := &Net{}
	n.find()
	return n
}

// find finds the adapters again (they come and go: a dock, a VPN).
func (n *Net) find() {
	t, ok := winapi.GetIfTable2()
	if !ok {
		return
	}
	defer winapi.FreeMibTable(t)
	count := *(*uint32)(t)
	rows := unsafe.Slice((*windows.MibIfRow2)(unsafe.Add(t, 8)), count)
	for _, a := range n.List {
		a.seen = false
	}
	for i := range rows {
		r := &rows[i]
		// Hardware, not a filter's view of it; Ethernet or Wi-Fi.
		if r.InterfaceAndOperStatusFlags&1 == 0 || r.InterfaceAndOperStatusFlags&2 != 0 {
			continue
		}
		if r.Type != 6 && r.Type != 71 {
			continue
		}
		var a *Adapter
		for _, x := range n.List {
			if x.LUID == r.InterfaceLuid {
				a = x
				break
			}
		}
		if a == nil {
			a = &Adapter{LUID: r.InterfaceLuid}
			n.List = append(n.List, a)
		}
		a.seen = true
		a.WiFi = r.Type == 71
		a.Connected = r.OperStatus == 1
		a.LinkSpeed = r.ReceiveLinkSpeed
		a.Name = utf16Cached(a.Name, r.Alias[:])
		a.Description = utf16Cached(a.Description, r.Description[:])
	}
	keep := n.List[:0]
	for _, a := range n.List {
		if a.seen {
			keep = append(keep, a)
		}
	}
	clear(n.List[len(keep):])
	n.List = keep
	// Connected first, then by name, as Task Manager lists them.
	sort.SliceStable(n.List, func(i, j int) bool {
		x, y := n.List[i], n.List[j]
		if x.Connected != y.Connected {
			return x.Connected
		}
		return x.Name < y.Name
	})
}

// utf16Cached returns old if it already says what the NUL-terminated
// UTF-16 in s says, else a new string.
func utf16Cached(old string, s []uint16) string {
	n := 0
	for n < len(s) && s[n] != 0 {
		n++
	}
	if utf16Equal(s[:n], old) {
		return old
	}
	return windows.UTF16ToString(s[:n])
}

// Sample reads every adapter's counters.
func (n *Net) Sample(now time.Time) {
	n.samples++
	if n.samples%15 == 0 {
		n.find()
	}
	for _, a := range n.List {
		n.row = windows.MibIfRow2{InterfaceLuid: a.LUID}
		if !winapi.GetIfEntry2(&n.row) {
			continue
		}
		r := &n.row
		a.Connected = r.OperStatus == 1
		a.LinkSpeed = r.ReceiveLinkSpeed
		if !a.prevTime.IsZero() && now.After(a.prevTime) {
			dt := now.Sub(a.prevTime).Seconds()
			a.Recv = float32(float64(sub(r.InOctets, a.prevIn)) * 8 / dt)
			a.Send = float32(float64(sub(r.OutOctets, a.prevOut)) * 8 / dt)
		}
		a.prevIn, a.prevOut, a.prevTime = r.InOctets, r.OutOctets, now
		a.HistRecv.Push(a.Recv)
		a.HistSend.Push(a.Send)
	}
}

// Load is the network's use, 0–1: the busiest connected adapter's
// throughput against its link speed (the Network column's total).
func (n *Net) Load() float32 {
	var load float32
	for _, a := range n.List {
		if a.Connected && a.LinkSpeed > 0 {
			load = max(load, (a.Send+a.Recv)/float32(a.LinkSpeed))
		}
	}
	return min(1, load)
}

// AdapterAddress is an adapter's first IPv4 and IPv6 address.
type AdapterAddress struct {
	LUID       uint64
	IPv4, IPv6 string
}

// ReadAddresses reads each adapter's first IPv4 and IPv6 address. It can
// take a while (the TCP/IP stack's lock): call it off the UI thread.
func ReadAddresses() []AdapterAddress {
	size := uint32(32 * 1024)
	var buf []uint64
	for {
		buf = make([]uint64, (size+4095)/8)
		size = uint32(len(buf) * 8)
		// Skip anycast, multicast and DNS servers.
		err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0x2|0x4|0x8, 0, (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])), &size)
		if err == windows.ERROR_BUFFER_OVERFLOW {
			continue
		}
		if err != nil {
			return nil
		}
		break
	}
	var out []AdapterAddress
	for aa := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0])); aa != nil; aa = aa.Next {
		ad := AdapterAddress{LUID: aa.Luid}
		for u := aa.FirstUnicastAddress; u != nil; u = u.Next {
			sa := u.Address.Sockaddr
			if sa == nil {
				continue
			}
			switch sa.Addr.Family {
			case windows.AF_INET:
				if ad.IPv4 == "" {
					p := (*windows.RawSockaddrInet4)(unsafe.Pointer(sa))
					ad.IPv4 = netip.AddrFrom4(p.Addr).String()
				}
			case windows.AF_INET6:
				if ad.IPv6 == "" {
					p := (*windows.RawSockaddrInet6)(unsafe.Pointer(sa))
					ad.IPv6 = netip.AddrFrom16(p.Addr).String()
				}
			}
		}
		out = append(out, ad)
	}
	return out
}
