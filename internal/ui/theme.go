package ui

import (
	"image"
	"image/color"
	"os"
	"path/filepath"

	"gioui.org/font"
	"gioui.org/font/gofont"
	"gioui.org/font/opentype"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"golang.org/x/exp/shiny/materialdesign/icons"
)

// Dark COSMIC-style palette.
var (
	colBg       = rgb(0x1b1b1b)
	colHeader   = rgb(0x242424)
	colCard     = rgb(0x2a2a2a)
	colCardHov  = rgb(0x333333)
	colSelected = rgb(0x203a52)
	colBorder   = rgb(0x3a3a3a)
	colText     = rgb(0xececec)
	colDim      = rgb(0xa3a3a3)
	colAccent   = rgb(0x5ab0f6)
	colOnAccent = rgb(0x0d1b26)
	colWarn     = rgb(0xf5a524)
	colError    = rgb(0xff6b6b)
	colOK       = rgb(0x63d489)
)

func rgb(c uint32) color.NRGBA {
	return color.NRGBA{R: uint8(c >> 16), G: uint8(c >> 8), B: uint8(c), A: 0xff}
}

const fontDir = "/overlayer/syshub/share/fonts"

// newTheme uses Zainium's Noto Sans (the desktop font) when present, Go fonts otherwise.
func newTheme() *material.Theme {
	var faces []font.FontFace
	for _, f := range []string{"noto/NotoSans-Regular.ttf", "noto/NotoSans-Bold.ttf", "noto/NotoSans-Italic.ttf", "noto/NotoSansSymbols2-Regular.ttf"} {
		b, err := os.ReadFile(filepath.Join(fontDir, f))
		if err != nil {
			continue
		}
		if fs, err := opentype.ParseCollection(b); err == nil {
			faces = append(faces, fs...)
		}
	}
	faces = append(faces, gofont.Collection()...)
	th := material.NewTheme()
	th.Shaper = text.NewShaper(text.NoSystemFonts(), text.WithCollection(faces))
	th.Palette = material.Palette{Bg: colBg, Fg: colText, ContrastBg: colAccent, ContrastFg: colOnAccent}
	th.TextSize = 15
	return th
}

func mustIcon(data []byte) *widget.Icon {
	ic, err := widget.NewIcon(data)
	if err != nil {
		panic(err)
	}
	return ic
}

var (
	icKeyboard = mustIcon(icons.HardwareKeyboard)
	icLanguage = mustIcon(icons.ActionLanguage)
	icTime     = mustIcon(icons.DeviceAccessTime)
	icDisk     = mustIcon(icons.DeviceStorage)
	icHost     = mustIcon(icons.HardwareComputer)
	icRoot     = mustIcon(icons.ActionLock)
	icUser     = mustIcon(icons.SocialPerson)
	icWarn     = mustIcon(icons.AlertWarning)
	icCheck    = mustIcon(icons.ActionCheckCircle)
	icSearch   = mustIcon(icons.ActionSearch)
	icError    = mustIcon(icons.AlertError)
	icPending  = mustIcon(icons.ToggleRadioButtonUnchecked)
	icWorking  = mustIcon(icons.ActionAutorenew)
)

// fillRect paints a rounded rectangle the size of the constraints' minimum.
func fillRect(gtx layout.Context, c color.NRGBA, radius unit.Dp) layout.Dimensions {
	sz := gtx.Constraints.Min
	r := gtx.Dp(radius)
	paint.FillShape(gtx.Ops, c, clip.UniformRRect(image.Rectangle{Max: sz}, r).Op(gtx.Ops))
	return layout.Dimensions{Size: sz}
}

// background draws content on a rounded colored box.
func background(gtx layout.Context, c color.NRGBA, radius unit.Dp, w layout.Widget) layout.Dimensions {
	m := op.Record(gtx.Ops)
	dims := w(gtx)
	call := m.Stop()
	r := gtx.Dp(radius)
	paint.FillShape(gtx.Ops, c, clip.UniformRRect(image.Rectangle{Max: dims.Size}, r).Op(gtx.Ops))
	call.Add(gtx.Ops)
	return dims
}

