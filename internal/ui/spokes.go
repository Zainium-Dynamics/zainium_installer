package ui

import (
	"fmt"
	"image/color"
	"strings"
	"unicode"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
)

var spokeTitles = map[page]string{
	pageKeyboard: "KEYBOARD LAYOUT",
	pageLanguage: "LANGUAGE SUPPORT",
	pageTime:     "TIME & DATE",
	pageDisk:     "INSTALLATION DESTINATION",
	pageHost:     "HOST NAME",
	pageRoot:     "ROOT ACCOUNT",
	pageUser:     "CREATE USER",
}

func (a *App) layoutSpoke(gtx layout.Context) {
	if a.doneBtn.Clicked(gtx) {
		a.page = pageHub
		a.w.Invalidate()
	}
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.header(gtx, spokeTitles[a.page]) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			return layout.Inset{Top: 24, Bottom: 24, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				switch a.page {
				case pageKeyboard:
					return a.keyboardSpoke(gtx)
				case pageLanguage:
					return a.languageSpoke(gtx)
				case pageTime:
					return a.timeSpoke(gtx)
				case pageDisk:
					return a.diskSpoke(gtx)
				case pageHost:
					return a.form(gtx, a.hostSpoke)
				case pageRoot:
					return a.form(gtx, a.rootSpoke)
				case pageUser:
					return a.form(gtx, a.userSpoke)
				}
				return layout.Dimensions{}
			})
		}),
		layout.Rigid(a.spokeFooter),
	)
}

// spokeFooter is the bottom bar of a spoke: its current status on the left, Done on the right.
func (a *App) spokeFooter(gtx layout.Context) layout.Dimensions {
	status, ok := "", true
	for _, t := range a.tiles {
		if t.target == a.page {
			status, ok = t.status()
		}
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return background(gtx, colHeader, 0, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: 14, Bottom: 14, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					if ok {
						return icon(gtx, icCheck, colOK, 18)
					}
					return icon(gtx, icWarn, colWarn, 18)
				}),
				hspace(8),
				layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
					c := colDim
					if !ok {
						c = colWarn
					}
					l := label(a.th, 13, status, c, font.Medium)
					l.MaxLines = 1
					return l.Layout(gtx)
				}),
				hspace(16),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return button(a.th, gtx, &a.doneBtn, "Done", true, true)
				}),
			)
		})
	})
}

func (a *App) intro(gtx layout.Context, title, text string) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(label(a.th, 19, title, colText, font.SemiBold).Layout),
		vspace(6),
		layout.Rigid(label(a.th, 14, text, colDim, font.Normal).Layout),
		vspace(14),
	)
}

func (a *App) keyboardSpoke(gtx layout.Context) layout.Dimensions {
	layout_, _ := a.keyboardLayout()
	note := "The layout is switched now, so you can try it in the field below."
	if strings.Contains(layout_, ",") {
		note = "English (US) is added as a second layout for typing user names. Switch with Alt+Shift."
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Which keyboard layout do you use?",
				"Choose the layout printed on your keyboard. It is used on the login screen and the desktop.")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			d, changed := a.keyboards.Layout(a.th, gtx, "Search layouts")
			if changed {
				a.keyboardSet = true
				a.applyLiveKeyboard()
			}
			return d
		}),
		vspace(14),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(a.th, gtx, &a.kbTest, "Test your keyboard", "Type here", note, colDim)
		}),
	)
}

func (a *App) languageSpoke(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Which language should Zainium OS use?",
				"Menus, apps and messages of the installed system use this language. The keyboard and time zone follow it until you choose them yourself.")
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			d, changed := a.langs.Layout(a.th, gtx, "Search languages")
			if changed {
				if it, ok := a.langs.SelectedItem(); ok {
					a.applyLanguage(it.Key)
				}
			}
			return d
		}),
	)
}

func (a *App) timeSpoke(gtx layout.Context) layout.Dimensions {
	it, _ := a.zones.SelectedItem()
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Where are you?", "Search for your country or city. The clock is set from the internet once you are connected.")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return background(gtx, colCard, 10, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 12, Bottom: 12, Left: 16, Right: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icTime, colAccent, 26) }),
							hspace(12),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
									layout.Rigid(label(a.th, 15, it.Title, colText, font.SemiBold).Layout),
									layout.Rigid(label(a.th, 13, zoneTime(it.Key), colDim, font.Normal).Layout),
								)
							}),
						)
					})
				})
			})
		}),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			d, changed := a.zones.Layout(a.th, gtx, "Search country or city")
			if changed {
				a.zoneSet = true
			}
			return d
		}),
	)
}

