package app

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// The Settings page: the theme, the update speed, the page to start on,
// and window options, as cards of Windows 11's settings rows. Kept
// between runs (settings.ini).

var startPages = [...]Page{PageProcesses, PagePerformance, PageAppHistory, PageStartup, PageUsers, PageDetails, PageServices}

func (a *App) settingsPage(c *ui.Context, k *colors) {
	pageHeader(c, k, "Settings", nil)
	w, _ := c.Size()
	room := w - a.navWidth(w)
	roomy := room >= 480
	ui.Scroll(c.Key("settings")).FillWidth().Grow(1).MinHeight(0).Children(func() {
		ui.Column(c).FillWidth().Padding(0, pagePad, 24, pagePad).Gap(6).Children(func() {
			section(c, k, "Appearance", true)
			a.card(c, k, room, "App theme", "Choose how Task Manager looks", 350, func() {
				mode := a.set.Theme
				g := ui.RadioGroup(c, func() {
					ui.Radio(c, &mode, ThemeSystem, "Use system setting")
					ui.Radio(c, &mode, ThemeLight, "Light")
					ui.Radio(c, &mode, ThemeDark, "Dark")
				}).Label("App theme").Gap(16)
				if roomy {
					g.Row()
				}
				if mode != a.set.Theme {
					a.setTheme(mode)
				}
			})
			section(c, k, "Task Manager options", false)
			a.card(c, k, room, "Real time update speed", "How often the figures and graphs refresh", 300, func() {
				speed := a.set.Speed
				g := ui.RadioGroup(c, func() {
					for _, s := range [...]Speed{SpeedHigh, SpeedNormal, SpeedLow, SpeedPaused} {
						ui.Radio(c, &speed, s, s.Title())
					}
				}).Label("Real time update speed").Gap(16)
				if roomy {
					g.Row()
				}
				if speed != a.set.Speed {
					a.setSpeed(speed)
				}
			})
			a.card(c, k, room, "Default start page", "The page Task Manager opens on", 200, func() {
				var names [len(startPages)]string
				chosen := ""
				for i, p := range startPages {
					names[i] = p.Title()
					if p == a.set.StartPage {
						chosen = names[i]
					}
				}
				if ui.Select(c, &chosen, names[:]).Width(200).Label("Default start page").Changed() {
					for i, n := range names {
						if n == chosen {
							a.set.StartPage = startPages[i]
						}
					}
					a.saveSettings()
				}
			})
			a.toggleCard(c, k, room, "Always on top", "Keep Task Manager above other windows", &a.set.AlwaysOnTop, func() {
				if a.win != nil {
					a.win.SetAlwaysOnTop(a.set.AlwaysOnTop)
				}
			})
			a.toggleCard(c, k, room, "Group by type", "Show process categories; clicking a column sorts across all categories", &a.set.GroupByType, nil)
			section(c, k, "Window management", false)
			a.toggleCard(c, k, room, "Minimize on use", "Minimize Task Manager after switching to an app", &a.set.MinimizeOnUse, nil)
			a.toggleCard(c, k, room, "Hide when minimized", "Keep Task Manager in the notification area, not the taskbar, while minimized", &a.set.HideMinimized, nil)
			section(c, k, "Other options", false)
			a.toggleCard(c, k, room, "Show full account name", "On the Users page, the name each account belongs to", &a.set.FullAccountName, nil)
			a.toggleCard(c, k, room, "Show history for all processes", "App history lists every program of yours, not only apps", &a.set.HistoryAll, nil)
			a.toggleCard(c, k, room, "Ask me before applying Efficiency mode", "Confirm before lowering a process's priority and power use", &a.set.AskEfficiency, nil)
			if !a.netmon.Available {
				a.card(c, k, room, "Run as administrator", "Each process's network use, and ending and changing other users' processes, need administrator rights", 90, func() {
					if ui.Button(c, "Restart").Clicked() && relaunchAsAdmin() {
						mygo.App.Quit()
					}
				})
			}
			section(c, k, "About", false)
			a.card(c, k, room, "Keyboard shortcuts", "Ctrl+F: search  ·  Ctrl+P: pause or resume  ·  F5: refresh now  ·  Ctrl+C: copy selection  ·  Ctrl+1–8: switch pages", 0, func() {})
			a.card(c, k, room, "Task Manager on mygo", "A native Windows 11 Task Manager in Go, drawn by mygo", 120, func() {
				ui.Text(c, "mygo "+mygo.Version).FontSize(fontSmall).TextColor(k.secondary).NoWrap()
			})
		})
	})
}

func section(c *ui.Context, k *colors, title string, first bool) {
	top := float32(18)
	if first {
		top = 6
	}
	ui.Row(c).Padding(top, 0, 4, 0).Children(func() {
		ui.Text(c, title).FontSize(fontBody).FontWeight(600).TextColor(k.text).NoWrap()
	})
}

// card is a settings row: its title and description, then its control.
// Wide controls get their own line when the page is narrow.
func (a *App) card(c *ui.Context, k *colors, room float32, title, desc string, controlW float32, control func()) {
	narrow := room < 760 && controlW >= 300
	r := ui.Column(c.Key(title)).FillWidth().MinWidth(0).Gap(16).Padding(14, 18).Radius(6).Background(k.card).Border(1, k.cardBorder)
	if !narrow {
		r.Row().AlignItems(ui.Center)
	} else {
		r.AlignItems(ui.Start)
	}
	r.Children(func() {
		words := ui.Column(c).Gap(2)
		if narrow {
			words.FillWidth()
		} else {
			words.Grow(1).MinWidth(80)
		}
		words.Children(func() {
			ui.Text(c, title).FontSize(fontBody).TextColor(k.text)
			ui.Text(c, desc).FontSize(fontSmall).TextColor(k.secondary)
		})
		control()
	})
}

func (a *App) toggleCard(c *ui.Context, k *colors, room float32, title, desc string, v *bool, then func()) {
	a.card(c, k, room, title, desc, 44, func() {
		if ui.Switch(c, v).Label(title).Changed() {
			a.dirty = true
			a.saveSettings()
			if then != nil {
				then()
			}
		}
	})
}

// setTheme changes the app's theme, the window frame's and menus' too.
func (a *App) setTheme(mode ThemeMode) {
	a.set.Theme = mode
	switch mode {
	case ThemeLight:
		mygo.Theme.SetSource(mygo.ThemeLight)
	case ThemeDark:
		mygo.Theme.SetSource(mygo.ThemeDark)
	default:
		mygo.Theme.SetSource(mygo.ThemeSystem)
	}
	a.saveSettings()
}
