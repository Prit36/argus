package app

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

func fixtureApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	list := []sys.Proc{
		{PID: 10, CPU: 1, PrivateWS: 10 << 20, Windowed: true, Session: 1},
		{PID: 20, CPU: 9, PrivateWS: 90 << 20, Session: 1},
		{PID: 30, CPU: 4, PrivateWS: 40 << 20, Session: 2},
		{PID: 11, PPID: 10, CPU: 2, PrivateWS: 20 << 20, Session: 1},
	}
	for i, name := range []string{"Alpha", "Beta", "Gamma", "Worker"} {
		path := `C:\Tools\` + name + ".exe"
		if i == 2 {
			path = `C:\Windows\System32\Gamma.exe`
		}
		list[i].Info = &sys.Info{PID: list[i].PID, Name: name + ".exe", CreateTime: int64(i + 1),
			Exe: &sys.Exe{Path: path, File: strings.ToLower(name) + ".exe", Description: name, Company: "Example"}}
		list[i].ReadRate, list[i].Net, list[i].GPU = list[i].CPU, list[i].CPU, list[i].CPU
	}
	a := &App{set: defaultSettings(), sampler: &sys.Sampler{Procs: list}, model: newModel(),
		windir: `c:\windows\`, dirty: true, ascending: true, detailsAsc: true,
		servicesAsc: true, startupAsc: true, historyAsc: true, startupSel: -1,
		disks: &sys.Disks{}, net: &sys.Net{}, history: &sys.History{},
		usersOpen: map[uint32]bool{}, resumeSpeed: SpeedLow, active: true, scale: 1,
		startupRead: true, sessionsBusy: true, servicesBusy: true,
		startup:  []sys.StartupItem{{Name: "Zulu", Publisher: "Example", Enabled: true}, {Name: "Alpha", Publisher: "Other"}},
		sessions: []sys.Session{{ID: 1, User: "Ada"}, {ID: 2, User: "Grace"}},
	}
	a.icons.byExe = map[*sys.Exe]*exeIcon{}
	a.set.Speed = SpeedPaused
	a.history.List = []sys.HistoryEntry{{Key: "zulu", Title: "Zulu", App: true, CPU: 20}, {Key: "alpha", Title: "Alpha", App: true, CPU: 10}}
	t.Cleanup(func() { a.setSpeed(SpeedPaused); a.closed = true })
	return a
}

func TestColumnSortAcrossCategories(t *testing.T) {
	for _, key := range []Sort{SortCPU, SortMemory, SortDisk, SortNetwork, SortGPU, SortName} {
		t.Run(itoa(key), func(t *testing.T) {
			a := fixtureApp(t)
			a.refreshRows()
			if !slices.ContainsFunc(a.model.Rows, func(r Row) bool { return r.Kind == RowHeader }) {
				t.Fatal("fixture lacks category headers")
			}
			selected := a.model.Rows[1].Key
			a.selected = selected
			for range 2 {
				a.sortBy(key)
				a.refreshRows()
				if a.set.GroupByType {
					t.Fatal("column sort kept category grouping")
				}
				for i, r := range a.model.Rows {
					if r.Kind == RowHeader {
						t.Fatal("column sort retained a category header")
					}
					if i > 0 {
						prev := a.model.groups[i-1]
						curr := a.model.groups[i]
						o := buildOptions{sort: key, ascending: a.ascending}
						if a.model.compare(o, a.model.groupFigures(a.sampler.Procs, o, &prev), a.model.groupFigures(a.sampler.Procs, o, &curr)) > 0 {
							t.Fatal("rows are not globally ordered")
						}
					}
				}
				if a.selectedRow() == nil || a.selected != selected {
					t.Fatal("sort lost selection")
				}
			}
			a.set.GroupByType, a.dirty = true, true
			a.refreshRows()
			if !slices.ContainsFunc(a.model.Rows, func(r Row) bool { return r.Kind == RowHeader }) {
				t.Fatal("explicit grouping did not return")
			}
		})
	}
}

func TestCollapsedGroupsExpandInSortedOrder(t *testing.T) {
	a := fixtureApp(t)
	a.sortBy(SortCPU)
	a.refreshRows()
	r := slices.IndexFunc(a.model.Rows, func(r Row) bool { return r.Kind == RowGroup })
	if r < 0 {
		t.Fatal("no group")
	}
	key := a.model.Rows[r].Key
	a.model.Toggle(key)
	a.dirty = true
	a.refreshRows()
	r = slices.IndexFunc(a.model.Rows, func(r Row) bool { return r.Key == key })
	if !a.model.Rows[r].Expanded || a.model.Rows[r+1].CPU != 2 || a.model.Rows[r+2].CPU != 1 {
		t.Fatal("expanding a formerly unsorted group did not sort its children")
	}
}

func TestSearchStaysOnEachDataPage(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.view, 1080, 720)
	for _, tc := range []struct {
		page  Page
		query string
		count func() int
	}{
		{PageStartup, "Other", func() int { return len(a.startupOrder) }},
		{PageAppHistory, "Zulu", func() int { return len(a.historyRows) }},
		{PageUsers, "Grace", func() int { return len(a.userRows) }},
	} {
		a.goTo(tc.page)
		tt.Frame()
		click(t, tt, "Search")
		tt.Type(tc.query)
		tt.Frame()
		if a.page != tc.page || tc.count() != 1 {
			t.Fatalf("search %q: page=%v count=%v", tc.query, a.page, tc.count())
		}
	}
	a.goTo(PageStartup)
	if a.query != "Other" {
		t.Fatalf("startup filter lost: %q", a.query)
	}
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if a.query != "" || len(a.startupOrder) != 2 {
		t.Fatal("Escape did not restore all startup rows")
	}
}

func TestPauseControlAndModalShortcuts(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.view, 1080, 720)
	click(t, tt, "Resume updates")
	if a.set.Speed != SpeedLow {
		t.Fatal("resume did not restore previous speed")
	}
	tt.Key(ui.Ctrl, ui.KeyP)
	tt.Frame()
	if a.set.Speed != SpeedPaused || !tt.HasText("Resume updates") {
		t.Fatal("pause shortcut failed")
	}
	a.runOpen = true
	tt.Frame()
	tt.Key(ui.Ctrl, ui.Key2)
	tt.Frame()
	if a.page != PageProcesses {
		t.Fatal("page shortcut escaped the modal")
	}
	tt.Key(ui.Ctrl, ui.KeyP)
	tt.Frame()
	if a.set.Speed != SpeedPaused {
		t.Fatal("pause shortcut escaped the modal")
	}
}

func TestSortSettingsRoundTrip(t *testing.T) {
	a := fixtureApp(t)
	a.sortBy(SortMemory)
	a.detailsSortBy(SortCPUTime)
	a.servicesSortBy(3)
	a.startupSort, a.startupAsc = 2, false
	a.historySort, a.historyAsc = 1, false
	a.saveSettings()
	s := loadSettings()
	if s.GroupByType || s.ProcSort != SortMemory || s.ProcAscending || s.DetailsSort != SortCPUTime || s.DetailsAscending ||
		s.ServicesSort != 3 || s.StartupSort != 2 || s.StartupAscending || s.HistorySort != 1 || s.HistoryAscending {
		t.Fatalf("sort preferences did not round-trip: %+v", s)
	}
	if err := os.WriteFile(filepath.Join(dataDir(), "settings.ini"), []byte("proc_sort=255\ndetails_sort=255\nservices_sort=255\nstartup_sort=255\nhistory_sort=255\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s = loadSettings()
	if s.ProcSort != SortName || s.DetailsSort != SortName || s.ServicesSort != 0 || s.StartupSort != 0 || s.HistorySort != 0 {
		t.Fatal("invalid sorts were not reset")
	}
}

func TestUsersSearchAndAggregates(t *testing.T) {
	a := fixtureApp(t)
	a.page, a.query = PageUsers, "Worker"
	a.refreshRows()
	if len(a.userRows) != 1 || a.sessions[a.userRows[0]].ID != 1 {
		t.Fatal("process search did not find its user")
	}
	if sum := a.userSums[1]; sum.cpu != 12 || sum.count != 3 || sum.memory != 120<<20 {
		t.Fatalf("incorrect aggregate: %+v", sum)
	}
	a.query = "no such account"
	a.refreshRows()
	if len(a.userRows) != 0 {
		t.Fatal("no-results search retained users")
	}
}

func TestCopySelectionAndGoToDetails(t *testing.T) {
	a := fixtureApp(t)
	a.refreshRows()
	a.selected = a.model.Rows[1].Key
	if text := a.selectionText(); !strings.Contains(text, "Alpha") || !strings.Contains(text, "Unavailable") {
		t.Fatalf("copy text: %q", text)
	}
	a.queries[PageDetails] = "old filter"
	a.goToDetails(20)
	a.refreshRows()
	if a.query != "" || a.detailsSelected() == nil || a.detailsSelected().PID != 20 {
		t.Fatal("Go to details failed to reveal process through stale filter")
	}
	if !strings.Contains(a.selectionText(), "Beta.exe\t20") {
		t.Fatal("details copy omitted PID")
	}
}

func TestUnchangedRowsAreReused(t *testing.T) {
	a := fixtureApp(t)
	for _, page := range []Page{PageProcesses, PageDetails, PageStartup, PageAppHistory, PageUsers} {
		a.page, a.dirty = page, true
		a.refreshRows()
		if allocs := testing.AllocsPerRun(100, a.refreshRows); allocs != 0 {
			t.Errorf("page %v: idle row refresh allocates %v", page, allocs)
		}
	}
}

func TestF5WhilePaused(t *testing.T) {
	a, tt := testApp(t)
	a.setSpeed(SpeedPaused)
	before := a.samples
	tt.Key(0, ui.KeyF5)
	tt.Advance(time.Millisecond)
	if a.samples != before+1 || a.set.Speed != SpeedPaused {
		t.Fatal("F5 must refresh once without resuming")
	}
}

func TestColumnClicksUpdateImmediately(t *testing.T) {
	a := fixtureApp(t)
	tt := ui.NewTester(a.view, 1080, 720)
	click(t, tt, "CPU")
	if a.set.GroupByType || a.model.Rows[0].Title != "Beta" || a.ascending {
		t.Fatal("CPU header did not immediately rank all categories busiest first")
	}
	a.goTo(PageStartup)
	tt.Frame()
	click(t, tt, "Name")
	if a.startup[a.startupOrder[0]].Name != "Zulu" {
		t.Fatal("startup sort required another sample")
	}
	a.goTo(PageAppHistory)
	tt.Frame()
	click(t, tt, "CPU time")
	if a.history.List[a.historyRows[0]].Title != "Zulu" {
		t.Fatal("history sort required another sample")
	}
	a.query = "nothing matches"
	tt.Frame()
	if len(a.historyRows) != 0 || !tt.HasText(noResults(a.query)) {
		t.Fatal("history empty state was not immediate")
	}
}
