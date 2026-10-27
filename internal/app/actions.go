package app

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"github.com/egoist/mygo"
	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// What the commands do: end processes, put them in efficiency mode, set
// their priority, run a new task, show a program in Explorer or its
// properties, search for it online. Each says what happened in a toast.

// rowPIDs are the processes a row of the Processes page stands for.
func (a *App) rowPIDs(r *Row) []uint32 {
	list := a.sampler.Procs
	if r.Kind == RowGroup {
		out := make([]uint32, 0, r.Len)
		for _, i := range a.model.Members(r) {
			out = append(out, list[i].PID)
		}
		return out
	}
	return []uint32{list[r.Proc].PID}
}

var criticalNames = [...]string{"csrss.exe", "wininit.exe", "winlogon.exe", "smss.exe", "services.exe", "lsass.exe"}

// critical reports whether ending pid would bring Windows down.
func (a *App) critical(pid uint32) bool {
	if pid <= 4 {
		return true
	}
	if p := a.sampler.ByPID(pid); p != nil {
		for _, n := range criticalNames {
			if strings.EqualFold(p.Info.Name, n) {
				return true
			}
		}
	}
	return false
}

// endProcesses ends each of pids; says how many it could and couldn't.
func (a *App) endProcesses(pids []uint32, title string) {
	done, denied := 0, 0
	for _, pid := range pids {
		if pid == a.sampler.SelfPID || a.critical(pid) {
			denied++
			continue
		}
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid)
		if err != nil {
			denied++
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			done++
		} else {
			denied++
		}
		windows.CloseHandle(h)
	}
	switch {
	case denied == 0:
		a.toast("success", "Ended "+title)
	case done == 0:
		a.toast("error", "Couldn't end "+title+": access denied")
	default:
		a.toast("warning", fmt.Sprintf("Ended %d of %d processes of %s", done, done+denied, title))
	}
	// Sample again to show it gone.
	a.sample()
}

// efficiencyOn reports whether pid is in efficiency mode (EcoQoS).
func efficiencyOn(pid uint32) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	// PROCESS_POWER_THROTTLING_STATE: version, control mask, state mask.
	st := [3]uint32{1, 0, 0}
	if !winapi.GetProcessInformation(h, 4, unsafe.Pointer(&st), 12) {
		return false
	}
	return st[1]&1 != 0 && st[2]&1 != 0
}

// setEfficiency turns efficiency mode on or off for each of pids: idle
// priority and EcoQoS, as Task Manager sets it.
func (a *App) setEfficiency(pids []uint32, on bool) {
	failed := 0
	for _, pid := range pids {
		h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if err != nil {
			failed++
			continue
		}
		st := [3]uint32{1, 1, 0}
		class := uint32(windows.NORMAL_PRIORITY_CLASS)
		if on {
			st[2] = 1
			class = windows.IDLE_PRIORITY_CLASS
		}
		ok1 := winapi.SetProcessInformation(h, 4, unsafe.Pointer(&st), 12)
		ok2 := windows.SetPriorityClass(h, class) == nil
		if !ok1 || !ok2 {
			failed++
		}
		windows.CloseHandle(h)
	}
	if failed > 0 {
		s := "es"
		if failed == 1 {
			s = ""
		}
		a.toast("error", fmt.Sprintf("Couldn't change efficiency mode for %d process%s", failed, s))
	}
	for i := range a.sampler.Procs {
		for _, pid := range pids {
			if a.sampler.Procs[i].PID == pid {
				a.sampler.Procs[i].Info.EfficiencyChecked = false
			}
		}
	}
	a.sample()
}

// Priority is a priority class.
type Priority uint8

const (
	PrioRealtime Priority = iota
	PrioHigh
	PrioAboveNormal
	PrioNormal
	PrioBelowNormal
	PrioLow
)

var priorities = [...]Priority{PrioRealtime, PrioHigh, PrioAboveNormal, PrioNormal, PrioBelowNormal, PrioLow}

func (p Priority) Title() string {
	return [...]string{"Realtime", "High", "Above normal", "Normal", "Below normal", "Low"}[p]
}

