package sys

import (
	"fmt"
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

// ServiceReader reuses the enumeration buffer and unchanged strings.
// Read is called by one worker at a time; published Services stay immutable.
type ServiceReader struct {
	buf      []uint64
	previous *Services
}

// ReadServices lists the services through the Service Control Manager.
func ReadServices(groups ServiceGroups) *Services {
	return new(ServiceReader).Read(groups)
}

func (r *ServiceReader) Read(groups ServiceGroups) *Services {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE|windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil
	}
	defer windows.CloseServiceHandle(scm)
	var needed, count, resume uint32
	for {
		var p *byte
		if len(r.buf) > 0 {
			p = (*byte)(unsafe.Pointer(&r.buf[0]))
		}
		err := windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO, windows.SERVICE_WIN32, windows.SERVICE_STATE_ALL, p, uint32(len(r.buf)*8), &needed, &count, &resume, nil)
		if err == nil {
			break
		}
		if err != windows.ERROR_MORE_DATA || needed == 0 {
			return nil
		}
		r.buf = make([]uint64, (needed+4096)/8+1)
		resume = 0
	}
	if count == 0 {
		if r.previous != nil && len(r.previous.List) == 0 {
			return r.previous
		}
		out := &Services{}
		r.previous = out
		return out
	}
	items := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&r.buf[0])), count)
	return r.snapshot(items, groups)
}

func utf16PointerSlice(p *uint16) []uint16 {
	if p == nil {
		return nil
	}
	n := 0
	for *(*uint16)(unsafe.Add(unsafe.Pointer(p), n*2)) != 0 {
		n++
	}
	return unsafe.Slice(p, n)
}

func (r *ServiceReader) snapshot(items []windows.ENUM_SERVICE_STATUS_PROCESS, groups ServiceGroups) *Services {
	previous := r.previous
	unchanged := previous != nil && len(previous.List) == len(items)
	if unchanged {
		for i := range items {
			it, old := &items[i], &previous.List[i]
			if it.ServiceStatusProcess.ProcessId != old.PID || ServiceState(it.ServiceStatusProcess.CurrentState) != old.State ||
				!utf16Equal(utf16PointerSlice(it.ServiceName), old.Name) || !utf16Equal(utf16PointerSlice(it.DisplayName), old.Display) {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return previous
	}
	out := &Services{List: make([]Service, 0, len(items)), byPID: make(map[uint32]pidServices, 128)}
	for i := range items {
		it := &items[i]
		var old Service
		if previous != nil && i < len(previous.List) {
			old = previous.List[i]
		}
		name := utf16Cached(old.Name, utf16PointerSlice(it.ServiceName))
		pid := it.ServiceStatusProcess.ProcessId
		out.List = append(out.List, Service{
			Name:    name,
			Display: utf16Cached(old.Display, utf16PointerSlice(it.DisplayName)),
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
	r.previous = out
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

// ControlService starts, stops or restarts service name. A restart waits
// for the service to stop: call it off the UI thread.
func ControlService(name string, action ServiceAction) error {
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(scm)
	access := uint32(windows.SERVICE_QUERY_STATUS)
	if action == ServiceStart || action == ServiceRestart {
		access |= windows.SERVICE_START
	}
	if action == ServiceStop || action == ServiceRestart {
		access |= windows.SERVICE_STOP
	}
	h, err := windows.OpenService(scm, windows.StringToUTF16Ptr(name), access)
	if err != nil {
		return err
	}
	defer windows.CloseServiceHandle(h)
	if action == ServiceStop || action == ServiceRestart {
		var st windows.SERVICE_STATUS
		if err := windows.ControlService(h, windows.SERVICE_CONTROL_STOP, &st); err != nil {
			if action != ServiceRestart || err != windows.ERROR_SERVICE_NOT_ACTIVE {
				return err
			}
		}
		if action == ServiceRestart {
			for tries := 0; ; tries++ {
				if err := windows.QueryServiceStatus(h, &st); err != nil {
					return err
				}
				if st.CurrentState == windows.SERVICE_STOPPED {
					break
				}
				if tries >= 100 {
					return fmt.Errorf("timed out waiting for the service to stop")
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}
	if action == ServiceStart || action == ServiceRestart {
		return windows.StartService(h, 0, nil)
	}
	return nil
}
