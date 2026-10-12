// Package ui is the graphical installer: Anaconda-style hub and spokes on Gio (Wayland).
package ui

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"gioui.org/app"
	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	"github.com/Zainium-Dynamics/zainium-installer/assets"
	"github.com/Zainium-Dynamics/zainium-installer/internal/catalog"
	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
	"github.com/Zainium-Dynamics/zainium-installer/internal/target"
)

const (
	syshubShare = "/overlayer/syshub/share"
	productName = "ZAINIUM OS INSTALLATION"
)

type page int

const (
	pageWelcome page = iota
	pageHub
	pageKeyboard
	pageLanguage
	pageTime
	pageDisk
	pageHost
	pageRoot
	pageUser
	pageInstall
	pageDone
)

type App struct {
	w    *app.Window
	th   *material.Theme
	page page

	logo, logoSmall paint.ImageOp

	welcomeLangs *pickList
	langs        *pickList
	keyboards    *pickList
	zones        *pickList
	keyboardSet  bool // the user picked a layout; language changes no longer override it
	zoneSet      bool
	primaryZone  map[string]string // country code → its main zone

	disks      []disk.Disk
	diskClicks []widget.Clickable
	diskSel    int
	erase      widget.Bool
	rescan     widget.Clickable

	kbTest     widget.Editor
	hostname   widget.Editor
	deviceName string

	rootMode           widget.Enum
	rootPw, rootPw2    widget.Editor
	fullName, userName widget.Editor
	userPw, userPw2    widget.Editor
	admin              widget.Bool
	userNameTouched    bool
	lastFullName       string
	lastAutoUser       string

	tiles []*tile

	continueBtn, quitBtn, beginBtn, doneBtn, restartBtn, detailsBtn widget.Clickable
	showDetails                                                     bool
	hubList, spokeList, logList                                     widget.List

	inst *installer
	deco widget.Decorations
}

