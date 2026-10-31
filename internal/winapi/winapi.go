// Package winapi declares the Win32 and NT calls the task manager makes
// that golang.org/x/sys/windows lacks, the structures they fill, and a few
// helpers around UTF-16 and the registry. Every call goes through a lazily
// loaded system DLL: nothing here needs cgo.
package winapi

import (
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	user32   = windows.NewLazySystemDLL("user32.dll")
	advapi32 = windows.NewLazySystemDLL("advapi32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	gdi32    = windows.NewLazySystemDLL("gdi32.dll")
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	pdh      = windows.NewLazySystemDLL("pdh.dll")
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")
	dwmapi   = windows.NewLazySystemDLL("dwmapi.dll")
	dxgi     = windows.NewLazySystemDLL("dxgi.dll")
	dbghelp  = windows.NewLazySystemDLL("dbghelp.dll")
	netapi32 = windows.NewLazySystemDLL("netapi32.dll")

	procNtQuerySystemInformation     = ntdll.NewProc("NtQuerySystemInformation")
	procNtQueryInformationProcess    = ntdll.NewProc("NtQueryInformationProcess")
	procGlobalMemoryStatusEx         = kernel32.NewProc("GlobalMemoryStatusEx")
	procK32GetPerformanceInfo        = kernel32.NewProc("K32GetPerformanceInfo")
	procGetPhysicallyInstalledMemory = kernel32.NewProc("GetPhysicallyInstalledSystemMemory")
	procGetTickCount64               = kernel32.NewProc("GetTickCount64")
	procGetProcessInformation        = kernel32.NewProc("GetProcessInformation")
	procSetProcessInformation        = kernel32.NewProc("SetProcessInformation")
	procGetLogicalProcessorInfoEx    = kernel32.NewProc("GetLogicalProcessorInformationEx")
	procIsProcessorFeaturePresent    = kernel32.NewProc("IsProcessorFeaturePresent")
	procGetSystemFirmwareTable       = kernel32.NewProc("GetSystemFirmwareTable")
	procDeviceIoControl              = kernel32.NewProc("DeviceIoControl")
	procGetProcessAffinityMask       = kernel32.NewProc("GetProcessAffinityMask")
	procSetProcessAffinityMask       = kernel32.NewProc("SetProcessAffinityMask")
	procInternalGetWindowText        = user32.NewProc("InternalGetWindowText")
	procGetClassNameW                = user32.NewProc("GetClassNameW")
	procGetWindow                    = user32.NewProc("GetWindow")
	procGetWindowLongPtrW            = user32.NewProc("GetWindowLongPtrW")
	procIsHungAppWindow              = user32.NewProc("IsHungAppWindow")
	procIsIconic                     = user32.NewProc("IsIconic")
	procShowWindow                   = user32.NewProc("ShowWindow")
	procSetForegroundWindow          = user32.NewProc("SetForegroundWindow")
	procGetWindowTextLengthW         = user32.NewProc("GetWindowTextLengthW")
	procGetIconInfo                  = user32.NewProc("GetIconInfo")
	procDestroyIcon                  = user32.NewProc("DestroyIcon")
	procGetDC                        = user32.NewProc("GetDC")
	procReleaseDC                    = user32.NewProc("ReleaseDC")
	procGetDpiForWindow              = user32.NewProc("GetDpiForWindow")
	procGetClientRect                = user32.NewProc("GetClientRect")
	procGetWindowRect                = user32.NewProc("GetWindowRect")
	procSetWindowPos                 = user32.NewProc("SetWindowPos")
	procAllowSetForegroundWindow     = user32.NewProc("AllowSetForegroundWindow")
	procGetDIBits                    = gdi32.NewProc("GetDIBits")
	procDeleteObject                 = gdi32.NewProc("DeleteObject")
	procShellExecuteExW              = shell32.NewProc("ShellExecuteExW")
	procSHGetStockIconInfo           = shell32.NewProc("SHGetStockIconInfo")
	procSHDefExtractIconW            = shell32.NewProc("SHDefExtractIconW")
	procLookupAccountSidW            = advapi32.NewProc("LookupAccountSidW")
	procStartTraceW                  = advapi32.NewProc("StartTraceW")
	procControlTraceW                = advapi32.NewProc("ControlTraceW")
	procEnableTraceEx2               = advapi32.NewProc("EnableTraceEx2")
	procOpenTraceW                   = advapi32.NewProc("OpenTraceW")
	procProcessTrace                 = advapi32.NewProc("ProcessTrace")
	procCloseTrace                   = advapi32.NewProc("CloseTrace")
	procGetIfTable2                  = iphlpapi.NewProc("GetIfTable2")
	procGetIfEntry2                  = iphlpapi.NewProc("GetIfEntry2")
	procFreeMibTable                 = iphlpapi.NewProc("FreeMibTable")
	procPdhOpenQueryW                = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglishCounterW        = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollectQueryData          = pdh.NewProc("PdhCollectQueryData")
	procPdhGetFormattedCounterValue  = pdh.NewProc("PdhGetFormattedCounterValue")
	procPdhGetFormattedCounterArrayW = pdh.NewProc("PdhGetFormattedCounterArrayW")
	procPdhCloseQuery                = pdh.NewProc("PdhCloseQuery")
	procWTSQuerySessionInformationW  = wtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSDisconnectSession         = wtsapi32.NewProc("WTSDisconnectSession")
	procWTSLogoffSession             = wtsapi32.NewProc("WTSLogoffSession")
	procDwmGetWindowAttribute        = dwmapi.NewProc("DwmGetWindowAttribute")
	procCreateDXGIFactory1           = dxgi.NewProc("CreateDXGIFactory1")
	procMiniDumpWriteDump            = dbghelp.NewProc("MiniDumpWriteDump")
	procNetUserGetInfo               = netapi32.NewProc("NetUserGetInfo")
	procNetApiBufferFree             = netapi32.NewProc("NetApiBufferFree")
)

// Information classes of NtQuerySystemInformation.
const (
	SystemProcessInformation                     = 5
	SystemProcessorPerformanceInformation        = 8
	SystemProcessIDInformation                   = 88
	StatusInfoLengthMismatch              uint32 = 0xC0000004
)

// UnicodeString is the NT counted string.
type UnicodeString struct {
	Length    uint16
	MaxLength uint16
	Buffer    *uint16
}

// Slice is the string's UTF-16 code units (without copying them).
func (s UnicodeString) Slice() []uint16 {
	if s.Buffer == nil || s.Length == 0 {
		return nil
	}
	return unsafe.Slice(s.Buffer, s.Length/2)
}

// SystemProcessInformation is SYSTEM_PROCESS_INFORMATION, 0x100 bytes,
// followed by its threads.
type SystemProcessInfo struct {
	NextEntryOffset    uint32
	NumberOfThreads    uint32
	PrivateWorkingSet  int64
	HardFaultCount     uint32
	ThreadsHighWater   uint32
	CycleTime          uint64
	CreateTime         int64
	UserTime           int64
	KernelTime         int64
	ImageName          UnicodeString
	BasePriority       int32
	UniqueProcessID    uintptr
	InheritedFromPID   uintptr
	HandleCount        uint32
	SessionID          uint32
	UniqueProcessKey   uintptr
	PeakVirtualSize    uintptr
	VirtualSize        uintptr
	PageFaultCount     uint32
	PeakWorkingSetSize uintptr
	WorkingSetSize     uintptr
	QuotaPeakPaged     uintptr
	QuotaPaged         uintptr
	QuotaPeakNonPaged  uintptr
	QuotaNonPaged      uintptr
	PagefileUsage      uintptr
	PeakPagefileUsage  uintptr
	PrivatePageCount   uintptr
	ReadOperations     int64
	WriteOperations    int64
	OtherOperations    int64
	ReadBytes          int64
	WriteBytes         int64
	OtherBytes         int64
}

// SystemThreadInfo is SYSTEM_THREAD_INFORMATION, 0x50 bytes.
type SystemThreadInfo struct {
	KernelTime    int64
	UserTime      int64
	CreateTime    int64
	WaitTime      uint32
	StartAddress  uintptr
	ProcessID     uintptr
	ThreadID      uintptr
	Priority      int32
	BasePriority  int32
	ContextSwitch uint32
	State         uint32
	WaitReason    uint32
}

// ProcessorPerformance is SYSTEM_PROCESSOR_PERFORMANCE_INFORMATION.
type ProcessorPerformance struct {
	Idle       int64
	Kernel     int64
	User       int64
	DPC        int64
	Interrupt  int64
	Interrupts uint32
	_          uint32
}

// SystemProcessIDInfo is SYSTEM_PROCESS_ID_INFORMATION.
type SystemProcessIDInfo struct {
	PID  uintptr
	Name UnicodeString
}

var _ [0x100]byte = [unsafe.Sizeof(SystemProcessInfo{})]byte{}
var _ [0x50]byte = [unsafe.Sizeof(SystemThreadInfo{})]byte{}
var _ [48]byte = [unsafe.Sizeof(ProcessorPerformance{})]byte{}

// NtQuerySystemInformation returns the NTSTATUS (0 is success).
func NtQuerySystemInformation(class uint32, buf unsafe.Pointer, size uint32, ret *uint32) uint32 {
	r, _, _ := syscall.SyscallN(procNtQuerySystemInformation.Addr(), uintptr(class), uintptr(buf), uintptr(size), uintptr(unsafe.Pointer(ret)))
	return uint32(r)
}

// NtQueryInformationProcess returns the NTSTATUS (0 is success).
func NtQueryInformationProcess(h windows.Handle, class uint32, buf unsafe.Pointer, size uint32) uint32 {
	r, _, _ := syscall.SyscallN(procNtQueryInformationProcess.Addr(), uintptr(h), uintptr(class), uintptr(buf), uintptr(size), 0)
	return uint32(r)
}

// MemoryStatusEx is MEMORYSTATUSEX.
type MemoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func GlobalMemoryStatusEx(m *MemoryStatusEx) bool {
	m.Length = uint32(unsafe.Sizeof(*m))
	r, _, _ := syscall.SyscallN(procGlobalMemoryStatusEx.Addr(), uintptr(unsafe.Pointer(m)))
	return r != 0
}

// PerformanceInfo is PERFORMANCE_INFORMATION.
type PerformanceInfo struct {
	CB                uint32
	CommitTotal       uintptr
	CommitLimit       uintptr
	CommitPeak        uintptr
	PhysicalTotal     uintptr
	PhysicalAvailable uintptr
	SystemCache       uintptr
	KernelTotal       uintptr
	KernelPaged       uintptr
	KernelNonpaged    uintptr
	PageSize          uintptr
	HandleCount       uint32
	ProcessCount      uint32
	ThreadCount       uint32
}

func GetPerformanceInfo(p *PerformanceInfo) bool {
	p.CB = uint32(unsafe.Sizeof(*p))
	r, _, _ := syscall.SyscallN(procK32GetPerformanceInfo.Addr(), uintptr(unsafe.Pointer(p)), uintptr(p.CB))
	return r != 0
}

func GetPhysicallyInstalledSystemMemory() (kb uint64, ok bool) {
	r, _, _ := syscall.SyscallN(procGetPhysicallyInstalledMemory.Addr(), uintptr(unsafe.Pointer(&kb)))
	return kb, r != 0
}

func GetTickCount64() uint64 {
	r, _, _ := syscall.SyscallN(procGetTickCount64.Addr())
	return uint64(r)
}

func GetProcessInformation(h windows.Handle, class uint32, info unsafe.Pointer, size uint32) bool {
	r, _, _ := syscall.SyscallN(procGetProcessInformation.Addr(), uintptr(h), uintptr(class), uintptr(info), uintptr(size))
	return r != 0
}

func SetProcessInformation(h windows.Handle, class uint32, info unsafe.Pointer, size uint32) bool {
	r, _, _ := syscall.SyscallN(procSetProcessInformation.Addr(), uintptr(h), uintptr(class), uintptr(info), uintptr(size))
	return r != 0
}

func GetLogicalProcessorInformationEx(rel uint32, buf unsafe.Pointer, size *uint32) bool {
	r, _, _ := syscall.SyscallN(procGetLogicalProcessorInfoEx.Addr(), uintptr(rel), uintptr(buf), uintptr(unsafe.Pointer(size)))
	return r != 0
}

func IsProcessorFeaturePresent(f uint32) bool {
	r, _, _ := syscall.SyscallN(procIsProcessorFeaturePresent.Addr(), uintptr(f))
	return r != 0
}

func GetSystemFirmwareTable(sig, id uint32, buf unsafe.Pointer, size uint32) uint32 {
	r, _, _ := syscall.SyscallN(procGetSystemFirmwareTable.Addr(), uintptr(sig), uintptr(id), uintptr(buf), uintptr(size))
	return uint32(r)
}

// DeviceIoControl, without the error x/sys builds: a sample makes one per
// disk every second, and a failure is only a missed sample.
func DeviceIoControl(h windows.Handle, code uint32, in unsafe.Pointer, inLen uint32, out unsafe.Pointer, outLen uint32) (uint32, bool) {
	var ret uint32
	r, _, _ := syscall.SyscallN(procDeviceIoControl.Addr(), uintptr(h), uintptr(code), uintptr(in), uintptr(inLen), uintptr(out), uintptr(outLen), uintptr(unsafe.Pointer(&ret)), 0)
	return ret, r != 0
}

func GetProcessAffinityMask(h windows.Handle) (process, system uintptr, ok bool) {
	r, _, _ := syscall.SyscallN(procGetProcessAffinityMask.Addr(), uintptr(h), uintptr(unsafe.Pointer(&process)), uintptr(unsafe.Pointer(&system)))
	return process, system, r != 0
}

func SetProcessAffinityMask(h windows.Handle, mask uintptr) bool {
	r, _, _ := syscall.SyscallN(procSetProcessAffinityMask.Addr(), uintptr(h), mask)
	return r != 0
}

// ---------------------------------------------------------------- windows

const (
	GwOwner        = 4
	GwlExStyle     = -20
	WsExToolWindow = 0x00000080
	SwShow         = 5
	SwMinimize     = 6
	SwRestore      = 9
)

// InternalGetWindowText reads a window's title without sending it a
// message: a hung window can't stall the caller.
func InternalGetWindowText(h windows.HWND, buf []uint16) int {
	r, _, _ := syscall.SyscallN(procInternalGetWindowText.Addr(), uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return int(int32(r))
}

func GetClassName(h windows.HWND, buf []uint16) int {
	r, _, _ := syscall.SyscallN(procGetClassNameW.Addr(), uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return int(int32(r))
}

func GetWindow(h windows.HWND, cmd uint32) windows.HWND {
	r, _, _ := syscall.SyscallN(procGetWindow.Addr(), uintptr(h), uintptr(cmd))
	return windows.HWND(r)
}

func GetWindowLongPtr(h windows.HWND, index int32) uintptr {
	r, _, _ := syscall.SyscallN(procGetWindowLongPtrW.Addr(), uintptr(h), uintptr(index))
	return r
}

func IsHungAppWindow(h windows.HWND) bool {
	r, _, _ := syscall.SyscallN(procIsHungAppWindow.Addr(), uintptr(h))
	return r != 0
}

func IsIconic(h windows.HWND) bool {
	r, _, _ := syscall.SyscallN(procIsIconic.Addr(), uintptr(h))
	return r != 0
}

func ShowWindow(h windows.HWND, cmd int32) {
	syscall.SyscallN(procShowWindow.Addr(), uintptr(h), uintptr(cmd))
}

func SetForegroundWindow(h windows.HWND) bool {
	r, _, _ := syscall.SyscallN(procSetForegroundWindow.Addr(), uintptr(h))
	return r != 0
}

// AllowSetForegroundWindow lets process pid (ASFW_ANY: any) take the
// foreground, as Explorer needs to after we start it.
func AllowSetForegroundWindow(pid uint32) {
	syscall.SyscallN(procAllowSetForegroundWindow.Addr(), uintptr(pid))
}

func GetWindowTextLength(h windows.HWND) int {
	r, _, _ := syscall.SyscallN(procGetWindowTextLengthW.Addr(), uintptr(h))
	return int(int32(r))
}

func GetDpiForWindow(h windows.HWND) uint32 {
	r, _, _ := syscall.SyscallN(procGetDpiForWindow.Addr(), uintptr(h))
	return uint32(r)
}

// Rect is a RECT.
type Rect struct{ Left, Top, Right, Bottom int32 }

func GetClientRect(h windows.HWND) (r Rect, ok bool) {
	ret, _, _ := syscall.SyscallN(procGetClientRect.Addr(), uintptr(h), uintptr(unsafe.Pointer(&r)))
	return r, ret != 0
}

func GetWindowRect(h windows.HWND) (r Rect, ok bool) {
	ret, _, _ := syscall.SyscallN(procGetWindowRect.Addr(), uintptr(h), uintptr(unsafe.Pointer(&r)))
	return r, ret != 0
}

// MoveWindow places and sizes a window without activating it or changing
// its order.
func MoveWindow(h windows.HWND, x, y, w, ht int32) {
	const swpNoZOrder, swpNoActivate = 0x0004, 0x0010
	syscall.SyscallN(procSetWindowPos.Addr(), uintptr(h), 0, uintptr(x), uintptr(y), uintptr(w), uintptr(ht), swpNoZOrder|swpNoActivate)
}

// DwmCloaked reports whether the shell keeps the window hidden (a
// suspended Store app's frame).
func DwmCloaked(h windows.HWND) bool {
	var cloaked uint32
	r, _, _ := syscall.SyscallN(procDwmGetWindowAttribute.Addr(), uintptr(h), 14, uintptr(unsafe.Pointer(&cloaked)), 4)
	return r == 0 && cloaked != 0
}

// ---------------------------------------------------------------- icons

type IconInfo struct {
	Icon  int32
	XHot  uint32
	YHot  uint32
	Mask  windows.Handle
	Color windows.Handle
}

type BitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
	_             [4]uint32 // room for the color table GetDIBits may write
}

func GetIconInfo(icon windows.Handle, info *IconInfo) bool {
	r, _, _ := syscall.SyscallN(procGetIconInfo.Addr(), uintptr(icon), uintptr(unsafe.Pointer(info)))
	return r != 0
}

func DestroyIcon(icon windows.Handle) {
	syscall.SyscallN(procDestroyIcon.Addr(), uintptr(icon))
}

func DeleteObject(h windows.Handle) {
	syscall.SyscallN(procDeleteObject.Addr(), uintptr(h))
}

func GetDC(h windows.HWND) windows.Handle {
	r, _, _ := syscall.SyscallN(procGetDC.Addr(), uintptr(h))
	return windows.Handle(r)
}

func ReleaseDC(h windows.HWND, dc windows.Handle) {
	syscall.SyscallN(procReleaseDC.Addr(), uintptr(h), uintptr(dc))
}

func GetDIBits(dc, bitmap windows.Handle, lines uint32, bits unsafe.Pointer, info *BitmapInfoHeader) int {
	r, _, _ := syscall.SyscallN(procGetDIBits.Addr(), uintptr(dc), uintptr(bitmap), 0, uintptr(lines), uintptr(bits), uintptr(unsafe.Pointer(info)), 0)
	return int(int32(r))
}

// StockIconInfo is SHSTOCKICONINFO.
type StockIconInfo struct {
	Size      uint32
	Icon      windows.Handle
	SysIndex  int32
	IconIndex int32
	Path      [260]uint16
}

const SIIDApplication = 2

func SHGetStockIconLocation(id uint32, info *StockIconInfo) bool {
	info.Size = uint32(unsafe.Sizeof(*info))
	r, _, _ := syscall.SyscallN(procSHGetStockIconInfo.Addr(), uintptr(id), 0, uintptr(unsafe.Pointer(info)))
	return r == 0
}

// SHDefExtractIcon extracts icon `index` of `file` at `px` pixels.
func SHDefExtractIcon(file *uint16, index int32, px uint32) windows.Handle {
	var large windows.Handle
	r, _, _ := syscall.SyscallN(procSHDefExtractIconW.Addr(), uintptr(unsafe.Pointer(file)), uintptr(index), 0, uintptr(unsafe.Pointer(&large)), 0, uintptr(px))
	if r != 0 {
		return 0
	}
	return large
}

// ---------------------------------------------------------------- shell

// ShellExecuteInfo is SHELLEXECUTEINFOW.
type ShellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Hwnd       windows.HWND
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	InstApp    windows.Handle
	IDList     uintptr
	Class      *uint16
	KeyClass   windows.Handle
	HotKey     uint32
	IconOrMon  windows.Handle
	Process    windows.Handle
}

const (
	SeeMaskInvokeIDList = 0x0000000C
	SeeMaskNoCloseProc  = 0x00000040
	SeeMaskFlagNoUI     = 0x00000400
)

// ShellExecute runs `file` with `verb` ("" for the default) and `params`
// through the shell, as Explorer and the Run dialog do.
func ShellExecute(verb, file, params string, mask uint32) bool {
	info := ShellExecuteInfo{Mask: mask, Show: SwShow}
	info.Size = uint32(unsafe.Sizeof(info))
	if verb != "" {
		info.Verb = windows.StringToUTF16Ptr(verb)
	}
	info.File = windows.StringToUTF16Ptr(file)
	if params != "" {
		info.Parameters = windows.StringToUTF16Ptr(params)
	}
	r, _, _ := syscall.SyscallN(procShellExecuteExW.Addr(), uintptr(unsafe.Pointer(&info)))
	return r != 0
}

// ---------------------------------------------------------------- accounts

// LookupAccountSid returns the account name of a SID.
func LookupAccountSid(sid *windows.SID) (string, bool) {
	var name, dom [256]uint16
	nl, dl := uint32(len(name)), uint32(len(dom))
	var use uint32
	r, _, _ := syscall.SyscallN(procLookupAccountSidW.Addr(), 0, uintptr(unsafe.Pointer(sid)), uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&nl)), uintptr(unsafe.Pointer(&dom[0])), uintptr(unsafe.Pointer(&dl)), uintptr(unsafe.Pointer(&use)))
	if r == 0 {
		return "", false
	}
	return windows.UTF16ToString(name[:nl]), true
}

