package sys

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"taskmgr/internal/winapi"
)

// Session is a signed-in user's session.
type Session struct {
	ID     uint32
	User   string
	Domain string
	// Full is the account's full name ("Ada Lovelace"), if it has one.
	Full   string
	Active bool
}

// fullNames remembers accounts' full names: NetUserGetInfo can take a
// while, and names don't change while the program runs.
var fullNames = map[string]string{}

// ReadSessions lists the sessions with a user signed in. Called off the UI
// thread, one call at a time.
func ReadSessions() []Session {
	var info *windows.WTS_SESSION_INFO
	var count uint32
	if windows.WTSEnumerateSessions(0, 0, 1, &info, &count) != nil || info == nil {
		return nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(info)))
	var out []Session
	for _, s := range unsafe.Slice(info, count) {
		user := winapi.WTSQueryString(s.SessionID, winapi.WTSUserName)
		if user == "" {
			continue
		}
		full, ok := fullNames[user]
		if !ok {
			full = winapi.NetUserFullName(user)
			fullNames[user] = full
		}
		out = append(out, Session{
			ID:     s.SessionID,
			User:   user,
			Domain: winapi.WTSQueryString(s.SessionID, winapi.WTSDomainName),
			Full:   full,
			// WTSActive.
			Active: s.State == 0,
		})
	}
	return out
}
