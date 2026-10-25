package app

import (
	"math"

	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows/registry"

	"taskmgr/internal/winapi"
)

// Windows 11's Task Manager colors, light and dark: the window's backdrop
// (Mica's gray), the page on it, text in three strengths, the accent the
// user chose, the accent-tinted heat of busy cells, and each graph's color
// (CPU blue, memory purple, disk green, network brown, GPU blue).

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
	backdrop, page, pageBorder ui.Color
	card, cardBorder           ui.Color
	field, fieldBorder         ui.Color
	text, secondary, tertiary  ui.Color
	accent, onAccent           ui.Color
	hover, pressed             ui.Color
	navSelected                ui.Color
	rowHover, rowSelected      ui.Color
	divider, cellDivider       ui.Color
	// heatLow to heatHigh are busy cells, from barely to very.
	heatLow, heatHigh, heatSelected, heatText ui.Color
	closeHover, danger, success               ui.Color
	cpu, memory, disk, network, gpu           ui.Color
	graphBG                                   ui.Color
}

var (
	white = ui.RGB(255, 255, 255)
	black = ui.RGB(0, 0, 0)
)

func palette(dark bool, ac accent) colors {
	if dark {
		page := ui.Hex("#272727")
		k := colors{
			dark:         true,
			backdrop:     ui.Hex("#1f1f1f"),
			page:         page,
			pageBorder:   ui.Hex("#333333"),
			card:         ui.Hex("#2d2d2d"),
			cardBorder:   ui.Hex("#383838"),
			field:        ui.Hex("#2b2b2b"),
			fieldBorder:  ui.Hex("#404040"),
			text:         ui.Hex("#ffffff"),
			secondary:    ui.Hex("#c8c8c8"),
			tertiary:     ui.Hex("#969696"),
			accent:       ac.dark,
			onAccent:     black,
			hover:        white.Alpha(0.065),
			pressed:      white.Alpha(0.04),
			navSelected:  white.Alpha(0.08),
			rowHover:     white.Alpha(0.055).Over(page),
			rowSelected:  ac.dark.Alpha(0.20).Over(page),
			divider:      white.Alpha(0.09),
			cellDivider:  white.Alpha(0.09 * 0.35),
			heatLow:      page.Mix(ac.light, 0.12),
			heatHigh:     page.Mix(ac.light, 0.62),
			heatSelected: ac.dark.Alpha(0.28).Over(page),
			heatText:     ui.Hex("#ffffff"),
			closeHover:   ui.Hex("#c42b1c"),
			danger:       ui.Hex("#ff99a4"),
			success:      ui.Hex("#6ccb5f"),
			cpu:          ui.Hex("#4cc2ff"),
			memory:       ui.Hex("#c58af9"),
			disk:         ui.Hex("#6ccb5f"),
			network:      ui.Hex("#f2a950"),
			gpu:          ui.Hex("#4cc2ff"),
			graphBG:      ui.Hex("#1f1f1f"),
		}
		return k
	}
	page := ui.Hex("#fafafa")
	return colors{
		backdrop:     ui.Hex("#f3f3f3"),
		page:         page,
		pageBorder:   ui.Hex("#e5e5e5"),
		card:         ui.Hex("#ffffff"),
		cardBorder:   ui.Hex("#e6e6e6"),
		field:        ui.Hex("#ffffff"),
		fieldBorder:  ui.Hex("#d6d6d6"),
		text:         ui.Hex("#1b1b1b"),
		secondary:    ui.Hex("#5c5c5c"),
		tertiary:     ui.Hex("#757575"),
		accent:       ac.light,
		onAccent:     white,
		hover:        black.Alpha(0.045),
		pressed:      black.Alpha(0.03),
		navSelected:  black.Alpha(0.06),
		rowHover:     black.Alpha(0.035).Over(page),
		rowSelected:  ac.light.Alpha(0.14).Over(page),
		divider:      black.Alpha(0.08),
		cellDivider:  black.Alpha(0.08 * 0.35),
		heatLow:      page.Mix(ac.light, 0.07),
		heatHigh:     page.Mix(ac.light, 0.42),
		heatSelected: ac.light.Alpha(0.24).Over(page),
		heatText:     ui.Hex("#1b1b1b"),
		closeHover:   ui.Hex("#c42b1c"),
		danger:       ui.Hex("#c42b1c"),
		success:      ui.Hex("#107c10"),
		cpu:          ui.Hex("#0067c0"),
		memory:       ui.Hex("#881798"),
		disk:         ui.Hex("#107c10"),
		network:      ui.Hex("#9a5006"),
		gpu:          ui.Hex("#0067c0"),
		graphBG:      ui.Hex("#ffffff"),
	}
}

// uiTheme is mygo's theme for its widgets (dialogs, switches, radio
// buttons, selects, tooltips, toasts, scroll bars) in this palette.
func uiTheme(k *colors) *ui.Theme {
	var t ui.Theme
	if k.dark {
		t = *ui.DarkTheme()
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#2c2c2c"), ui.Hex("#383838"), ui.Hex("#333333")
		t.Border = ui.Hex("#404040")
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