// NetUserFullName is a local account's full name (USER_INFO_10's), or "".
func NetUserFullName(user string) string {
	var buf unsafe.Pointer
	r, _, _ := syscall.SyscallN(procNetUserGetInfo.Addr(), 0, uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(user))), 10, uintptr(unsafe.Pointer(&buf)))
	if r != 0 || buf == nil {
		return ""
	}
	defer syscall.SyscallN(procNetApiBufferFree.Addr(), uintptr(buf))
	// USER_INFO_10: name, comment, user comment, full name.
	fields := (*[4]*uint16)(buf)
	if fields[3] == nil {
		return ""
	}
	return windows.UTF16PtrToString(fields[3])
}

// ---------------------------------------------------------------- sessions

const (
	WTSUserName   = 5
	WTSDomainName = 7
)

// WTSQueryString reads a session's string (user, domain).
func WTSQueryString(session, class uint32) string {
	var buf *uint16
	var n uint32
	r, _, _ := syscall.SyscallN(procWTSQuerySessionInformationW.Addr(), 0, uintptr(session), uintptr(class), uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 || buf == nil {
		return ""
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf)))
	return windows.UTF16PtrToString(buf)
}

func WTSDisconnectSession(session uint32) bool {
	r, _, _ := syscall.SyscallN(procWTSDisconnectSession.Addr(), 0, uintptr(session), 0)
	return r != 0
}