func (a *App) diskSpoke(gtx layout.Context) layout.Dimensions {
	for i := range a.diskClicks {
		if a.diskClicks[i].Clicked(gtx) {
			if ok, _ := a.disks[i].Usable(); ok && a.diskSel != i {
				a.diskSel = i
				a.erase.Value = false
			}
		}
	}
	if a.rescan.Clicked(gtx) {
		a.scanDisks()
	}
	a.erase.Update(gtx)

	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Where should Zainium OS be installed?",
				"Select a disk. Zainium OS uses the whole disk: an EFI partition and one system partition are created.")
		}),
	}
	if len(a.disks) == 0 {
		children = append(children, layout.Rigid(label(a.th, 14, "No disks found.", colError, font.Medium).Layout))
	}
	for i := range a.disks {
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions { return a.diskCard(gtx, i) })
		}))
	}
	children = append(children,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 4, Bottom: 16}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return button(a.th, gtx, &a.rescan, "Refresh", false, true)
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if a.diskSel < 0 {
				return layout.Dimensions{}
			}
			d := a.disks[a.diskSel]
			return border(gtx, colWarn, 10, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.UniformInset(16).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icWarn, colWarn, 20) }),
								hspace(8),
								layout.Rigid(label(a.th, 15, "Everything on "+d.Title()+" will be erased", colWarn, font.SemiBold).Layout),
							)
						}),
						vspace(6),
						layout.Rigid(label(a.th, 13, partsSummary(d), colDim, font.Normal).Layout),
						vspace(10),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return checkbox(a.th, gtx, &a.erase, "I understand, erase this disk and install Zainium OS")
						}),
					)
				})
			})
		}),
	)
	return a.spokeList.Layout(gtx, len(children), func(gtx layout.Context, i int) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children[i])
	})
}

func partsSummary(d disk.Disk) string {
	if len(d.Partitions) == 0 {
		return "The disk is empty."
	}
	var parts []string
	for _, p := range d.Partitions {
		s := p.Name + " " + disk.HumanSize(p.Size)
		if p.Label != "" {
			s += " (" + p.Label + ")"
		}
		parts = append(parts, s)
	}
	return "Partitions that will be deleted: " + strings.Join(parts, ", ")
}

func (a *App) diskCard(gtx layout.Context, i int) layout.Dimensions {
	d := a.disks[i]
	usable, why := d.Usable()
	selected := i == a.diskSel
	return a.diskClicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg, bc := colCard, colBorder
		switch {
		case selected:
			bg, bc = colSelected, colAccent
		case usable && a.diskClicks[i].Hovered():
			bg = colCardHov
		}
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return border(gtx, bc, 10, func(gtx layout.Context) layout.Dimensions {
			return background(gtx, bg, 10, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					fg := colText
					if !usable {
						fg = rgb(0x777777)
					}
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icDisk, fg, 34) }),
						hspace(14),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							sub := d.Path
							if d.USB {
								sub += " · USB"
							}
							if len(d.Mounted) > 0 {
								sub += " · in use"
							}
							if !usable {
								sub += " · " + why
							}
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(label(a.th, 15, d.Title(), fg, font.SemiBold).Layout),
								layout.Rigid(label(a.th, 13, sub, colDim, font.Normal).Layout),
							)
						}),
						layout.Rigid(label(a.th, 15, disk.HumanSize(d.Size), fg, font.SemiBold).Layout),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							if !selected {
								return layout.Dimensions{}
							}
							return layout.Inset{Left: unit.Dp(12)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
								return icon(gtx, icCheck, colAccent, 22)
							})
						}),
					)
				})
			})
		})
	})
}

// form centers a fixed-width column of fields.
func (a *App) form(gtx layout.Context, w layout.Widget) layout.Dimensions {
	return a.spokeList.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions {
		return layout.N.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Max.X = min(gtx.Constraints.Max.X, gtx.Dp(560))
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return w(gtx)
		})
	})
}

func (a *App) hostSpoke(gtx layout.Context) layout.Dimensions {
	h := strings.TrimSpace(a.hostname.Text())
	msg, mc := "", colDim
	if err := plan.ValidHostname(h); err != nil {
		msg, mc = err.Error(), colError
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Name this computer", "Other devices on your network see this name.")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return background(gtx, colCard, 10, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				return layout.UniformInset(14).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icHost, colAccent, 26) }),
						hspace(12),
						layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(label(a.th, 12, "DETECTED DEVICE", colDim, font.Bold).Layout),
								layout.Rigid(label(a.th, 15, a.deviceName, colText, font.SemiBold).Layout),
							)
						}),
					)
				})
			})
		}),
		vspace(18),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(a.th, gtx, &a.hostname, "Host name", defaultHostname, msg, mc)
		}),
		vspace(8),
		layout.Rigid(label(a.th, 12, "Letters, digits and dashes. You can change it later in Settings.", colDim, font.Normal).Layout),
	)
}

func (a *App) rootProblem() string {
	if a.rootMode.Value != "enabled" {
		return ""
	}
	u := a.Plan().User
	if why := plan.PasswordProblemFor(a.rootPw.Text(), u.Name, u.FullName+" root"); why != "" {
		return "Password: " + why
	}
	if a.rootPw.Text() != a.rootPw2.Text() {
		return "The passwords do not match"
	}
	return ""
}

