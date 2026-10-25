package app

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The line icons Task Manager's pages and commands use: Windows 11's own,
// the Segoe Fluent Icons (or Segoe MDL2 Assets) font's glyphs. Where the
// font isn't installed, the same icons as SVG on a 24-unit grid. Tinted
// with the text's color.

// Icon is an icon of the app.
type Icon uint8

const (
	icMenu Icon = iota
	icProcesses
	icPerformance
	icHistory
	icStartup
	icUsers
	icDetails
	icServices
	icSettings
	icRun
	icEndTask
	icEfficiency
	icView
	icSearch
	icMore
	icPlay
	icStop
	icRestart
	icEnable
	icDisable
	icProperties
	icFolder
	icChevronRight
	icChevronDown
	icChevronUp
	icCopy
	icPause
	icGlobe
	icUser
	icDismiss
	icMinimize
	icMaximize
	icRestore
	icClose
	iconCount
)

// glyphs are the code points in Segoe Fluent Icons / Segoe MDL2 Assets.
var glyphs = [iconCount]string{
	icMenu:         "\uE700", // GlobalNavButton
	icProcesses:    "\uE737", // App grid
	icPerformance:  "\uE9D9", // Diagnostic
	icHistory:      "\uE81C", // History
	icStartup:      "\uEC4A", // SpeedHigh
	icUsers:        "\uE716", // People
	icDetails:      "\uE8FD", // BulletedList
	icServices:     "\uEA86", // Puzzle
	icSettings:     "\uE713", // Settings
	icRun:          "\uE710", // Add
	icEndTask:      "\uEA39", // ErrorBadge
	icEfficiency:   "\uEC0A", // Eco
	icView:         "\uE890", // View
	icSearch:       "\uE721", // Search
	icMore:         "\uE712", // More
	icPlay:         "\uE768", // Play
	icStop:         "\uE71A", // Stop
	icRestart:      "\uE72C", // Refresh
	icEnable:       "\uE73E", // CheckMark
	icDisable:      "\uE711", // Cancel
	icProperties:   "\uE946", // Info
	icFolder:       "\uE838", // FolderOpen
	icChevronRight: "\uE76C",
	icChevronDown:  "\uE70D",
	icChevronUp:    "\uE70E",
	icCopy:         "\uE8C8",
	icPause:        "\uE769",
	icGlobe:        "\uE774",
	icUser:         "\uE77B", // Contact
	icDismiss:      "\uE894", // Clear
	icMinimize:     "\uE921", // ChromeMinimize
	icMaximize:     "\uE922", // ChromeMaximize
	icRestore:      "\uE923", // ChromeRestore
	icClose:        "\uE8BB", // ChromeClose
}

// iconFont is the icon font's family; "" when neither is installed.
var iconFont = func() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	if _, err := os.Stat(filepath.Join(root, "Fonts", "SegoeIcons.ttf")); err == nil {
		return "Segoe Fluent Icons"
	}
	if _, err := os.Stat(filepath.Join(root, "Fonts", "segmdl2.ttf")); err == nil {
		return "Segoe MDL2 Assets"
	}
	return ""
}()