func WTSLogoffSession(session uint32) bool {
	r, _, _ := syscall.SyscallN(procWTSLogoffSession.Addr(), 0, uintptr(session), 0)
	return r != 0
}

// ---------------------------------------------------------------- network

// GetIfTable2 returns the interface table; free it with FreeMibTable.
func GetIfTable2() (unsafe.Pointer, bool) {
	var t unsafe.Pointer
	r, _, _ := syscall.SyscallN(procGetIfTable2.Addr(), uintptr(unsafe.Pointer(&t)))
	return t, r == 0 && t != nil
}

func FreeMibTable(t unsafe.Pointer) {
	syscall.SyscallN(procFreeMibTable.Addr(), uintptr(t))
}

// GetIfEntry2 fills one interface's row (its LUID set).
func GetIfEntry2(row *windows.MibIfRow2) bool {
	r, _, _ := syscall.SyscallN(procGetIfEntry2.Addr(), uintptr(unsafe.Pointer(row)))
	return r == 0
}

// ---------------------------------------------------------------- pdh

const (
	PdhFmtDouble   = 0x00000200
	PdhFmtNoCap100 = 0x00008000
	PdhMoreData    = 0x800007D2
)

type PdhFmtCounterValue struct {
	Status uint32
	_      uint32
	Value  float64
}

