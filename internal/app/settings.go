package app

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Settings are kept between runs in %LOCALAPPDATA%\argus\settings.ini
// as key=value lines: the theme, the update speed, the page to start on,
// and how the window and its tables were left.
type Settings struct {
	Theme           ThemeMode
	Speed           Speed
	StartPage       Page
	AlwaysOnTop     bool
	GroupByType     bool
	CPULogical      bool
	NavOpen         bool
	Width, Height   uint32
	ProcColumns     uint32
	DetailColumns   uint32
	MinimizeOnUse   bool
	HideMinimized   bool
	AskEfficiency   bool
	FullAccountName bool
	HistoryAll      bool
	KernelTimes     bool

	ProcSort, DetailsSort                                 Sort
	ProcAscending, DetailsAscending                       bool
	ServicesSort, StartupSort, HistorySort                uint8
	ServicesAscending, StartupAscending, HistoryAscending bool
}

func defaultSettings() Settings {
	return Settings{Speed: SpeedNormal, GroupByType: true, NavOpen: true, ProcColumns: procDefault, DetailColumns: detailDefault, AskEfficiency: true,
		ProcAscending: true, DetailsAscending: true, ServicesAscending: true, StartupAscending: true, HistoryAscending: true}
}

// dataDir is the folder settings and App history are kept in.
func dataDir() string {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "argus")
}

// fields lists the settings by their keys in the file.
func (s *Settings) fields() []struct {
	key string
	u8  *uint8
	u32 *uint32
	b   *bool
} {
	type f = struct {
		key string
		u8  *uint8
		u32 *uint32
		b   *bool
	}
	return []f{
		{key: "theme", u8: (*uint8)(&s.Theme)},
		{key: "speed", u8: (*uint8)(&s.Speed)},
		{key: "start_page", u8: (*uint8)(&s.StartPage)},
		{key: "always_on_top", b: &s.AlwaysOnTop},
		{key: "group_by_type", b: &s.GroupByType},
		{key: "cpu_logical", b: &s.CPULogical},
		{key: "nav_open", b: &s.NavOpen},
		{key: "width", u32: &s.Width},
		{key: "height", u32: &s.Height},
		{key: "proc_columns", u32: &s.ProcColumns},
		{key: "detail_columns", u32: &s.DetailColumns},
		{key: "minimize_on_use", b: &s.MinimizeOnUse},
		{key: "hide_when_minimized", b: &s.HideMinimized},
		{key: "ask_efficiency", b: &s.AskEfficiency},
		{key: "full_account_name", b: &s.FullAccountName},
		{key: "history_all", b: &s.HistoryAll},
		{key: "kernel_times", b: &s.KernelTimes},
		{key: "proc_sort", u8: (*uint8)(&s.ProcSort)},
		{key: "proc_ascending", b: &s.ProcAscending},
		{key: "details_sort", u8: (*uint8)(&s.DetailsSort)},
		{key: "details_ascending", b: &s.DetailsAscending},
		{key: "services_sort", u8: &s.ServicesSort},
		{key: "services_ascending", b: &s.ServicesAscending},
		{key: "startup_sort", u8: &s.StartupSort},
		{key: "startup_ascending", b: &s.StartupAscending},
		{key: "history_sort", u8: &s.HistorySort},
		{key: "history_ascending", b: &s.HistoryAscending},
	}
}

func loadSettings() Settings {
	s := defaultSettings()
	data, err := os.ReadFile(filepath.Join(dataDir(), "settings.ini"))
	if err != nil {
		return s
	}
	fields := s.fields()
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		n, err := strconv.ParseUint(strings.TrimSpace(v), 10, 32)
		if err != nil {
			continue
		}
		k = strings.TrimSpace(k)
		for _, f := range fields {
			if f.key != k {
				continue
			}
			switch {
			case f.u8 != nil:
				*f.u8 = uint8(min(n, 255))
			case f.u32 != nil:
				*f.u32 = uint32(n)
			case f.b != nil:
				*f.b = n != 0
			}
		}
	}
	if s.ProcSort > SortIsolation {
		s.ProcSort = SortName
	}
	if s.DetailsSort > SortIsolation {
		s.DetailsSort = SortName
	}
	if s.ServicesSort >= uint8(len(serviceCols)) {
		s.ServicesSort = 0
	}
	if s.StartupSort >= uint8(len(startupCols)) {
		s.StartupSort = 0
	}
	if s.HistorySort >= uint8(len(historyCols)) {
		s.HistorySort = 0
	}
	return s
}

func (s *Settings) save() {
	var b bytes.Buffer
	for _, f := range s.fields() {
		var v uint64
		switch {
		case f.u8 != nil:
			v = uint64(*f.u8)
		case f.u32 != nil:
			v = uint64(*f.u32)
		case f.b != nil && *f.b:
			v = 1
		}
		b.WriteString(f.key)
		b.WriteByte('=')
		b.WriteString(strconv.FormatUint(v, 10))
		b.WriteString("\r\n")
	}
	dir := dataDir()
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "settings.ini"), b.Bytes(), 0o644)
}
