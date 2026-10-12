package ui

import (
	"fmt"
	"os"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
)

type tile struct {
	click   widget.Clickable
	section string
	title   string
	ic      *widget.Icon
	target  page
	// status returns the line under the title and whether the spoke is complete.
	status func() (string, bool)
}

func (a *App) newTiles() []*tile {
	return []*tile{
		{section: "LOCALIZATION", title: "Keyboard", ic: icKeyboard, target: pageKeyboard, status: func() (string, bool) {
			it, ok := a.keyboards.SelectedItem()
			return it.Title, ok
		}},
		{section: "LOCALIZATION", title: "Language", ic: icLanguage, target: pageLanguage, status: func() (string, bool) {
			it, ok := a.langs.SelectedItem()
			return it.Title, ok
		}},
		{section: "LOCALIZATION", title: "Time & Date", ic: icTime, target: pageTime, status: func() (string, bool) {
			it, ok := a.zones.SelectedItem()
			return it.Title + " time zone", ok
		}},
		{section: "SYSTEM", title: "Installation Destination", ic: icDisk, target: pageDisk, status: func() (string, bool) {
			if a.diskSel < 0 {
				return "No disk selected", false
			}
			d := a.disks[a.diskSel]
			if !a.erase.Value {
				return "Confirm erasing " + d.Title(), false
			}
			return fmt.Sprintf("Erase %s, %s", d.Title(), disk.HumanSize(d.Size)), true
		}},
		{section: "SYSTEM", title: "Host Name", ic: icHost, target: pageHost, status: func() (string, bool) {
			h := a.Plan().Hostname
			if plan.ValidHostname(h) != nil {
				return "Invalid host name", false
			}
			return h, true
		}},
		{section: "USER SETTINGS", title: "Root Account", ic: icRoot, target: pageRoot, status: func() (string, bool) {
			if a.rootMode.Value != "enabled" {
				return "Root account is disabled", true
			}
			if a.rootProblem() != "" {
				return "Root password is not set", false
			}
			return "Root password is set", true
		}},
		{section: "USER SETTINGS", title: "User Creation", ic: icUser, target: pageUser, status: func() (string, bool) {
			p := a.Plan()
			if p.User.Name == "" {
				return "No user will be created", false
			}
			if a.userProblem() != "" {
				return "User settings need attention", false
			}
			if p.User.Admin {
				return "Administrator " + p.User.Name + " will be created", true
			}
			return "User " + p.User.Name + " will be created", true
		}},
	}
}

func (a *App) allComplete() bool {
	for _, t := range a.tiles {
		if _, ok := t.status(); !ok {
			return false
		}
	}
	return true
}

func (a *App) layoutHub(gtx layout.Context) {
	for _, t := range a.tiles {
		if t.click.Clicked(gtx) {
			a.page = t.target
			a.spokeList.Position.First = 0
			if t.target == pageDisk {
				a.scanDisks()
			}
			if p, ok := map[page]*pickList{pageKeyboard: a.keyboards, pageLanguage: a.langs, pageTime: a.zones}[t.target]; ok {
				p.scrollTo = true
			}
		}
	}
	if a.quitBtn.Clicked(gtx) {
		os.Exit(0)
	}
	ready := a.allComplete()
	if a.beginBtn.Clicked(gtx) && ready {
		a.startInstall()
	}

	sections := []string{"LOCALIZATION", "SYSTEM", "USER SETTINGS"}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.header(gtx, "INSTALLATION SUMMARY") }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return a.hubList.Layout(gtx, len(sections), func(gtx layout.Context, i int) layout.Dimensions {
				return layout.Inset{Top: 22, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return a.section(gtx, sections[i])
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.hubFooter(gtx, ready) }),
	)
}

func (a *App) section(gtx layout.Context, name string) layout.Dimensions {
	var tiles []*tile
	for _, t := range a.tiles {
		if t.section == name {
			tiles = append(tiles, t)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(label(a.th, 12, name, colDim, font.Bold).Layout),
		vspace(4),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			size := gtx.Constraints.Max
			size.Y = gtx.Dp(1)
			gtx.Constraints.Min = size
			return fillRect(gtx, colBorder, 0)
		}),
		vspace(10),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// two tiles per row, like Anaconda's hub
			cols := 2
			if gtx.Constraints.Max.X < gtx.Dp(700) {
				cols = 1
			}
			var rows []layout.FlexChild
			for r := 0; r < len(tiles); r += cols {
				row := tiles[r:min(r+cols, len(tiles))]
				rows = append(rows, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					var cells []layout.FlexChild
					for c := 0; c < cols; c++ {
						var t *tile
						if c < len(row) {
							t = row[c]
						}
						cells = append(cells, layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							if t == nil {
								return layout.Dimensions{}
							}
							return layout.UniformInset(4).Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.tile(gtx, t) })
						}))
					}
					return layout.Flex{}.Layout(gtx, cells...)
				}))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		}),
	)
}

func (a *App) tile(gtx layout.Context, t *tile) layout.Dimensions {
	status, ok := t.status()
	return t.click.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colBg
		if t.click.Hovered() {
			bg = colCard
		}
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return background(gtx, bg, 10, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return background(gtx, colCard, 10, func(gtx layout.Context) layout.Dimensions {
							return layout.UniformInset(10).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return icon(gtx, t.ic, colText, 28)
							})
						})
					}),
					hspace(14),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						sc := colDim
						if !ok {
							sc = colWarn
						}
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
									layout.Rigid(label(a.th, 15, t.title, colText, font.SemiBold).Layout),
									layout.Rigid(func(gtx layout.Context) layout.Dimensions {
										if ok {
											return layout.Dimensions{}
										}
										return layout.Inset{Left: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
											return icon(gtx, icWarn, colWarn, 16)
										})
									}),
								)
							}),
							vspace(2),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								l := label(a.th, 13, status, sc, font.Normal)
								l.MaxLines = 1
								return l.Layout(gtx)
							}),
						)
					}),
				)
			})
		})
	})
}

func (a *App) hubFooter(gtx layout.Context, ready bool) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return background(gtx, colHeader, 0, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Top: 14, Bottom: 14, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Flexed(1, label(a.th, 13, "Nothing is written to your disks until you click Begin Installation.", colDim, font.Normal).Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return button(a.th, gtx, &a.quitBtn, "Quit", false, true)
						}),
						hspace(12),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return button(a.th, gtx, &a.beginBtn, "Begin Installation", true, ready)
						}),
					)
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if ready {
				return layout.Dimensions{}
			}
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return background(gtx, rgb(0x4a3410), 0, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.Inset{Top: 8, Bottom: 8, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icWarn, colWarn, 18) }),
						hspace(8),
						layout.Rigid(label(a.th, 13, "Please complete the items marked with this icon before installing.", colWarn, font.Medium).Layout),
					)
				})
			})
		}),
	)
}
