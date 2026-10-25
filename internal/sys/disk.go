package sys

import (
	"encoding/binary"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

const (
	ioctlDiskPerformance            = 0x70020
	ioctlStorageQueryProperty       = 0x2D1400
	ioctlDiskGetDriveGeometryEx     = 0x700A0
	ioctlVolumeGetVolumeDiskExtents = 0x560000
)

// diskPerf is DISK_PERFORMANCE.
type diskPerf struct {
	BytesRead, BytesWritten int64
	ReadTime, WriteTime     int64
	IdleTime                int64
	ReadCount, WriteCount   uint32
	QueueDepth, SplitCount  uint32
	QueryTime               int64
	DeviceNumber            uint32
	Manager                 [8]uint16
}

// Disk is a physical disk: found once, read every sample through
// IOCTL_DISK_PERFORMANCE (the counters Task Manager's come from).
type Disk struct {
	Number uint32
	handle windows.Handle
	Model  string
	// Kind is "SSD", "HDD" or "Removable"; "" if unknown. Bus is "NVMe",
	// "SATA", …
	Kind, Bus string
	// Volumes are its drive letters ("C: D:").
	Volumes   string
	System    bool
	Pagefile  bool
	Capacity  uint64
	Formatted uint64

	prev    diskPerf
	hasPrev bool
	// Active is the share of the time it was busy, 0–1.
	Active              float32
	ReadRate, WriteRate float32
	ResponseMS          float32
	HistActive          Ring
	HistRead, HistWrite Ring
}

// Title is "Disk 0 (C:)".
func (d *Disk) Title() string { return fmt.Sprintf("Disk %d (%s)", d.Number, d.Volumes) }

func (d *Disk) sample() {
	var p diskPerf
	if _, ok := winapi.DeviceIoControl(d.handle, ioctlDiskPerformance, nil, 0, unsafe.Pointer(&p), uint32(unsafe.Sizeof(p))); !ok {
		return
	}
	if d.hasPrev {
		dq := float64(max(1, p.QueryTime-d.prev.QueryTime))
		di := float64(max(0, p.IdleTime-d.prev.IdleTime))
		d.Active = float32(min(1, max(0, 1-di/dq)))
		secs := dq / 1e7
		d.ReadRate = float32(float64(max(0, p.BytesRead-d.prev.BytesRead)) / secs)
		d.WriteRate = float32(float64(max(0, p.BytesWritten-d.prev.BytesWritten)) / secs)
		ops := (p.ReadCount - d.prev.ReadCount) + (p.WriteCount - d.prev.WriteCount)
		t := (p.ReadTime - d.prev.ReadTime) + (p.WriteTime - d.prev.WriteTime)
		if ops > 0 {
			d.ResponseMS = float32(float64(max(0, t)) / float64(ops) / 1e4)
		} else {
			d.ResponseMS = 0
		}
		d.HistActive.Push(d.Active * 100)
		d.HistRead.Push(d.ReadRate)
		d.HistWrite.Push(d.WriteRate)
	}
	d.prev, d.hasPrev = p, true
}

// Disks are the machine's physical disks.
type Disks struct{ List []*Disk }

// FindDisks finds the disks whose counters can be read.
func FindDisks() *Disks {
	ds := &Disks{}
	misses := 0
	for n := uint32(0); n < 32 && misses < 4; n++ {
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(fmt.Sprintf(`\\.\PhysicalDrive%d`, n)), 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			misses++
			continue
		}
		var p diskPerf
		if _, ok := winapi.DeviceIoControl(h, ioctlDiskPerformance, nil, 0, unsafe.Pointer(&p), uint32(unsafe.Sizeof(p))); !ok {
			windows.CloseHandle(h)
			continue
		}
		d := &Disk{Number: n, handle: h}
		d.describe()
		ds.List = append(ds.List, d)
	}
	ds.mapVolumes()
	return ds
}

// Sample reads every disk's counters.
func (ds *Disks) Sample() {
	for _, d := range ds.List {
		d.sample()
	}
}

// Close closes the disks' handles.
func (ds *Disks) Close() {
	for _, d := range ds.List {
		windows.CloseHandle(d.handle)
	}
}

// Busiest is the busiest disk's active time, 0–1.
func (ds *Disks) Busiest() float32 {
	var m float32
	for _, d := range ds.List {
		m = max(m, d.Active)
	}
	return m
}