func svgDoc(body string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`
}

// gear is a gear's outline: teeth teeth round (12, 12).
func gear(teeth int, rOut, rIn float64) string {
	var b strings.Builder
	n := teeth * 4
	for i := range n {
		a := (float64(i) + 0.5) / float64(n) * 2 * math.Pi
		r := rIn
		if i%4 < 2 {
			r = rOut
		}
		cmd := "L"
		if i == 0 {
			cmd = "M"
		}
		fmt.Fprintf(&b, "%s%.2f %.2f ", cmd, 12+r*math.Cos(a), 12+r*math.Sin(a))
	}
	return b.String() + "Z"
}

var svgSources = [iconCount]string{
	icMenu:         svgDoc(`<path d="M4 6.5h16M4 12h16M4 17.5h16"/>`),
	icProcesses:    svgDoc(`<rect x="3.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="3.5" width="7" height="7" rx="1.5"/><rect x="3.5" y="13.5" width="7" height="7" rx="1.5"/><rect x="13.5" y="13.5" width="7" height="7" rx="1.5"/>`),
	icPerformance:  svgDoc(`<rect x="3" y="4" width="18" height="16" rx="2.5"/><path d="M6 13h3l2-4 3 7 2-3h2"/>`),
	icHistory:      svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M12 7.5V12l3 2"/>`),
	icStartup:      svgDoc(`<path d="M4.2 17.5A8.5 8.5 0 1 1 19.8 17.5"/><path d="M12 13.5l4-5"/><circle cx="12" cy="14" r="1.2"/>`),
	icUsers:        svgDoc(`<circle cx="9" cy="8" r="3.5"/><path d="M2.5 20c0-3.6 2.9-6 6.5-6s6.5 2.4 6.5 6"/><circle cx="17" cy="9" r="2.6"/><path d="M16.5 14.2c2.9.2 5 2.3 5 5.3"/>`),
	icDetails:      svgDoc(`<path d="M9 6.5h11M9 12h11M9 17.5h11"/><circle cx="4.8" cy="6.5" r="0.9"/><circle cx="4.8" cy="12" r="0.9"/><circle cx="4.8" cy="17.5" r="0.9"/>`),
	icServices:     svgDoc(`<path d="M5 8.5h3.2a2.3 2.3 0 1 1 4.6 0H16v3.2a2.3 2.3 0 1 1 0 4.6V19.5H12.8a2.3 2.3 0 1 0-4.6 0H5V16.3a2.3 2.3 0 1 0 0-4.6Z"/>`),
	icSettings:     svgDoc(`<path d="` + gear(8, 9, 7) + `"/><circle cx="12" cy="12" r="3"/>`),
	icRun:          svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M12 8v8M8 12h8"/>`),
	icEndTask:      svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M9 9l6 6M15 9l-6 6"/>`),
	icEfficiency:   svgDoc(`<path d="M5 19c0-8.5 5-13 14.5-14C19 14 14.5 19 5 19Z"/><path d="M5 19l8-8"/>`),
	icView:         svgDoc(`<path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12Z"/><circle cx="12" cy="12" r="3"/>`),
	icSearch:       svgDoc(`<circle cx="10.5" cy="10.5" r="6.5"/><path d="M15.5 15.5L20.5 20.5"/>`),
	icMore:         `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="#000"><circle cx="5.5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="18.5" cy="12" r="1.5"/></svg>`,
	icPlay:         svgDoc(`<path d="M7 4.8v14.4L19 12Z"/>`),
	icStop:         svgDoc(`<rect x="5.5" y="5.5" width="13" height="13" rx="2"/>`),
	icRestart:      svgDoc(`<path d="M19.5 12a7.5 7.5 0 1 1-2.2-5.3"/><path d="M19.5 4v4.5H15"/>`),
	icEnable:       svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M8 12.3l2.7 2.7L16 9.5"/>`),
	icDisable:      svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M8 12h8"/>`),
	icProperties:   svgDoc(`<rect x="4" y="3.5" width="16" height="17" rx="2.5"/><path d="M8 8.5h8M8 12h8M8 15.5h5"/>`),
	icFolder:       svgDoc(`<path d="M3 6.5a2 2 0 0 1 2-2h4l2 2.5h8a2 2 0 0 1 2 2V17.5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"/>`),
	icChevronRight: svgDoc(`<path d="M9.5 6l6 6-6 6"/>`),
	icChevronDown:  svgDoc(`<path d="M6 9.5l6 6 6-6"/>`),
	icChevronUp:    svgDoc(`<path d="M6 14.5l6-6 6 6"/>`),
	icCopy:         svgDoc(`<rect x="8" y="8" width="12" height="12.5" rx="2"/><path d="M16 5.5V5a1.5 1.5 0 0 0-1.5-1.5h-9A1.5 1.5 0 0 0 4 5v9.5A1.5 1.5 0 0 0 5.5 16H6"/>`),
	icPause:        svgDoc(`<path d="M8.5 5.5v13M15.5 5.5v13"/>`),
	icGlobe:        svgDoc(`<circle cx="12" cy="12" r="8.5"/><path d="M3.5 12h17M12 3.5c2.5 2.5 3.5 5.5 3.5 8.5s-1 6-3.5 8.5C9.5 18 8.5 15 8.5 12s1-6 3.5-8.5Z"/>`),
	icUser:         svgDoc(`<circle cx="12" cy="8" r="4"/><path d="M4.5 20.5c0-4 3.4-6.5 7.5-6.5s7.5 2.5 7.5 6.5"/>`),
	icDismiss:      svgDoc(`<path d="M5 5l14 14M19 5L5 19"/>`),
	icMinimize:     svgDoc(`<path d="M3 12h18"/>`),
	icMaximize:     svgDoc(`<rect x="3" y="3" width="18" height="18" rx="2"/>`),
	icRestore:      svgDoc(`<rect x="3" y="7" width="14" height="14" rx="2"/><path d="M7 3h12a2 2 0 0 1 2 2v12"/>`),
	icClose:        svgDoc(`<path d="M5 5l14 14M19 5L5 19"/>`),
}

var svgs [iconCount]*ui.SVG

// icon draws ic size DIPs square in color: the font's glyph, in a box a
// little larger so it never clips at fractional scales.
func icon(c *ui.Context, ic Icon, size float32, color ui.Color) ui.Element {
	size = float32(math.Round(float64(size)))
	if iconFont != "" {
		box := ui.Row(c).Size(size+2, size+2).Center().Shrink(0)
		box.Children(func() {
			ui.Text(c, glyphs[ic]).Font(iconFont).FontSize(size).FixedLineHeight(size + 2).TextColor(color).NoWrap()
		})
		return box
	}
	if svgs[ic] == nil {
		svgs[ic] = ui.MustParseSVG([]byte(svgSources[ic]))
	}
	return ui.Icon(c, svgs[ic]).Size(size+2, size+2).FontSize(size).TextColor(color).Shrink(0)
}