type PdhFmtCounterValueItem struct {
	Name  *uint16
	Value PdhFmtCounterValue
}

func PdhOpenQuery() (windows.Handle, bool) {
	var q windows.Handle
	r, _, _ := syscall.SyscallN(procPdhOpenQueryW.Addr(), 0, 0, uintptr(unsafe.Pointer(&q)))
	return q, r == 0
}

func PdhAddEnglishCounter(q windows.Handle, path string) (windows.Handle, bool) {
	var c windows.Handle
	r, _, _ := syscall.SyscallN(procPdhAddEnglishCounterW.Addr(), uintptr(q), uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(path))), 0, uintptr(unsafe.Pointer(&c)))
	return c, r == 0
}

func PdhCollectQueryData(q windows.Handle) bool {
	r, _, _ := syscall.SyscallN(procPdhCollectQueryData.Addr(), uintptr(q))
	return r == 0
}

func PdhGetFormattedCounterValue(c windows.Handle, v *PdhFmtCounterValue) bool {
	r, _, _ := syscall.SyscallN(procPdhGetFormattedCounterValue.Addr(), uintptr(c), PdhFmtDouble|PdhFmtNoCap100, 0, uintptr(unsafe.Pointer(v)))
	return r == 0
}

// PdhGetFormattedCounterArray fills buf with the counter's instances; the
// status, the bytes needed and the count of items.
func PdhGetFormattedCounterArray(c windows.Handle, buf []byte) (status uint32, size, count uint32) {
	size = uint32(len(buf))
	var p uintptr
	if len(buf) > 0 {
		p = uintptr(unsafe.Pointer(&buf[0]))
	}
	r, _, _ := syscall.SyscallN(procPdhGetFormattedCounterArrayW.Addr(), uintptr(c), PdhFmtDouble|PdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), p)
	return uint32(r), size, count
}