func Run(w *app.Window) error {
	a := newApp(w)
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.DestroyEvent:
			return e.Err
		case app.ConfigEvent:
			a.deco.Maximized = e.Config.Mode == app.Maximized
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			a.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func decodePNG(b []byte) paint.ImageOp {
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return paint.NewImageOp(image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	}
	return paint.NewImageOp(img)
}

func newApp(w *app.Window) *App {
	a := &App{w: w, th: newTheme(), diskSel: -1}
	a.logo = decodePNG(assets.Logo)
	a.logoSmall = decodePNG(assets.LogoSmall)
	for _, l := range []*widget.List{&a.hubList, &a.spokeList, &a.logList} {
		l.Axis = layout.Vertical
	}

	var langItems []pickItem
	for _, l := range catalog.Languages {
		sub := ""
		if renderable(l.Native) && l.Native != l.Name {
			sub = l.Native
		}
		langItems = append(langItems, pickItem{Title: l.Name, Sub: sub, Key: l.Locale, Search: l.Native})
	}
	a.welcomeLangs = newPickList(langItems, "en_US.UTF-8")
	a.langs = newPickList(langItems, "en_US.UTF-8")

	var kbItems []pickItem
	if ls, err := catalog.Layouts(syshubShare + "/xkeyboard-config-2/rules/evdev.lst"); err == nil {
		for _, l := range ls {
			key := l.ID
			if l.Variant != "" {
				key += "+" + l.Variant
			}
			kbItems = append(kbItems, pickItem{Title: l.Name, Sub: key, Key: key})
		}
	} else {
		kbItems = append(kbItems, pickItem{Title: "English (US)", Sub: "us", Key: "us"})
	}
	a.keyboards = newPickList(kbItems, "us")

	var zoneItems []pickItem
	a.primaryZone = map[string]string{}
	zoneinfo := syshubShare + "/zoneinfo"
	if zs, err := catalog.Zones(zoneinfo); err == nil {
		for _, z := range zs {
			zoneItems = append(zoneItems, pickItem{Title: z.Title(), Sub: z.Name, Key: z.Name, Search: z.Country})
			if z.Primary {
				a.primaryZone[z.Country] = z.Name
			}
		}
	} else {
		zoneItems = append(zoneItems, pickItem{Title: "Coordinated Universal Time — UTC", Sub: "UTC", Key: "UTC"})
	}
	a.zones = newPickList(zoneItems, "UTC")
	if os.Getenv("ZONEINFO") == "" {
		os.Setenv("ZONEINFO", zoneinfo)
	}

	a.deviceName = catalog.DeviceName()
	a.hostname.SingleLine = true
	a.kbTest.SingleLine = true
	a.hostname.SetText(a.deviceName)
	for _, e := range []*widget.Editor{&a.rootPw, &a.rootPw2, &a.userPw, &a.userPw2} {
		e.SingleLine = true
		e.Mask = '•'
	}
	a.fullName.SingleLine = true
	a.userName.SingleLine = true
	a.rootMode.Value = "disabled"
	a.admin.Value = true
	a.scanDisks()
	a.applyLanguage("en_US.UTF-8")
	a.tiles = a.newTiles()
	return a
}

// renderable is true for scripts Zainium's fonts (Noto Sans) cover.
func renderable(s string) bool {
	for _, r := range s {
		if r > 0x52f && !unicode.IsSpace(r) && !unicode.IsPunct(r) {
			return false
		}
	}
	return true
}

func (a *App) scanDisks() {
	ds, _ := disk.List()
	a.disks = ds
	a.diskClicks = make([]widget.Clickable, len(ds))
	a.diskSel = -1
	a.erase.Value = false
	var usable []int
	for i, d := range ds {
		if ok, _ := d.Usable(); ok {
			usable = append(usable, i)
		}
	}
	if len(usable) == 1 {
		a.diskSel = usable[0]
	}
}

// applyLanguage also moves the keyboard and time zone defaults until the user picks them.
func (a *App) applyLanguage(locale string) {
	a.langs.Select(locale)
	a.welcomeLangs.Select(locale)
	if !a.keyboardSet {
		if !a.keyboards.Select(catalog.LayoutForLocale(locale)) {
			a.keyboards.Select("us")
		}
		a.applyLiveKeyboard()
	}
	if !a.zoneSet {
		base, _, _ := strings.Cut(locale, ".")
		base, _, _ = strings.Cut(base, "@")
		if _, cc, ok := strings.Cut(base, "_"); ok && a.primaryZone[cc] != "" {
			a.zones.Select(a.primaryZone[cc])
		}
	}
}

// Plan collects the choices for the engine.
func (a *App) Plan() plan.Plan {
	p := plan.Plan{
		Hostname: strings.TrimSpace(a.hostname.Text()),
		User: plan.User{
			FullName: strings.TrimSpace(a.fullName.Text()),
			Name:     strings.TrimSpace(a.userName.Text()),
			Password: a.userPw.Text(),
			Admin:    a.admin.Value,
		},
		BIOS: !isUEFI(),
	}
	if it, ok := a.langs.SelectedItem(); ok {
		p.Language = it.Key
	}
	p.Keyboard, p.KeyboardVariant = a.keyboardLayout()
	if it, ok := a.zones.SelectedItem(); ok {
		p.Timezone = it.Key
	}
	if a.diskSel >= 0 {
		p.Disk = a.disks[a.diskSel].Path
	}
	if a.rootMode.Value == "enabled" {
		p.RootPassword = a.rootPw.Text()
	}
	return p
}

// keyboardLayout is the chosen layout; non-Latin ones get English (US) as a second layout.
func (a *App) keyboardLayout() (layout, variant string) {
	it, ok := a.keyboards.SelectedItem()
	if !ok {
		return "us", ""
	}
	layout, variant, _ = strings.Cut(it.Key, "+")
	if catalog.NonLatin(layout) {
		return layout + ",us", variant + ","
	}
	return layout, variant
}

// applyLiveKeyboard switches the live COSMIC session to the chosen layout, so passwords are
// typed here exactly as they will be at the login screen.
func (a *App) applyLiveKeyboard() {
	if !strings.Contains(os.Getenv("XDG_CURRENT_DESKTOP"), "COSMIC") {
		return
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return
	}
	layout, variant := a.keyboardLayout()
	options := "None"
	if strings.Contains(layout, ",") {
		options = fmt.Sprintf("Some(%q)", target.SwitchOption)
	}
	p := filepath.Join(dir, "cosmic/com.system76.CosmicComp/v1")
	if os.MkdirAll(p, 0o755) != nil {
		return
	}
	// replace by rename: cosmic-comp sees one complete file, and a foreign-owned old file does not block us
	tmp := filepath.Join(p, ".xkb_config.installer")
	if os.WriteFile(tmp, []byte(target.XkbRON(layout, variant, options)), 0o644) == nil {
		if os.Rename(tmp, filepath.Join(p, "xkb_config")) != nil {
			os.Remove(tmp)
		}
	}
}

func isUEFI() bool {
	_, err := os.Stat("/sys/firmware/efi")
	return err == nil
}

// state summarises everything the screens show, to notice changes made during a frame.
func (a *App) state() string {
	return fmt.Sprintf("%d|%+v|%v|%s", a.page, a.Plan(), a.erase.Value, a.rootMode.Value)
}

func (a *App) layout(gtx layout.Context) {
	before := a.state()
	defer func() {
		// bars and tiles drawn before a click was handled would show the old value; redraw once
		if a.state() != before {
			gtx.Execute(op.InvalidateCmd{})
		}
	}()
	paint.Fill(gtx.Ops, colBg)
	layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(a.titleBar),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			a.content(gtx)
			return layout.Dimensions{Size: gtx.Constraints.Max}
		}),
	)
}

