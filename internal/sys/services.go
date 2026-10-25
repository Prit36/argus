package sys

import (
	"errors"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// ServiceState is a service's SERVICE_STATUS state.
type ServiceState uint32

const (
	ServiceStopped ServiceState = 1 + iota
	ServiceStartPending
	ServiceStopPending
	ServiceRunning
	ServiceContinuePending
	ServicePausePending
	ServicePaused
)

func (s ServiceState) String() string {
	switch s {
	case ServiceStopped:
		return "Stopped"
	case ServiceStartPending:
		return "Starting"
	case ServiceStopPending:
		return "Stopping"
	case ServiceRunning:
		return "Running"
	case ServiceContinuePending:
		return "Continuing"
	case ServicePausePending:
		return "Pausing"
	case ServicePaused:
		return "Paused"
	}
	return ""
}

// Service is a Windows service.
type Service struct {
	Name    string
	Display string
	PID     uint32
	State   ServiceState
	// Group is the svchost group it runs in ("netsvcs").
	Group string
}

type pidServices struct{ first, count int32 }

// Services is one listing of the services, and which run in which
// process. A listing doesn't change once read: a new one replaces it.
type Services struct {
	List  []Service
	byPID map[uint32]pidServices
}

// CountIn is how many services run in process pid.
func (s *Services) CountIn(pid uint32) int {
	if s == nil {
		return 0
	}
	return int(s.byPID[pid].count)
}

// FirstIn is the first service running in process pid.
func (s *Services) FirstIn(pid uint32) *Service {
	if s == nil {
		return nil
	}
	e, ok := s.byPID[pid]
	if !ok {
		return nil
	}
	return &s.List[e.first]
}

// ServiceGroups remembers each service's svchost group, read once from its
// ImagePath. Used by one listing at a time.
type ServiceGroups map[string]string

// ReadServices lists the services through the Service Control Manager.
func ReadServices(groups ServiceGroups) *Services {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE|windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil
	}
	defer windows.CloseServiceHandle(scm)
	var buf []uint64
	var needed, count, resume uint32
	for {
		var p *byte
		if len(buf) > 0 {
			p = (*byte)(unsafe.Pointer(&buf[0]))
		}
		err := windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, p, uint32(len(buf)*8), &needed, &count, &resume, nil)
		if err == nil {
			break
		}
		if err != windows.ERROR_MORE_DATA || needed == 0 {
			return nil
		}
		buf = make([]uint64, (needed+4096)/8+1)
		resume = 0
	}
	out := &Services{List: make([]Service, 0, count), byPID: make(map[uint32]pidServices, 128)}
	if count == 0 {
		return out
	}
	items := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), count)
	for i := range items {
		it := &items[i]
		name := windows.UTF16PtrToString(it.ServiceName)
		pid := it.ServiceStatusProcess.ProcessId
		out.List = append(out.List, Service{
			Name:    name,
			Display: windows.UTF16PtrToString(it.DisplayName),
			PID:     pid,
			State:   ServiceState(it.ServiceStatusProcess.CurrentState),
			Group:   groups.of(name),
		})
		if pid != 0 {
			e, ok := out.byPID[pid]
			if !ok {
				e.first = int32(len(out.List) - 1)
			}
			e.count++
			out.byPID[pid] = e
		}
	}
	return out
}

// of is the svchost group a service runs in, from its ImagePath.
func (g ServiceGroups) of(name string) string {
	if v, ok := g[name]; ok {
		return v
	}
	group := ""
	path := ""
	if k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\`+name, registry.QUERY_VALUE); err == nil {
		path, _, _ = k.GetStringValue("ImagePath")
		k.Close()
	}
	if i := indexFold(path, " -k "); i >= 0 {
		rest := path[i+4:]
		if j := strings.IndexByte(rest, ' '); j >= 0 {
			rest = rest[:j]
		}
		group = rest
	}
	g[name] = group
	return group
}

func indexFold(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if strings.EqualFold(s[i:i+len(sub)], sub) {
			return i
		}
	}
	return -1
}

// ServiceAction is what ControlService does.
type ServiceAction uint8

const (
	ServiceStart ServiceAction = iota
	ServiceStop
	ServiceRestart
)

func (a ServiceAction) String() string {
	switch a {
	case ServiceStart:
		return "start"
	case ServiceStop:
		return "stop"
	}
	return "restart"
}

// ErrAccessDenied is what Windows says without administrator rights.
var ErrAccessDenied = errors.New("access denied")

// ControlService starts, stops or restarts service name. A restart waits
// for the service to stop: call it off the UI thread.
func ControlService(name string, action ServiceAction) error {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return ErrAccessDenied
	}
	defer windows.CloseServiceHandle(scm)
	h, err := windows.OpenService(scm, windows.StringToUTF16Ptr(name), windows.SERVICE_START|windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		return ErrAccessDenied
	}
	defer windows.CloseServiceHandle(h)
	if action == ServiceStop || action == ServiceRestart {
		var st windows.SERVICE_STATUS
		if err := windows.ControlService(h, windows.SERVICE_CONTROL_STOP, &st); err != nil && action == ServiceStop {
			return err
		}
	}
	if action == ServiceStart || action == ServiceRestart {
		// A restart: give the service a moment to stop first.
		for tries := 0; ; tries++ {
			err := windows.StartService(h, 0, nil)
			if err == nil || err == windows.ERROR_SERVICE_ALREADY_RUNNING {
				return nil
			}
			if action != ServiceRestart || tries > 20 {
				return err
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
}
