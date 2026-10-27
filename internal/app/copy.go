package app

import (
	"strings"

	"github.com/egoist/mygo"

	"taskmgr/internal/sys"
)

// selectionText is tab-separated so a copied row also pastes into a sheet.
func (a *App) selectionText() string {
	var p *sys.Proc
	switch a.page {
	case PageProcesses:
		if r := a.selectedRow(); r != nil {
			p = &a.sampler.Procs[r.Proc]
			return strings.Join([]string{"Name", "CPU", "Memory", "Disk", "Network", "Path"}, "\t") + "\r\n" +
				strings.Join([]string{r.Title, cpuCell(r.CPU), mbCell(r.Memory), diskCell(r.Disk), a.copyNetwork(r.Net, r.NetUnknown), p.Info.Path()}, "\t")
		}
	case PageDetails:
		p = a.detailsSelected()
	case PageUsers:
		p = a.userProc()
	case PageServices:
		if s := a.serviceSelected(); s != nil {
			return "Name\tPID\tDescription\tStatus\tGroup\r\n" +
				strings.Join([]string{s.Name, itoa(s.PID), s.Display, s.State.String(), s.Group}, "\t")
		}
	case PageStartup:
		if s := a.startupItem(); s != nil {
			status := "Disabled"
			if s.Enabled {
				status = "Enabled"
			}
			return "Name\tPublisher\tStatus\tPath\r\n" + strings.Join([]string{s.Name, s.Publisher, status, s.Path}, "\t")
		}
	}
	if p == nil {
		return ""
	}
	return "Name\tPID\tUser\tCPU\tMemory\tPath\r\n" +
		strings.Join([]string{p.Info.Name, itoa(p.PID), p.Info.User, cpuCell(p.CPU), mbCell(p.PrivateWS), p.Info.Path()}, "\t")
}

func (a *App) copyNetwork(rate float32, unknown bool) string {
	if !a.netmon.Available || unknown {
		return "Unavailable"
	}
	return netCell(rate)
}

func (a *App) copySelection() {
	if text := a.selectionText(); text != "" {
		go mygo.Clipboard.WriteText(text)
		a.toast("success", "Copied")
	}
}