func (a *App) rootSpoke(gtx layout.Context) layout.Dimensions {
	a.rootMode.Update(gtx)
	children := []layout.FlexChild{
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Root account", "The root account has full control of the system. Administrators can already run admin tasks with their own password (elevate), so most people keep root disabled.")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return radio(a.th, gtx, &a.rootMode, "disabled", "Disable root account (recommended)")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return radio(a.th, gtx, &a.rootMode, "enabled", "Enable root account")
		}),
	}
	if a.rootMode.Value == "enabled" {
		prob := a.rootProblem()
		children = append(children,
			vspace(16),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				pw := a.rootPw.Text()
				msg, col := strengthText(pw), strengthColor(pw)
				u := a.Plan().User
				if why := plan.PasswordProblemFor(pw, u.Name, u.FullName+" root"); pw != "" && why != "" {
					msg, col = "Password "+why, colError
				}
				return editorBox(a.th, gtx, &a.rootPw, "Root password", "", msg, col)
			}),
			vspace(12),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				msg := ""
				if a.rootPw2.Text() != "" || a.rootPw.Text() != "" {
					msg = prob
				}
				return editorBox(a.th, gtx, &a.rootPw2, "Confirm root password", "", msg, colError)
			}),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

// suggestUsername turns "Ali Zain" into "alizain".
func suggestUsername(full string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(full) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9' && b.Len() > 0:
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-' || r == '_':
		}
		if b.Len() >= 32 {
			break
		}
	}
	return b.String()
}

func (a *App) userProblem() string {
	p := a.Plan()
	if err := plan.ValidUsername(p.User.Name); err != nil {
		return "User name: " + err.Error()
	}
	if strings.ContainsAny(p.User.FullName, ":\n") {
		return "Full name cannot contain ':'"
	}
	if why := plan.PasswordProblemFor(p.User.Password, p.User.Name, p.User.FullName); why != "" {
		return "Password: " + why
	}
	if a.userPw.Text() != a.userPw2.Text() {
		return "The passwords do not match"
	}
	return ""
}

func (a *App) userSpoke(gtx layout.Context) layout.Dimensions {
	// the user name follows the full name until the user edits it
	if fn := a.fullName.Text(); fn != a.lastFullName {
		a.lastFullName = fn
		if !a.userNameTouched || a.userName.Text() == a.lastAutoUser {
			a.lastAutoUser = suggestUsername(fn)
			a.userName.SetText(a.lastAutoUser)
		}
	}
	if a.userName.Text() != a.lastAutoUser {
		a.userNameTouched = true
	}
	a.admin.Update(gtx)

	name := strings.TrimSpace(a.userName.Text())
	nameMsg, nameCol := "Used to log in; your home folder gets this name", colDim
	if name != "" {
		if err := plan.ValidUsername(name); err != nil {
			nameMsg, nameCol = err.Error(), colError
		} else {
			nameMsg = "Home folder: /home/" + name
		}
	}
	confirmMsg := ""
	if a.userPw2.Text() != "" && a.userPw.Text() != a.userPw2.Text() {
		confirmMsg = "The passwords do not match"
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return a.intro(gtx, "Create your account", "This is the account you log in with.")
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(a.th, gtx, &a.fullName, "Full name", "Your name", "Shown on the login screen", colDim)
		}),
		vspace(14),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(a.th, gtx, &a.userName, "User name", "lowercase, no spaces", nameMsg, nameCol)
		}),
		vspace(10),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return checkbox(a.th, gtx, &a.admin, "Make this user an administrator (can install software and change system settings)")
		}),
		vspace(14),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			pw := a.userPw.Text()
			msg, col := strengthText(pw), strengthColor(pw)
			if pw == "" {
				msg = fmt.Sprintf("At least %d characters, without your name", plan.MinPassword)
			} else if why := plan.PasswordProblemFor(pw, name, a.fullName.Text()); why != "" {
				msg, col = "Password "+why, colError
			}
			return editorBox(a.th, gtx, &a.userPw, "Password", "", msg, col)
		}),
		vspace(14),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return editorBox(a.th, gtx, &a.userPw2, "Confirm password", "", confirmMsg, colError)
		}),
	)
}

// strength is a rough 0..4 score shown under password fields.
func strength(pw string) int {
	if pw == "" {
		return -1
	}
	if plan.PasswordProblem(pw) != "" {
		return 0
	}
	classes := 0
	for _, f := range []func(rune) bool{unicode.IsLower, unicode.IsUpper, unicode.IsDigit, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }} {
		if strings.IndexFunc(pw, f) >= 0 {
			classes++
		}
	}
	s := 1
	if len(pw) >= 10 {
		s++
	}
	if classes >= 3 {
		s++
	}
	if len(pw) >= 14 && classes >= 2 {
		s++
	}
	return min(s, 4)
}

func strengthText(pw string) string {
	s := strength(pw)
	if s < 0 {
		return ""
	}
	if s == 0 {
		return "Too short: " + plan.PasswordProblem(pw)
	}
	return fmt.Sprintf("Strength: %s", []string{"", "Weak", "Fair", "Good", "Strong"}[s])
}

func strengthColor(pw string) color.NRGBA {
	switch strength(pw) {
	case -1:
		return colDim
	case 0:
		return colError
	case 1:
		return colWarn
	case 2:
		return rgb(0xd8c34a)
	}
	return colOK
}