func (a *App) content(gtx layout.Context) {
	switch a.page {
	case pageWelcome:
		a.layoutWelcome(gtx)
	case pageHub:
		a.layoutHub(gtx)
	case pageInstall, pageDone:
		a.layoutInstall(gtx)
	default:
		a.layoutSpoke(gtx)
	}
}

// header is the top bar: page title on the left, product name and keyboard layout on the right.
func (a *App) header(gtx layout.Context, title string) layout.Dimensions {
	gtx.Constraints.Min.X = gtx.Constraints.Max.X
	return background(gtx, colHeader, 0, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X = gtx.Constraints.Max.X
		return layout.Inset{Top: 14, Bottom: 14, Left: 40, Right: 40}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
				layout.Flexed(1, label(a.th, 15, title, colText, font.Bold).Layout),
				layout.Rigid(label(a.th, 12, productName, colDim, font.SemiBold).Layout),
				hspace(14),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					kb := a.Plan().Keyboard
					return background(gtx, colCard, 6, func(gtx layout.Context) layout.Dimensions {
						return layout.Inset{Top: 4, Bottom: 4, Left: 8, Right: 10}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions { return icon(gtx, icKeyboard, colDim, 16) }),
								hspace(6),
								layout.Rigid(label(a.th, 12, kb, colText, font.Medium).Layout),
							)
						})
					})
				}),
			)
		})
	})
}

// image draws an embedded image scaled to fit a square of the given size.
func drawImage(gtx layout.Context, img paint.ImageOp, size unit.Dp) layout.Dimensions {
	s := gtx.Dp(size)
	gtx.Constraints = layout.Exact(image.Pt(s, s))
	return widget.Image{Src: img, Fit: widget.Contain, Position: layout.Center}.Layout(gtx)
}

func (a *App) layoutWelcome(gtx layout.Context) {
	if a.continueBtn.Clicked(gtx) {
		if it, ok := a.welcomeLangs.SelectedItem(); ok {
			a.applyLanguage(it.Key)
		}
		a.page = pageHub
		a.w.Invalidate()
	}
	if a.quitBtn.Clicked(gtx) {
		os.Exit(0)
	}
	layout.Flex{}.Layout(gtx,
		// left: branding
		layout.Flexed(0.42, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			return background(gtx, colHeader, 0, func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min = gtx.Constraints.Max
				return layout.Center.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical, Alignment: layout.Middle}.Layout(gtx,
						layout.Rigid(func(gtx layout.Context) layout.Dimensions { return drawImage(gtx, a.logo, 190) }),
						vspace(28),
						layout.Rigid(label(a.th, 26, "Welcome to Zainium OS", colText, font.Bold).Layout),
						vspace(8),
						layout.Rigid(label(a.th, 14, "Secure. Immutable. Yours.", colDim, font.Normal).Layout),
					)
				})
			})
		}),
		// right: language
		layout.Flexed(0.58, func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min = gtx.Constraints.Max
			return layout.UniformInset(32).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(a.th, 13, productName, colDim, font.SemiBold).Layout),
					vspace(18),
					layout.Rigid(label(a.th, 20, "What language would you like to use?", colText, font.SemiBold).Layout),
					vspace(6),
					layout.Rigid(label(a.th, 14, "This is the language of your installed system. You can add more later in Settings.", colDim, font.Normal).Layout),
					vspace(18),
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						d, _ := a.welcomeLangs.Layout(a.th, gtx, "Type here to search")
						return d
					}),
					vspace(20),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{}.Layout(gtx,
							spacer(),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								return button(a.th, gtx, &a.quitBtn, "Quit", false, true)
							}),
							hspace(12),
							layout.Rigid(func(gtx layout.Context) layout.Dimensions {
								_, ok := a.welcomeLangs.SelectedItem()
								return button(a.th, gtx, &a.continueBtn, "Continue", true, ok)
							}),
						)
					}),
				)
			})
		}),
	)
}

// clock shows the time in the chosen zone on the Time & Date spoke.
func zoneTime(name string) string {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return ""
	}
	return time.Now().In(loc).Format("Monday 2 January 2006, 15:04")
}
