package sys

import (
	"encoding/binary"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"taskmgr/internal/winapi"
)

// CPU is the processor, read once: its name, speed, cores and caches.
type CPU struct {
	Name           string
	BaseMHz        uint32
	Sockets        uint32
	Cores          uint32
	Logical        uint32
	L1, L2, L3     uint64
	Virtualization bool
}

// ReadCPU reads the processor's description.
func ReadCPU() CPU {
	var c CPU
	const key = `HARDWARE\DESCRIPTION\System\CentralProcessor\0`
	c.Name = strings.TrimSpace(winapi.RegString(registry.LOCAL_MACHINE, key, "ProcessorNameString"))
	c.BaseMHz, _ = winapi.RegDword(registry.LOCAL_MACHINE, key, "~MHz")
	c.Logical = windows.GetActiveProcessorCount(windows.ALL_PROCESSOR_GROUPS)
	// PF_VIRT_FIRMWARE_ENABLED.
	c.Virtualization = winapi.IsProcessorFeaturePresent(21)
	var n uint32
	winapi.GetLogicalProcessorInformationEx(0xffff, nil, &n)
	if n == 0 || n > 1<<20 {
		return c
	}
	buf := make([]byte, n)
	if !winapi.GetLogicalProcessorInformationEx(0xffff, unsafe.Pointer(&buf[0]), &n) {
		return c
	}
	for off := uint32(0); off+8 <= n; {
		rel := binary.LittleEndian.Uint32(buf[off:])
		size := binary.LittleEndian.Uint32(buf[off+4:])
		if size == 0 {
			break
		}
		body := buf[off+8:]
		switch rel {
		case 0: // RelationProcessorCore
			c.Cores++
		case 3: // RelationProcessorPackage
			c.Sockets++
		case 2: // RelationCache: level, associativity, line size, size.
			size := uint64(binary.LittleEndian.Uint32(body[4:]))
			switch body[0] {
			case 1:
				c.L1 += size
			case 2:
				c.L2 += size
			case 3:
				c.L3 += size
			}
		}
		off += size
	}
	return c
}

// MemoryHardware is the memory's speed, slots and form, from SMBIOS.
type MemoryHardware struct {
	Speed     uint32 // MT/s; 0 if SMBIOS doesn't say
	SlotsUsed uint32
	Slots     uint32
	Form      string
	Installed uint64
}

// ReadMemoryHardware reads the SMBIOS memory devices.
func ReadMemoryHardware() MemoryHardware {
	var m MemoryHardware
	if kb, ok := winapi.GetPhysicallyInstalledSystemMemory(); ok {
		m.Installed = kb * 1024
	}
	const sig = 0x52534D42 // 'RSMB'
	size := winapi.GetSystemFirmwareTable(sig, 0, nil, 0)
	if size < 8 || size > 1<<20 {
		return m
	}
	buf := make([]byte, size)
	if winapi.GetSystemFirmwareTable(sig, 0, unsafe.Pointer(&buf[0]), size) != size {
		return m
	}
	tlen := binary.LittleEndian.Uint32(buf[4:8])
	table := buf[8:min(int(size), 8+int(tlen))]
	for off := 0; off+4 <= len(table); {
		kind, flen := table[off], int(table[off+1])
		if flen < 4 || off+flen > len(table) {
			break
		}
		s := table[off : off+flen]
		if kind == 16 && flen >= 0x0F {
			m.Slots = max(m.Slots, uint32(binary.LittleEndian.Uint16(s[0x0D:])))
		}
		if kind == 17 && flen >= 0x17 {
			if sz := binary.LittleEndian.Uint16(s[0x0C:]); sz != 0 && sz != 0xFFFF {
				m.SlotsUsed++
				switch s[0x0E] {
				case 0x09:
					m.Form = "DIMM"
				case 0x0D:
					m.Form = "SODIMM"
				case 0x0F:
					m.Form = "FB-DIMM"
				case 0x0B:
					m.Form = "Row of chips"
				default:
					if m.Form == "" {
						m.Form = "Other"
					}
				}
				speed := uint32(binary.LittleEndian.Uint16(s[0x15:]))
				if flen >= 0x22 {
					if conf := binary.LittleEndian.Uint16(s[0x20:]); conf != 0 && conf != 0xFFFF {
						speed = uint32(conf)
					}
				}
				if speed != 0xFFFF {
					m.Speed = max(m.Speed, speed)
				}
			}
		}
		if kind == 127 {
			break
		}
		// The strings after the formatted part end with two NULs.
		j := off + flen
		for j+1 < len(table) && !(table[j] == 0 && table[j+1] == 0) {
			j++
		}
		off = j + 2
	}
	return m
}

// Memory is how the memory is used now (read each sample).
type Memory struct {
	Total, Available        uint64
	Commit, CommitLimit     uint64
	Cached                  uint64
	PagedPool, NonpagedPool uint64
	Processes               uint32
}

// Used is the memory in use.
func (m Memory) Used() uint64 {
	if m.Available > m.Total {
		return 0
	}
	return m.Total - m.Available
}

// Load is the share in use, 0–1.
func (m Memory) Load() float32 {
	if m.Total == 0 {
		return 0
	}
	return float32(m.Used()) / float32(m.Total)
}

// ReadMemory reads the memory's use now.
func ReadMemory() Memory {
	var ms winapi.MemoryStatusEx
	winapi.GlobalMemoryStatusEx(&ms)
	var pi winapi.PerformanceInfo
	winapi.GetPerformanceInfo(&pi)
	page := uint64(pi.PageSize)
	if page == 0 {
		page = 4096
	}
	return Memory{
		Total:        ms.TotalPhys,
		Available:    ms.AvailPhys,
		Commit:       uint64(pi.CommitTotal) * page,
		CommitLimit:  uint64(pi.CommitLimit) * page,
		Cached:       uint64(pi.SystemCache) * page,
		PagedPool:    uint64(pi.KernelPaged) * page,
		NonpagedPool: uint64(pi.KernelNonpaged) * page,
		Processes:    pi.ProcessCount,
	}
}

// Uptime is the seconds since the machine started.
func Uptime() uint64 { return winapi.GetTickCount64() / 1000 }

// BiosSeconds is how long the firmware took at the last boot (Startup
// apps' "Last BIOS time"); 0 if Windows doesn't say.
func BiosSeconds() float32 {
	ms, ok := winapi.RegDword(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Power`, "FwPOSTTime")
	if !ok {
		return 0
	}
	return float32(ms) / 1000
}
