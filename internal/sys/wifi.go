package sys

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wlanDLL    = windows.NewLazySystemDLL("wlanapi.dll")
	wlanOpen   = wlanDLL.NewProc("WlanOpenHandle")
	wlanClose  = wlanDLL.NewProc("WlanCloseHandle")
	wlanEnum   = wlanDLL.NewProc("WlanEnumInterfaces")
	wlanQuery  = wlanDLL.NewProc("WlanQueryInterface")
	wlanFree   = wlanDLL.NewProc("WlanFreeMemory")
	guidToLUID = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("ConvertInterfaceGuidToLuid")
)

// These layouts follow WLAN_INTERFACE_INFO and WLAN_CONNECTION_ATTRIBUTES.
type wlanInterface struct {
	GUID        windows.GUID
	Description [256]uint16
	State       uint32
}

func wifiStandard(phy uint32) string {
	switch phy {
	case 1, 2:
		return "802.11"
	case 4:
		return "802.11a"
	case 5:
		return "802.11b"
	case 6:
		return "802.11g"
	case 7:
		return "802.11n"
	case 8:
		return "802.11ac"
	case 9:
		return "802.11ad"
	case 10:
		return "802.11ax"
	case 11:
		return "802.11be"
	default:
		return ""
	}
}

// readWiFi uses the location-independent connection quality query.
func readWiFi(addrs []AdapterAddress) {
	var version uint32
	var handle windows.Handle
	if r, _, _ := wlanOpen.Call(2, 0, uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&handle))); r != 0 {
		return
	}
	defer wlanClose.Call(uintptr(handle), 0)
	var list unsafe.Pointer
	if r, _, _ := wlanEnum.Call(uintptr(handle), 0, uintptr(unsafe.Pointer(&list))); r != 0 || list == nil {
		return
	}
	defer wlanFree.Call(uintptr(list))
	count := *(*uint32)(list)
	if count > 1024 {
		return
	}
	interfaces := unsafe.Slice((*wlanInterface)(unsafe.Add(list, 8)), int(count))
	for i := range interfaces {
		it := &interfaces[i]
		var luid uint64
		if r, _, _ := guidToLUID.Call(uintptr(unsafe.Pointer(&it.GUID)), uintptr(unsafe.Pointer(&luid))); r != 0 {
			continue
		}
		for j := range addrs {
			ad := &addrs[j]
			if ad.LUID != luid || it.State != 1 {
				continue
			}
			var size uint32
			var data unsafe.Pointer
			// Realtime connection quality omits location-sensitive information.
			const wlanRealtimeConnectionQuality = 19
			r, _, _ := wlanQuery.Call(uintptr(handle), uintptr(unsafe.Pointer(&it.GUID)), wlanRealtimeConnectionQuality, 0, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&data)), 0)
			if r == 0 && data != nil {
				if size >= 8 {
					quality := (*[2]uint32)(data)
					ad.WiFiStandard = wifiStandard(quality[0])
					ad.Signal = min(quality[1], 100)
					ad.WiFiDetails = true
				}
				wlanFree.Call(uintptr(data))
			}
			break
		}
	}
}
