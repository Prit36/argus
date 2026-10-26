package app

import (
	"math"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows/registry"

	"taskmgr/internal/winapi"
)

// Neutral surfaces and interaction states follow File Explorer. Color is
// reserved for usage heat, the Windows accent, status indicators and graphs.

// Font sizes, in DIPs: the WinUI typographic ramp.
const (
	fontSmall    = 12   // secondary labels, column titles, axis labels
	fontCaption  = 13.5 // table cells, figures, key-value lines
	fontBody     = 14   // the nav, commands, fields, group headers
	fontMetric   = 16   // column totals
	fontSubtitle = 20   // page titles, large figures
	fontTitle    = 26   // the Performance page's part name
)

// accent is the user's accent color, in the shades Windows draws it: a
// darker one on light themes, a lighter one on dark (AccentPalette).
type accent struct{ light, dark ui.Color }

var defaultAccent = accent{light: ui.Hex("#0067c0"), dark: ui.Hex("#4cc2ff")}

// systemAccent reads the accent: AccentPalette holds eight RGBA shades,
// lightest first (Light3, Light2, Light1, the color, Dark1, Dark2, Dark3).
// Light themes use Dark1 and dark themes Light2. Without it, DWM's color.
func systemAccent() accent {
	if p := winapi.RegBinary(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Explorer\Accent`, "AccentPalette"); len(p) >= 28 {
		return accent{light: ui.RGB(p[16], p[17], p[18]), dark: ui.RGB(p[4], p[5], p[6])}
	}
	v, ok := winapi.RegDword(registry.CURRENT_USER, `Software\Microsoft\Windows\DWM`, "AccentColor")
	if !ok {
		return defaultAccent
	}
	base := ui.RGB(uint8(v), uint8(v>>8), uint8(v>>16))
	return accent{light: base, dark: base.Mix(ui.RGB(255, 255, 255), 0.45)}
}

// systemDark reports the system's app mode: dark unless apps use the
// light theme.
func systemDark() bool {
	v, ok := winapi.RegDword(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme")
	return ok && v == 0
}

// colors are the palette of a frame.
type colors struct {
	dark bool
	// backdrop is the title bar and the navigation pane; page the page, a
	// layer above it.
	backdrop, page, pageBorder     ui.Color
	card, cardBorder               ui.Color
	field, fieldBorder             ui.Color
	text, secondary, tertiary      ui.Color
	accent, onAccent               ui.Color
	hover, pressed                 ui.Color
	navSelected                    ui.Color
	rowHover, rowSelected          ui.Color
	hoverOverlay, selectionOverlay ui.Color
	selectionBorder                ui.Color
	divider, cellDivider           ui.Color
	// Resource intensity follows the reference's blue heat map.
	heatLow, heatHigh, heatText     ui.Color
	closeHover, danger, success     ui.Color
	cpu, memory, disk, network, gpu ui.Color
	graphBG                         ui.Color
}

var (
	white = ui.RGB(255, 255, 255)
	black = ui.RGB(0, 0, 0)
)

func palette(dark bool, ac accent) colors {
	if dark {
		page := ui.Hex("#191919")
		k := colors{
			dark:             true,
			backdrop:         ui.Hex("#202020"),
			page:             page,
			pageBorder:       ui.Hex("#383838"),
			card:             ui.Hex("#222222"),
			cardBorder:       ui.Hex("#383838"),
			field:            ui.Hex("#383838"),
			fieldBorder:      ui.Hex("#606060"),
			text:             ui.Hex("#f2f2f2"),
			secondary:        ui.Hex("#cccccc"),
			tertiary:         ui.Hex("#aaaaaa"),
			accent:           ac.dark,
			onAccent:         black,
			hover:            white.Alpha(0.08),
			pressed:          white.Alpha(0.12),
			navSelected:      ui.Hex("#333333"),
			rowHover:         white.Alpha(0.06).Over(page),
			rowSelected:      white.Alpha(0.12).Over(page),
			hoverOverlay:     white.Alpha(0.06),
			selectionOverlay: white.Alpha(0.12),
			selectionBorder:  ui.Hex("#686868"),
			divider:          ui.Hex("#383838"),
			cellDivider:      ui.Transparent,
			heatLow:          ui.Hex("#11243e"),
			heatHigh:         ui.Hex("#0a3d83"),
			heatText:         ui.Hex("#f2f2f2"),
			closeHover:       ui.Hex("#c42b1c"),
			danger:           ui.Hex("#ff99a4"),
			success:          ui.Hex("#6ccb5f"),
			cpu:              ui.Hex("#39b8e3"),
			memory:           ui.Hex("#c58af9"),
			disk:             ui.Hex("#6ccb5f"),
			network:          ui.Hex("#f2a950"),
			gpu:              ui.Hex("#4cc2ff"),
			graphBG:          page,
		}
		return k
	}
	page := ui.Hex("#ffffff")
	return colors{
		backdrop:         ui.Hex("#f3f3f3"),
		page:             page,
		pageBorder:       ui.Hex("#d5d5d5"),
		card:             ui.Hex("#fafafa"),
		cardBorder:       ui.Hex("#d9d9d9"),
		field:            ui.Hex("#ffffff"),
		fieldBorder:      ui.Hex("#b5b5b5"),
		text:             ui.Hex("#1b1b1b"),
		secondary:        ui.Hex("#525252"),
		tertiary:         ui.Hex("#6b6b6b"),
		accent:           ac.light,
		onAccent:         white,
		hover:            black.Alpha(0.045),
		pressed:          black.Alpha(0.03),
		navSelected:      ui.Hex("#e5e5e5"),
		rowHover:         black.Alpha(0.04).Over(page),
		rowSelected:      black.Alpha(0.08).Over(page),
		hoverOverlay:     black.Alpha(0.04),
		selectionOverlay: black.Alpha(0.08),
		selectionBorder:  ui.Hex("#b3b3b3"),
		divider:          ui.Hex("#dedede"),
		cellDivider:      ui.Transparent,
		heatLow:          page.Mix(ac.light, 0.07),
		heatHigh:         page.Mix(ac.light, 0.42),
		heatText:         ui.Hex("#1b1b1b"),
		closeHover:       ui.Hex("#c42b1c"),
		danger:           ui.Hex("#c42b1c"),
		success:          ui.Hex("#107c10"),
		cpu:              ui.Hex("#0067c0"),
		memory:           ui.Hex("#881798"),
		disk:             ui.Hex("#107c10"),
		network:          ui.Hex("#9a5006"),
		gpu:              ui.Hex("#0067c0"),
		graphBG:          ui.Hex("#ffffff"),
	}
}

// uiTheme is mygo's theme for its widgets (dialogs, switches, radio
// buttons, selects, tooltips, toasts, scroll bars) in this palette.
func uiTheme(k *colors) *ui.Theme {
	var t ui.Theme
	if k.dark {
		t = *ui.DarkTheme()
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#303030"), ui.Hex("#3b3b3b"), ui.Hex("#353535")
		t.Border = ui.Hex("#606060")
	} else {
		t = *ui.LightTheme()
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#ffffff"), ui.Hex("#f2f2f2"), ui.Hex("#ececec")
		t.Border = ui.Hex("#e0e0e0")
	}
	t.Background = k.page
	t.Text, t.TextMuted = k.text, k.secondary
	t.Accent = k.accent
	if k.dark {
		t.AccentHover, t.AccentPressed = k.accent.Mix(white, 0.1), k.accent.Mix(white, 0.18)
	} else {
		t.AccentHover, t.AccentPressed = k.accent.Mix(black, 0.1), k.accent.Mix(black, 0.18)
	}
	t.AccentText = k.onAccent
	t.Danger = k.danger
	t.Success = k.accent
	t.Font = "Segoe UI"
	t.FontSize = fontBody
	t.Radius = 4
	return &t
}

// heat is a busy cell's background: a faint accent tint when idle, deeper
// as share (0–1 of what counts as busy) grows.
func (k *colors) heat(share float32) ui.Color {
	t := min(1, max(0, share))
	return k.heatLow.Mix(k.heatHigh, float32(math.Sqrt(float64(t))))
}