func (p Priority) class() uint32 {
	return [...]uint32{0x100, 0x80, 0x8000, 0x20, 0x4000, 0x40}[p]
}

// priorityOf is the class a base priority comes from.
func priorityOf(base int32) Priority {
	switch {
	case base >= 24:
		return PrioRealtime
	case base >= 13:
		return PrioHigh
	case base >= 10:
		return PrioAboveNormal
	case base >= 8:
		return PrioNormal
	case base >= 5:
		return PrioBelowNormal
	}
	return PrioLow
}

func (a *App) setPriority(pid uint32, p Priority) {
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION, false, pid)
	if err != nil {
		a.toast("error", "Couldn't change the priority: access denied")
		return
	}
	defer windows.CloseHandle(h)
	if windows.SetPriorityClass(h, p.class()) != nil {
		a.toast("error", "Couldn't change the priority")
		return
	}
	a.sample()
}

// openLocation shows the program in an Explorer window, selected.
func (a *App) openLocation(path string) {
	if path == "" {
		a.toast("warning", "This process's file can't be found")
		return
	}
	go mygo.Shell.ShowItemInFolder(path)
}

// properties opens the file's Properties window.
func (a *App) properties(path string) {
	if path == "" {
		a.toast("warning", "This process's file can't be found")
		return
	}
	go winapi.ShellExecute("properties", path, "", winapi.SeeMaskInvokeIDList)
}

// searchOnline searches the web for the program, as Task Manager does.
func searchOnline(name, description string) {
	go mygo.Shell.OpenExternal("https://www.bing.com/search?q=" + url.QueryEscape(name+" "+description))
}

// runTask runs line as the Run dialog does: a program, a folder, a
// document or an address, with administrator rights if asked.
func (a *App) runTask(line string, admin bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return
	}
	file, params := t, ""
	if t[0] == '"' {
		if end := strings.IndexByte(t[1:], '"'); end >= 0 {
			file, params = t[1:1+end], strings.TrimSpace(t[2+end:])
		}
	} else if sp := strings.IndexByte(t, ' '); sp >= 0 && !winapi.FileExists(t) {
		// "notepad file.txt", unless the whole line names a file.
		file, params = t[:sp], strings.TrimSpace(t[sp+1:])
	}
	verb := ""
	if admin {
		verb = "runas"
	}
	go func() {
		if !winapi.ShellExecute(verb, file, params, winapi.SeeMaskFlagNoUI) {
			a.post(func() { a.toast("error", "Windows can't find '"+truncate(file, 120)+"'") })
		}
	}()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// open opens a Windows tool (Services, Resource Monitor, Settings).
func open(target string) {
	go winapi.ShellExecute("", target, "", 0)
}

// switchTo brings an app's window forward; with "Minimize on use", the
// task manager then gets out of the way.
func (a *App) switchTo(pids []uint32) {
	switchCallbackPIDs, switchFound = pids, 0
	windows.EnumWindows(switchCallback, nil)
	found := switchFound
	switchCallbackPIDs = nil
	if found == 0 {
		a.toast("warning", "This app has no window to switch to")
		return
	}
	if winapi.IsIconic(found) {
		winapi.ShowWindow(found, winapi.SwRestore)
	}
	winapi.SetForegroundWindow(found)
	if a.set.MinimizeOnUse && a.win != nil {
		a.win.Minimize()
	}
}

// The callback switchTo finds a window with: made once, as Windows
// callbacks can't be freed.
var (
	switchCallbackPIDs []uint32
	switchFound        windows.HWND
	switchCallback     = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		if !windows.IsWindowVisible(h) || winapi.GetWindow(h, winapi.GwOwner) != 0 || winapi.GetWindowTextLength(h) == 0 {
			return 1
		}
		var pid uint32
		windows.GetWindowThreadProcessId(h, &pid)
		for _, p := range switchCallbackPIDs {
			if p == pid {
				switchFound = h
				return 0
			}
		}
		return 1
	})
)

// isExplorer reports whether pid is the shell (Windows Explorer), whose
// "End task" is "Restart".
func (a *App) isExplorer(pid uint32) bool {
	p := a.sampler.ByPID(pid)
	if p == nil || !strings.EqualFold(p.Info.Name, "explorer.exe") {
		return false
	}
	path := p.Info.Path()
	return len(path) >= len(a.windir) && strings.EqualFold(path[:len(a.windir)], a.windir)
}