func PdhCloseQuery(q windows.Handle) {
	syscall.SyscallN(procPdhCloseQuery.Addr(), uintptr(q))
}

// ---------------------------------------------------------------- dxgi

// DXGIAdapterDesc1 is DXGI_ADAPTER_DESC1.
type DXGIAdapterDesc1 struct {
	Description           [128]uint16
	VendorID              uint32
	DeviceID              uint32
	SubSysID              uint32
	Revision              uint32
	DedicatedVideoMemory  uintptr
	DedicatedSystemMemory uintptr
	SharedSystemMemory    uintptr
	LuidLow               uint32
	LuidHigh              int32
	Flags                 uint32
}

var (
	iidIDXGIFactory1 = windows.GUID{Data1: 0x770aae78, Data2: 0xf26f, Data3: 0x4dba, Data4: [8]byte{0xa8, 0x29, 0x25, 0x3c, 0x83, 0xd1, 0xb3, 0x87}}
	iidIDXGIDevice   = windows.GUID{Data1: 0x54ec77fa, Data2: 0x1377, Data3: 0x44e6, Data4: [8]byte{0x8c, 0x32, 0x88, 0xfd, 0x5f, 0x44, 0xc8, 0x4c}}
)

// comCall calls method `index` of a COM object's vtable.
func comCall(obj unsafe.Pointer, index int, args ...uintptr) uintptr {
	vtbl := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vtbl, index*int(unsafe.Sizeof(uintptr(0)))))
	r, _, _ := syscall.SyscallN(fn, append([]uintptr{uintptr(obj)}, args...)...)
	return r
}

