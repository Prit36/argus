package app

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// The pieces the pages are made of, in Windows 11's manner: a page's title
// over its command bar, command buttons (an icon and a label), the tables'
// sortable headers and cells, and the "busy" heat of a figure.

const (
	rowH           = 32
	headerH        = 58
	compactHeaderH = 38
	pagePad        = 24
	pageHeaderH    = 52
)

// pageHeader is the page's title and its command bar; fn adds the commands.
func pageHeader(c *ui.Context, k *colors, title string, fn func()) {
	ui.Row(c).FillWidth().Height(pageHeaderH).AlignItems(ui.Center).Gap(4).Padding(0, pagePad-8, 0, pagePad).Shrink(0).Children(func() {
		ui.Text(c, title).FontSize(fontSubtitle).FontWeight(600).TextColor(k.text).NoWrap()
		ui.Spacer(c)
		if fn != nil {
			fn()
		}
	})
}

// command is a command bar's button.
type command struct {
	icon    Icon
	label   string
	enabled bool
	// compact keeps only the icon (the label goes to a tooltip), when the
	// bar is narrow.
	compact bool
	// menu, when set, opens below the button.
	menu func(m *ui.Menu)
}

// commandButton draws a command; whether it was clicked.
func commandButton(c *ui.Context, k *colors, cmd command) bool {
	showLabel := cmd.label != "" && !cmd.compact
	b := ui.ButtonBase(c.Key(cmd.icon)).Height(32).Gap(8).Radius(4).Shrink(0)
	if showLabel {
		b.Padding(0, 10)
	} else {
		b.Padding(0, 8)
	}
	if !cmd.enabled {
		b.Opacity(0.42).Disabled(true)
	} else if b.Pressed() {
		b.Background(k.pressed)
	} else if b.Hovered() {
		b.Background(k.hover)
	}
	if !showLabel {
		tip := cmd.label
		if tip == "" {
			tip = "More options"
		}
		b.Tooltip(tip)
	} else {
		b.Label(cmd.label)
	}
	if cmd.menu != nil && cmd.enabled {
		b.Menu(cmd.menu)
	}
	b.Children(func() {
		icon(c, cmd.icon, 15, k.text)
		if showLabel {
			ui.Text(c, cmd.label).FontSize(fontCaption).TextColor(k.text).NoWrap()
		}
	})
	return cmd.enabled && b.Clicked()
}

// separator is a thin vertical line between command groups.
func separator(c *ui.Context, k *colors) {
	ui.Row(c).Size(11, 20).Center().Shrink(0).Children(func() {
		ui.Box(c).Size(1, 20).Background(k.divider)
	})
}

// column is a table's column.
type column struct {
	title string
	// width in DIPs; 0 shares the room left with the table's other such
	// columns.
	width float32
	right bool
	// total is a figure over the title (the Processes page's "12%"), and
	// totalHeat how busy it is (0–1): past 0.85 it turns red.
	total     string
	totalHeat float32
}

// cell is a table cell's box.
func cell(c *ui.Context, col *column) ui.Element {
	e := ui.Row(c).FillHeight().AlignItems(ui.Center).Padding(0, 10).Clip()
	if col.width > 0 {
		e.Width(col.width).Shrink(0)
	} else {
		e.Grow(1).MinWidth(0)
	}
	if col.right {
		e.Justify(ui.End)
	}
	return e
}

