package app

import (
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"taskmgr/internal/sys"
)

// testApp is the task manager with this machine's figures, drawn without a
// window. Its settings and history live in a directory of the test's.
func testApp(t *testing.T) (*App, *ui.Tester) {
	t.Helper()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	a := newApp()
	a.darkSystem, a.accent = true, systemAccent()
	a.sample()
	a.sampler.WaitResolved(time.Second)
	time.Sleep(200 * time.Millisecond)
	a.sample()
	a.services = sys.ReadServices(a.serviceGroups)
	a.dirty = true
	t.Cleanup(func() {
		a.closed = true
		if a.perf != nil {
			a.perf.Close()
		}
		a.disks.Close()
	})
	return a, ui.NewTester(a.view, 1080, 720)
}

// snap keeps the frame as name.png in TASKMAN_SNAPSHOTS, when it is set.
func snap(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("TASKMAN_SNAPSHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func click(t *testing.T, tt *ui.Tester, s string) {
	t.Helper()
	if err := tt.Click(s); err != nil {
		t.Fatalf("click %q: %v", s, err)
	}
}

func TestPages(t *testing.T) {
	a, tt := testApp(t)
	for _, p := range []struct {
		nav  string
		page Page
		want string
	}{
		{"Performance", PagePerformance, "Utilization"},
		{"App history", PageAppHistory, "Delete usage history"},
		{"Startup apps", PageStartup, "Startup impact"},
		{"Users", PageUsers, "Manage user accounts"},
		{"Details", PageDetails, "User name"},
		{"Services", PageServices, "Description"},
		{"Settings", PageSettings, "Real time update speed"},
		{"Processes", PageProcesses, "Background processes"},
	} {
		click(t, tt, p.nav)
		tt.Advance(time.Second)
		if a.page != p.page || !tt.HasText(p.want) {
			t.Errorf("%s: page %d, no %q among %q", p.nav, a.page, p.want, tt.Texts())
		}
		snap(t, tt, "page-"+p.nav)
	}
	// The last category can be below the virtualized viewport on machines
	// with many background processes; verify it exists in the model.
	if !slices.ContainsFunc(a.model.Rows, func(r Row) bool {
		return r.Kind == RowHeader && r.Category == CatWindows
	}) {
		t.Error("no Windows processes group")
	}
	tt.SetDark(false)
	a.darkSystem = false
	tt.Frame()
	snap(t, tt, "light-processes")
}

func TestPerformanceTabs(t *testing.T) {
	a, tt := testApp(t)
	click(t, tt, "Performance")
	for _, tab := range []string{"Memory", "Wi-Fi", "GPU 0", "CPU"} {
		if err := tt.Click(tab); err != nil {
			t.Logf("no %s here: %v", tab, err)
			continue
		}
		tt.Advance(time.Second)
		snap(t, tt, "perf-"+tab)
	}
	if a.perfTab.Kind != PerfCPU {
		t.Errorf("tab %v after CPU", a.perfTab.Kind)
	}
}

func TestMenusAndDialogs(t *testing.T) {
	a, tt := testApp(t)
	// The search narrows the rows to the System process.
	click(t, tt, "Search")
	tt.Type("System")
	tt.Advance(time.Second)
	if !tt.HasText("System") || tt.HasText("Background processes") {
		t.Fatalf("search for System shows %q", tt.Texts())
	}
	snap(t, tt, "search")
	if err := tt.RightClick("System"); err != nil {
		t.Fatal(err)
	}
	menu := tt.Menu()
	for _, want := range []string{"End task", "Open file location", "Go to details", "Properties"} {
		if !slices.Contains(menu, want) {
			t.Errorf("process menu %q has no %q", menu, want)
		}
	}

	// Go to details selects the process on the Details page.
	if err := tt.RightClick("System"); err != nil {
		t.Fatal(err)
	}
	if err := tt.ChooseMenuItem("Go to details"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if a.page != PageDetails {
		t.Errorf("Go to details went to page %d", a.page)
	}
	snap(t, tt, "details-selected")

	// Run new task opens its dialog; Escape closes it.
	click(t, tt, "Run new task")
	tt.Advance(time.Second)
	if !tt.HasText("Create new task") {
		t.Fatalf("no run dialog in %q", tt.Texts())
	}
	snap(t, tt, "run-dialog")
	tt.Key(0, ui.KeyEscape)
	tt.Advance(time.Second)
	if tt.HasText("Create new task") {
		t.Error("Escape left the run dialog open")
	}

	// The view menu of the Processes page.
	click(t, tt, "Processes")
	click(t, tt, "View")
	if m := tt.Menu(); !slices.Contains(m, "Group by type") {
		t.Errorf("view menu %q", m)
	}
	tt.Key(0, ui.KeyEscape)
}