func comRelease(obj unsafe.Pointer) { comCall(obj, 2) }

// Adapter is a DXGI adapter's description and its driver's version.
type Adapter struct {
	Desc   DXGIAdapterDesc1
	Driver [4]uint16
}

// EnumAdapters lists the machine's DXGI adapters.
func EnumAdapters() []Adapter {
	var factory unsafe.Pointer
	r, _, _ := syscall.SyscallN(procCreateDXGIFactory1.Addr(), uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory)))
	if int32(r) < 0 || factory == nil {
		return nil
	}
	defer comRelease(factory)
	var out []Adapter
	for i := range 16 {
		var adapter unsafe.Pointer
		// IDXGIFactory1::EnumAdapters1.
		if int32(comCall(factory, 12, uintptr(i), uintptr(unsafe.Pointer(&adapter)))) < 0 || adapter == nil {
			break
		}
		var a Adapter
		// IDXGIAdapter1::GetDesc1.
		ok := int32(comCall(adapter, 10, uintptr(unsafe.Pointer(&a.Desc)))) >= 0
		var v uint64
		// IDXGIAdapter::CheckInterfaceSupport: the user-mode driver's version.
		if ok && int32(comCall(adapter, 9, uintptr(unsafe.Pointer(&iidIDXGIDevice)), uintptr(unsafe.Pointer(&v)))) >= 0 {
			a.Driver = [4]uint16{uint16(v >> 48), uint16(v >> 32), uint16(v >> 16), uint16(v)}
		}
		comRelease(adapter)
		if ok {
			out = append(out, a)
		}
	}
	return out
}