// border strokes a rounded outline around content.
func border(gtx layout.Context, c color.NRGBA, radius unit.Dp, w layout.Widget) layout.Dimensions {
	return widget.Border{Color: c, CornerRadius: radius, Width: unit.Dp(1)}.Layout(gtx, w)
}

func label(th *material.Theme, size unit.Sp, txt string, c color.NRGBA, weight font.Weight) material.LabelStyle {
	l := material.Label(th, size, txt)
	l.Color = c
	l.Font.Weight = weight
	return l
}

// button is a flat rounded button; primary ones use the accent color.
func button(th *material.Theme, gtx layout.Context, b *widget.Clickable, txt string, primary, enabled bool) layout.Dimensions {
	gtx.Constraints.Min = image.Point{} // size to the label, not the parent
	bg, fg := colCard, colText
	if primary {
		bg, fg = colAccent, colOnAccent
	}
	if !enabled {
		bg, fg = rgb(0x2a2a2a), rgb(0x666666)
		gtx = gtx.Disabled()
	} else if b.Hovered() {
		if primary {
			bg = rgb(0x7cc2f8)
		} else {
			bg = colCardHov
		}
	}
	return b.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return background(gtx, bg, 8, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: 9, Bottom: 9, Left: 20, Right: 20}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return label(th, 14, txt, fg, font.SemiBold).Layout(gtx)
			})
		})
	})
}

// icon draws an icon at a fixed size and color.
func icon(gtx layout.Context, ic *widget.Icon, c color.NRGBA, size unit.Dp) layout.Dimensions {
	s := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(s, s))
	return ic.Layout(gtx, c)
}

// editorBox is a single-line text field with a label above it and an optional message below.
func editorBox(th *material.Theme, gtx layout.Context, ed *widget.Editor, title, hint, msg string, msgCol color.NRGBA) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			if title == "" {
				return layout.Dimensions{}
			}
			return layout.Inset{Bottom: 6}.Layout(gtx, label(th, 13, title, colDim, font.Medium).Layout)
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			bc := colBorder
			if gtx.Focused(ed) {
				bc = colAccent
			}
			return border(gtx, bc, 8, func(gtx layout.Context) layout.Dimensions {
				return background(gtx, colCard, 8, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 9, Bottom: 9, Left: 12, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						gtx.Constraints.Min.X = gtx.Constraints.Max.X
						e := material.Editor(th, ed, hint)
						e.HintColor = rgb(0x6f6f6f)
						e.SelectionColor = rgb(0x2f5f8a)
						return e.Layout(gtx)
					})
				})
			})
		}),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			// the hint line is always there, so fields never move while the user types
			if msg == "" {
				msg = " "
			}
			l := label(th, 12, msg, msgCol, font.Normal)
			l.MaxLines = 1
			return layout.Inset{Top: 5}.Layout(gtx, l.Layout)
		}),
	)
}

// checkbox with the theme's accent.
func checkbox(th *material.Theme, gtx layout.Context, b *widget.Bool, txt string) layout.Dimensions {
	c := material.CheckBox(th, b, txt)
	c.Color = colText
	c.IconColor = colAccent
	c.TextSize = 14
	return c.Layout(gtx)
}

func radio(th *material.Theme, gtx layout.Context, e *widget.Enum, key, txt string) layout.Dimensions {
	r := material.RadioButton(th, e, key, txt)
	r.Color = colText
	r.IconColor = colAccent
	r.TextSize = 14
	return r.Layout(gtx)
}

// spacer takes all free room in a Flex, pushing what follows to the end.
func spacer() layout.FlexChild {
	return layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: gtx.Constraints.Min}
	})
}

// hspace / vspace are fixed gaps.
func hspace(d unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(gtx.Dp(d), 0)}
	})
}

func vspace(d unit.Dp) layout.FlexChild {
	return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return layout.Dimensions{Size: image.Pt(0, gtx.Dp(d))}
	})
}
