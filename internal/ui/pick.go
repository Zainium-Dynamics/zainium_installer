package ui

import (
	"strings"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

type pickItem struct {
	Title string
	Sub   string // shown dimmed on the right
	Key   string
	// Search holds extra text the filter matches.
	Search string
}

// pickList is a searchable single-choice list (languages, time zones, keyboard layouts).
type pickList struct {
	items    []pickItem
	clicks   []widget.Clickable
	shown    []int
	search   widget.Editor
	list     widget.List
	query    string
	Selected int
	// scrollTo makes the next layout bring the selection into view.
	scrollTo bool
}

func newPickList(items []pickItem, selectedKey string) *pickList {
	p := &pickList{items: items, clicks: make([]widget.Clickable, len(items)), Selected: -1}
	p.search.SingleLine = true
	p.list.Axis = layout.Vertical
	p.Select(selectedKey)
	p.filter()
	return p
}

func (p *pickList) Select(key string) bool {
	for i, it := range p.items {
		if it.Key == key {
			p.Selected = i
			p.scrollTo = true
			return true
		}
	}
	return false
}

func (p *pickList) SelectedItem() (pickItem, bool) {
	if p.Selected < 0 {
		return pickItem{}, false
	}
	return p.items[p.Selected], true
}

func (p *pickList) filter() {
	q := strings.ToLower(strings.TrimSpace(p.query))
	p.shown = p.shown[:0]
	for i, it := range p.items {
		if q == "" || strings.Contains(strings.ToLower(it.Title+" "+it.Sub+" "+it.Search+" "+it.Key), q) {
			p.shown = append(p.shown, i)
		}
	}
}

// Layout draws the search field and list; it reports whether the selection changed.
func (p *pickList) Layout(th *material.Theme, gtx layout.Context, hint string) (layout.Dimensions, bool) {
	changed := false
	for _, i := range p.shown {
		if p.clicks[i].Clicked(gtx) && p.Selected != i {
			p.Selected, changed = i, true
		}
	}
	if q := p.search.Text(); q != p.query {
		p.query = q
		p.filter()
		p.list.Position.First, p.list.Position.Offset = 0, 0
	}
	if p.scrollTo {
		p.scrollTo = false
		for n, i := range p.shown {
			if i == p.Selected {
				p.list.Position.First = max(n-3, 0)
				p.list.Position.Offset = 0
			}
		}
	}
	dims := layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return border(gtx, colBorder, 8, func(gtx layout.Context) layout.Dimensions {
				return background(gtx, colCard, 8, func(gtx layout.Context) layout.Dimensions {
					return layout.Inset{Top: 8, Bottom: 8, Left: 10, Right: 12}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
							layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icSearch, colDim, 18) }),
							hspace(8),
							layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
								e := material.Editor(th, &p.search, hint)
								e.HintColor = rgb(0x6f6f6f)
								return e.Layout(gtx)
							}),
						)
					})
				})
			})
		}),
		vspace(10),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return border(gtx, colBorder, 8, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = gtx.Constraints.Max
				if len(p.shown) == 0 {
					return layout.Center.Layout(gtx, label(th, 14, "Nothing matches “"+p.query+"”", colDim, font.Normal).Layout)
				}
				l := material.List(th, &p.list)
				l.AnchorStrategy = material.Overlay
				return l.Layout(gtx, len(p.shown), func(gtx layout.Context, n int) layout.Dimensions {
					i := p.shown[n]
					return p.row(th, gtx, i)
				})
			})
		}),
	)
	return dims, changed
}

func (p *pickList) row(th *material.Theme, gtx layout.Context, i int) layout.Dimensions {
	it := p.items[i]
	return p.clicks[i].Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		bg := colBg
		switch {
		case i == p.Selected:
			bg = colSelected
		case p.clicks[i].Hovered():
			bg = colCard
		}
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return background(gtx, bg, 0, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X = gtx.Constraints.Max.X
			return layout.Inset{Top: 10, Bottom: 10, Left: 14, Right: 14}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						w := font.Normal
						if i == p.Selected {
							w = font.SemiBold
						}
						return label(th, 14, it.Title, colText, w).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return label(th, 13, it.Sub, colDim, font.Normal).Layout(gtx)
					}),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if i != p.Selected {
							return layout.Dimensions{}
						}
						return layout.Inset{Left: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return icon(gtx, icCheck, colAccent, 18)
						})
					}),
				)
			})
		})
	})
}