// ---------------------------------------------------------------- dumps

func MiniDumpWriteDump(process windows.Handle, pid uint32, file windows.Handle, kind uint32) bool {
	r, _, _ := syscall.SyscallN(procMiniDumpWriteDump.Addr(), uintptr(process), uintptr(pid), uintptr(file), uintptr(kind), 0, 0, 0)
	return r != 0
}

// ---------------------------------------------------------------- ETW

// EventTraceProperties is EVENT_TRACE_PROPERTIES (with its WNODE_HEADER).
type EventTraceProperties struct {
	WnodeBufferSize     uint32
	WnodeProviderID     uint32
	WnodeHistorical     uint64
	WnodeTimeStamp      int64
	WnodeGUID           windows.GUID
	WnodeClientContext  uint32
	WnodeFlags          uint32
	BufferSize          uint32
	MinimumBuffers      uint32
	MaximumBuffers      uint32
	MaximumFileSize     uint32
	LogFileMode         uint32
	FlushTimer          uint32
	EnableFlags         uint32
	AgeLimit            int32
	NumberOfBuffers     uint32
	FreeBuffers         uint32
	EventsLost          uint32
	BuffersWritten      uint32
	LogBuffersLost      uint32
	RealTimeBuffersLost uint32
	LoggerThreadID      windows.Handle
	LogFileNameOffset   uint32
	LoggerNameOffset    uint32
}

