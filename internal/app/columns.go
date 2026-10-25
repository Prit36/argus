package app

// The columns the Processes and Details pages can show, as Task Manager
// offers them (right-click a column's title to choose): each one's title,
// width, alignment and the key it sorts by. Which are shown is a bit set,
// kept between runs.

type columnInfo struct {
	title string
	width float32
	right bool
	sort  Sort
}

// ProcCol is a column of the Processes page, in Task Manager's order.
type ProcCol uint8

const (
	pcType ProcCol = iota
	pcStatus
	pcPublisher
	pcPID
	pcProcessName
	pcCommandLine
	pcCPU
	pcMemory
	pcDisk
	pcNetwork
	pcGPU
	pcGPUEngine
	pcIsolation
	procColumnCount
)

var procColumns = [procColumnCount]columnInfo{
	pcType:        {"Type", 148, false, SortType},
	pcStatus:      {"Status", 136, false, SortStatus},
	pcPublisher:   {"Publisher", 168, false, SortPublisher},
	pcPID:         {"PID", 72, true, SortPID},
	pcProcessName: {"Process name", 160, false, SortProcessName},
	pcCommandLine: {"Command line", 260, false, SortCommandLine},
	pcCPU:         {"CPU", 80, true, SortCPU},
	pcMemory:      {"Memory", 104, true, SortMemory},
	pcDisk:        {"Disk", 88, true, SortDisk},
	pcNetwork:     {"Network", 88, true, SortNetwork},
	pcGPU:         {"GPU", 72, true, SortGPU},
	pcGPUEngine:   {"GPU engine", 132, false, SortGPUEngine},
	pcIsolation:   {"Isolation", 112, false, SortIsolation},
}

const procDefault = 1<<pcStatus | 1<<pcCPU | 1<<pcMemory | 1<<pcDisk | 1<<pcNetwork

// DetailCol is a column of the Details page.
type DetailCol uint8

const (
	dcPID DetailCol = iota
	dcStatus
	dcUser
	dcSession
	dcCPU
	dcCPUTime
	dcMemory
	dcWorkingSet
	dcCommit
	dcThreads
	dcHandles
	dcBasePriority
	dcArch
	dcElevated
	dcImagePath
	dcCommandLine
	dcDescription
	dcIsolation
	detailColumnCount
)

var detailColumns = [detailColumnCount]columnInfo{
	dcPID:          {"PID", 68, true, SortPID},
	dcStatus:       {"Status", 108, false, SortStatus},
	dcUser:         {"User name", 112, false, SortUser},
	dcSession:      {"Session ID", 84, true, SortSession},
	dcCPU:          {"CPU", 56, true, SortCPU},
	dcCPUTime:      {"CPU time", 88, true, SortCPUTime},
	dcMemory:       {"Memory (active private working set)", 188, true, SortMemory},
	dcWorkingSet:   {"Working set (memory)", 140, true, SortWorkingSet},
	dcCommit:       {"Commit size", 104, true, SortCommit},
	dcThreads:      {"Threads", 72, true, SortThreads},
	dcHandles:      {"Handles", 76, true, SortHandles},
	dcBasePriority: {"Base priority", 104, false, SortBasePriority},
	dcArch:         {"Architecture", 92, false, SortArch},
	dcElevated:     {"Elevated", 76, false, SortElevated},
	dcImagePath:    {"Image path name", 280, false, SortPath},
	dcCommandLine:  {"Command line", 320, false, SortCommandLine},
	dcDescription:  {"Description", 240, false, SortDescription},
	dcIsolation:    {"Isolation", 112, false, SortIsolation},
}

const detailDefault = 1<<dcPID | 1<<dcStatus | 1<<dcUser | 1<<dcCPU | 1<<dcMemory | 1<<dcArch | 1<<dcDescription

func shown[T ~uint8](mask uint32, c T) bool { return mask&(1<<c) != 0 }
