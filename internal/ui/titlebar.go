package ui

import (
	"gioui.org/font"
	"gioui.org/io/system"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

var (
	icClose    = mustIcon(icons.NavigationClose)
	icMinimize = mustIcon(icons.ContentRemove)
	icMaximize = mustIcon(icons.ImageCropSquare)
)

// titleBar replaces the compositor's decorations (COSMIC gives Gio none): drag area and window buttons.
func (a *App) titleBar(gtx layout.Context) layout.Dimensions {
	if act := a.deco.Update(gtx); act != 0 {
		if act&system.ActionClose != 0 && a.installing() {
			act &^= system.ActionClose // the install keeps running; closing now would hide its progress
		}
		a.w.Perform(act)
	}
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return background(gtx, rgb(0x141414), 0, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
				return a.deco.LayoutMove(gtx, func(gtx layout.Context) layout.Dimensions {
					gtx.Constraints.Min.X = gtx.Constraints.Max.X
					return layout.Inset{Top: 8, Bottom: 8, Left: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawImage(gtx, a.logoSmall, 20) }),
							hspace(8),
							layout.Rigid(label(a.th, 13, "Install Zainium OS", colText, font.SemiBold).Layout),
						)
					})
				})
			}),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.winButton(gtx, system.ActionMinimize, icMinimize) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return a.winButton(gtx, system.ActionMaximize, icMaximize) }),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				if a.installing() {
					return layout.Dimensions{}
				}
				return a.winButton(gtx, system.ActionClose, icClose)
			}),
		)
	})
}

func (a *App) winButton(gtx layout.Context, act system.Action, ic *widget.Icon) layout.Dimensions {
	c := a.deco.Clickable(act)
	return c.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := rgb(0x141414)
		if c.Hovered() {
			bg = colCardHov
			if act == system.ActionClose {
				bg = rgb(0xc42b1c)
			}
		}
		return background(gtx, bg, 0, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 8, Bottom: 8, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return icon(gtx, ic, colText, unit.Dp(18))
			})
		})
	})
}

func (a *App) installing() bool {
	if a.inst == nil {
		return false
	}
	st := a.inst.snapshot()
	return !st.done && st.err == ""
}