// mapVolumes finds which drive letters live on which disk, which holds
// Windows and a page file, and how much of each is formatted.
func (ds *Disks) mapVolumes() {
	sysLetter := byte('C')
	if dir, err := windows.GetSystemWindowsDirectory(); err == nil && len(dir) > 0 {
		sysLetter = strings.ToUpper(dir[:1])[0]
	}
	drives, _ := windows.GetLogicalDrives()
	for i := range 26 {
		if drives&(1<<i) == 0 {
			continue
		}
		letter := byte('A' + i)
		root := string(letter) + `:\`
		t := windows.GetDriveType(windows.StringToUTF16Ptr(root))
		if t != windows.DRIVE_REMOVABLE && t != windows.DRIVE_FIXED {
			continue
		}
		h, err := windows.CreateFile(windows.StringToUTF16Ptr(`\\.\`+string(letter)+":"), 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			continue
		}
		var ext [64]byte
		_, ok := winapi.DeviceIoControl(h, ioctlVolumeGetVolumeDiskExtents, nil, 0, unsafe.Pointer(&ext[0]), uint32(len(ext)))
		windows.CloseHandle(h)
		if !ok || binary.LittleEndian.Uint32(ext[0:]) == 0 {
			continue
		}
		disk := binary.LittleEndian.Uint32(ext[8:])
		for _, d := range ds.List {
			if d.Number != disk {
				continue
			}
			if d.Volumes != "" {
				d.Volumes += " "
			}
			d.Volumes += string(letter) + ":"
			if letter == sysLetter {
				d.System = true
			}
			if winapi.FileExists(string(letter) + `:\pagefile.sys`) {
				d.Pagefile = true
			}
			var total uint64
			if windows.GetDiskFreeSpaceEx(windows.StringToUTF16Ptr(root), nil, &total, nil) == nil {
				d.Formatted += total
			}
		}
	}
}

func (d *Disk) describe() {
	var geo [64]uint64
	if _, ok := winapi.DeviceIoControl(d.handle, ioctlDiskGetDriveGeometryEx, nil, 0, unsafe.Pointer(&geo[0]), 64); ok {
		d.Capacity = uint64(max(0, int64(geo[3])))
	}
	// STORAGE_PROPERTY_QUERY: the device's descriptor.
	q := [3]uint32{0, 0, 0}
	var descBuf [128]uint64
	desc := unsafe.Slice((*byte)(unsafe.Pointer(&descBuf[0])), 1024)
	if ret, ok := winapi.DeviceIoControl(d.handle, ioctlStorageQueryProperty, unsafe.Pointer(&q[0]), 12, unsafe.Pointer(&desc[0]), 1024); ok && ret >= 32 {
		vendor := binary.LittleEndian.Uint32(desc[12:])
		product := binary.LittleEndian.Uint32(desc[16:])
		bus := binary.LittleEndian.Uint32(desc[28:])
		var parts []string
		for _, o := range [2]uint32{vendor, product} {
			if o == 0 || o >= ret {
				continue
			}
			s := desc[o:ret]
			if i := indexByte(s, 0); i >= 0 {
				s = s[:i]
			}
			if t := strings.TrimSpace(string(s)); t != "" {
				parts = append(parts, t)
			}
		}
		d.Model = strings.Join(parts, " ")
		if desc[10] != 0 {
			d.Kind = "Removable"
		}
		switch bus {
		case 17:
			d.Bus = "NVMe"
		case 11:
			d.Bus = "SATA"
		case 7:
			d.Bus = "USB"
		case 10:
			d.Bus = "SAS"
		case 3:
			d.Bus = "ATA"
		case 8:
			d.Bus = "RAID"
		}
	}
	if d.Kind == "" {
		q[0] = 7 // StorageDeviceSeekPenaltyProperty
		var pen [4]uint32
		if ret, ok := winapi.DeviceIoControl(d.handle, ioctlStorageQueryProperty, unsafe.Pointer(&q[0]), 12, unsafe.Pointer(&pen[0]), 12); ok && ret >= 9 {
			if byte(pen[2]) != 0 {
				d.Kind = "HDD"
			} else {
				d.Kind = "SSD"
			}
		}
	}
}

func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// KindTitle is "SSD (NVMe)", or the kind alone.
func (d *Disk) KindTitle() string {
	if d.Bus != "" {
		return d.Kind + " (" + d.Bus + ")"
	}
	return d.Kind
}