// EventHeader is EVENT_HEADER.
type EventHeader struct {
	Size          uint16
	HeaderType    uint16
	Flags         uint16
	EventProperty uint16
	ThreadID      uint32
	ProcessID     uint32
	TimeStamp     int64
	ProviderID    windows.GUID
	ID            uint16
	Version       uint8
	Channel       uint8
	Level         uint8
	Opcode        uint8
	Task          uint16
	Keyword       uint64
	ProcessorTime uint64
	ActivityID    windows.GUID
}

// EventRecord is EVENT_RECORD.
type EventRecord struct {
	Header            EventHeader
	BufferContext     uint32
	ExtendedDataCount uint16
	UserDataLength    uint16
	ExtendedData      uintptr
	UserData          unsafe.Pointer
	UserContext       uintptr
}

// EventTraceLogfile is EVENT_TRACE_LOGFILEW.
type EventTraceLogfile struct {
	LogFileName         *uint16
	LoggerName          *uint16
	CurrentTime         int64
	BuffersRead         uint32
	ProcessTraceMode    uint32
	CurrentEvent        [88]byte
	LogfileHeader       [280]byte
	BufferCallback      uintptr
	BufferSize          uint32
	Filled              uint32
	EventsLost          uint32
	EventRecordCallback uintptr
	IsKernelTrace       uint32
	Context             uintptr
}

var _ [120]byte = [unsafe.Sizeof(EventTraceProperties{})]byte{}
var _ [80]byte = [unsafe.Sizeof(EventHeader{})]byte{}
var _ [448]byte = [unsafe.Sizeof(EventTraceLogfile{})]byte{}

const InvalidTrace = ^uint64(0)

func StartTrace(handle *uint64, name *uint16, props unsafe.Pointer) uint32 {
	r, _, _ := syscall.SyscallN(procStartTraceW.Addr(), uintptr(unsafe.Pointer(handle)), uintptr(unsafe.Pointer(name)), uintptr(props))
	return uint32(r)
}

func ControlTrace(handle uint64, name *uint16, props unsafe.Pointer, code uint32) uint32 {
	r, _, _ := syscall.SyscallN(procControlTraceW.Addr(), uintptr(handle), uintptr(unsafe.Pointer(name)), uintptr(props), uintptr(code))
	return uint32(r)
}

func EnableTraceEx2(handle uint64, provider *windows.GUID, code uint32, level uint8, keywords uint64) uint32 {
	r, _, _ := syscall.SyscallN(procEnableTraceEx2.Addr(), uintptr(handle), uintptr(unsafe.Pointer(provider)), uintptr(code), uintptr(level), uintptr(keywords), 0, 0, 0)
	return uint32(r)
}

func OpenTrace(log *EventTraceLogfile) uint64 {
	r, _, _ := syscall.SyscallN(procOpenTraceW.Addr(), uintptr(unsafe.Pointer(log)))
	return uint64(r)
}

func ProcessTrace(handle *uint64) uint32 {
	r, _, _ := syscall.SyscallN(procProcessTrace.Addr(), uintptr(unsafe.Pointer(handle)), 1, 0, 0)
	return uint32(r)
}

func CloseTrace(handle uint64) {
	syscall.SyscallN(procCloseTrace.Addr(), uintptr(handle))
}