// tableHeader draws a table's titles, sorted by the clicked one (its
// chevron at the top), in an element that scrolls sideways with the rows;
// the index of the column clicked, or -1. A table that isn't sortable has
// titles that don't light up.
func tableHeader(c *ui.Context, k *colors, cols []column, sorted int, ascending bool, h, minW float32, sortable bool, menu func(m *ui.Menu)) (int, ui.Element) {
	clicked := -1
	clip := ui.Row(c).FillWidth().Height(h).Clip().Shrink(0)
	if menu != nil {
		clip.ContextMenu(menu)
	}
	clip.Children(func() {
		ui.Row(c).FillWidth().MinWidth(minW).Height(h).AlignItems(ui.Stretch).Padding(0, 14, 0, pagePad-10).Shrink(0).Children(func() {
			for i := range cols {
				col := &cols[i]
				if i > 0 {
					ui.Box(c).Width(1).Padding(6, 0).FillHeight().Shrink(0).Children(func() {
						ui.Box(c).Width(1).Grow(1).Background(k.divider.Alpha(0.5))
					})
				}
				var d ui.Element
				if sortable {
					d = ui.ButtonBase(c.Key(col.title))
					if d.Hovered() {
						d.Background(k.hover)
					}
				} else {
					d = ui.Box(c.Key(col.title))
				}
				d.Column().FillHeight().Padding(0, 10, 7, 10).Gap(0)
				if col.width > 0 {
					d.Width(col.width).Shrink(0)
				} else {
					d.Grow(1).MinWidth(0)
				}
				if col.right {
					d.AlignItems(ui.End)
				} else {
					d.AlignItems(ui.Start)
				}
				d.Justify(ui.Start)
				if sortable && d.Clicked() {
					clicked = i
				}
				on := sorted == i
				d.Children(func() {
					// The chevron, centered at the top, on the sorted column.
					ui.Row(c).FillWidth().Height(12).Padding(2, 0, 0, 0).Justify(ui.Center).Shrink(0).Children(func() {
						if on {
							ic := icChevronDown
							if ascending {
								ic = icChevronUp
							}
							icon(c, ic, 10, k.secondary)
						}
					})
					ui.Spacer(c)
					if col.total != "" {
						color := k.text
						if col.totalHeat > 0.85 {
							color = k.danger
						}
						ui.Text(c, col.total).FontSize(fontMetric).FontWeight(600).TextColor(color).NoWrap()
					}
					t := ui.Text(c, col.title).FontSize(fontSmall).SingleLine()
					if on {
						t.FontWeight(600).TextColor(k.text)
					} else {
						t.TextColor(k.secondary)
					}
				})
			}
		})
	})
	ui.Box(c).FillWidth().Height(1).Background(k.divider).Shrink(0)
	return clicked, clip
}

// tableList is a table's rows, built only as they scroll into view, at
// least minW wide (they scroll sideways with header where wider than the
// table). The chosen row is tinted as Task Manager tints it.
func (a *App) tableList(c *ui.Context, key string, s *ui.ListState, n int, minW float32, header ui.Element, row func(ui.ListRow)) ui.Element {
	c.SetTheme(a.listTheme)
	list := ui.List(c.Key(key), s, n).Grow(1).MinHeight(0).FillWidth().RowWidth(minW)
	list.Rows(row)
	c.SetTheme(a.uiTheme)
	if header.Valid() {
		header.ScrollWith(list)
	}
	return list
}

// heatCell is a figure's cell, its background as busy as share says.
func heatCell(c *ui.Context, k *colors, col *column, text string, share float32, selected bool) {
	e := cell(c, col)
	if selected {
		e.Background(k.heatSelected)
	} else {
		e.Background(k.heat(share))
	}
	if text != "" {
		e.Children(func() {
			ui.Text(c, text).FontSize(fontCaption).TextColor(k.heatText).SingleLine()
		})
	}
}

// textCell is a plain cell of text.
func textCell(c *ui.Context, col *column, text string, color ui.Color) {
	e := cell(c, col)
	if text != "" {
		e.Children(func() {
			ui.Text(c, text).FontSize(fontCaption).TextColor(color).SingleLine()
		})
	}
}

// kv is a line of the Performance page's details: label, then value.
func kv(c *ui.Context, k *colors, label, value string) {
	ui.Row(c).FillWidth().Height(24).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Row(c).Width(148).Shrink(0).Clip().Children(func() {
			ui.Text(c, label).FontSize(fontSmall).TextColor(k.secondary).SingleLine()
		})
		ui.Row(c).Grow(1).MinWidth(0).FillHeight().AlignItems(ui.Center).Clip().Tooltip(value).Children(func() {
			ui.Text(c, value).FontSize(fontCaption).FontWeight(600).TextColor(k.text).SingleLine()
		})
	})
}

// stat is a big figure under its label, as the Performance page's.
func stat(c *ui.Context, k *colors, label, value string) {
	ui.Column(c).Gap(2).MinWidth(96).Padding(0, 20, 10, 0).Shrink(0).Children(func() {
		ui.Text(c, label).FontSize(fontSmall).TextColor(k.secondary).NoWrap()
		ui.Text(c, value).FontSize(fontSubtitle).FontWeight(600).TextColor(k.text).NoWrap()
	})
}

// empty is what an empty table says, under its header.
func empty(c *ui.Context, k *colors, message string) {
	ui.Column(c).FillWidth().Padding(64, pagePad, 0, pagePad).AlignItems(ui.Center).Children(func() {
		ui.Text(c, message).FontSize(fontBody).TextColor(k.secondary).SingleLine()
	})
}

// noResults is what a search that finds nothing says.
func noResults(query string) string {
	return `No results for "` + strings.TrimSpace(query) + `"`
}

// narrowerThan reports whether the page's room is narrower than w.
func (a *App) narrowerThan(c *ui.Context, w float32) bool {
	ww, _ := c.Size()
	return ww-a.navWidth(ww) < w
}