// restartExplorer ends Windows Explorer and starts it again (Windows
// usually restarts it itself; if it hasn't in two seconds, it's started
// here).
func (a *App) restartExplorer(pids []uint32) {
	for _, pid := range pids {
		if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
			windows.TerminateProcess(h, 1)
			windows.CloseHandle(h)
		}
	}
	time.AfterFunc(2*time.Second, func() {
		a.post(func() {
			a.sample()
			for i := range a.sampler.Procs {
				if strings.EqualFold(a.sampler.Procs[i].Info.Name, "explorer.exe") {
					return
				}
			}
			open("explorer.exe")
		})
	})
	a.toast("success", "Restarting Windows Explorer")
}

// Dump states.
const (
	dumpNone uint32 = iota
	dumpWriting
)

// createDump writes a full memory dump of pid to the temporary folder as
// NAME.DMP, as Task Manager's "Create memory dump file", on a goroutine.
func (a *App) createDump(pid uint32, name string) {
	if !a.dumpState.CompareAndSwap(dumpNone, dumpWriting) {
		a.toast("warning", "A dump file is already being written")
		return
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	where := filepath.Join(os.TempDir(), base+".DMP")
	a.toast("info", "Writing the dump file")
	go func() {
		ok := writeDump(pid, where)
		a.post(func() {
			a.dumpState.Store(dumpNone)
			if ok {
				a.toast("success", "Dump file written: "+where)
			} else {
				a.toast("error", "Couldn't write the dump file: access denied")
			}
		})
	}()
}

func writeDump(pid uint32, where string) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	f, err := windows.CreateFile(windows.StringToUTF16Ptr(where), windows.GENERIC_WRITE, 0, nil, windows.CREATE_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(f)
	// MiniDumpWithFullMemory | MiniDumpWithHandleData | MiniDumpWithThreadInfo.
	return winapi.MiniDumpWriteDump(h, pid, f, 0x2|0x4|0x1000)
}

// affinity reads the processors pid may run on, and the machine's.
func affinity(pid uint32) (process, system uint64, ok bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return 0, 0, false
	}
	defer windows.CloseHandle(h)
	p, s, ok := winapi.GetProcessAffinityMask(h)
	return uint64(p), uint64(s), ok
}

func (a *App) setAffinity(pid uint32, mask uint64) {
	if mask == 0 {
		a.toast("warning", "The process must be allowed at least one processor")
		return
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_INFORMATION|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		a.toast("error", "Couldn't change the affinity: access denied")
		return
	}
	defer windows.CloseHandle(h)
	if !winapi.SetProcessAffinityMask(h, uintptr(mask)) {
		a.toast("error", "Couldn't change the affinity")
	}
}

// userSession disconnects session id, or signs its user out.
func (a *App) userSession(id uint32, signOut bool) {
	var ok bool
	if signOut {
		ok = winapi.WTSLogoffSession(id)
	} else {
		ok = winapi.WTSDisconnectSession(id)
	}
	if !ok {
		verb := "disconnect"
		if signOut {
			verb = "sign out"
		}
		a.toast("error", "Couldn't "+verb+" this user")
	}
	a.sessionsAt = time.Time{}
	a.refreshSessions(0)
}

// browse asks for a program to run, on a goroutine (the dialog waits for
// the user); its path goes into the Run dialog's field.
func (a *App) browse() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:  a.win,
			Title:   "Browse",
			Filters: []mygo.FileFilter{{Name: "Programs", Extensions: []string{"exe", "com", "bat", "cmd", "msc", "lnk"}}, {Name: "All files", Extensions: []string{"*"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		p := paths[0]
		a.post(func() {
			// Quoted when it has spaces, so it runs as one program.
			if strings.ContainsRune(p, ' ') {
				p = `"` + p + `"`
			}
			a.runText = p
		})
	}()
}

// relaunchAsAdmin starts this program again as administrator; true if it started.
func relaunchAsAdmin() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return winapi.ShellExecute("runas", exe, "", 0)
}
