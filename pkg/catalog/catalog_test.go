package catalog

import (
	"strings"
	"testing"
)

const staging = "/home/alizain/os/zairoot/overlayer/syshub/share/"

func TestHostFrom(t *testing.T) {
	for _, c := range []struct{ vendor, product, want string }{
		{"LENOVO", "ThinkPad X1 Carbon Gen 9", "lenovo-thinkpad-x1-carbon-gen-9"},
		{"Dell Inc.", "XPS 13 9310", "dell-xps-13-9310"},
		{"QEMU", "Standard PC (Q35 + ICH9, 2009)", "qemu-standard-pc"},
		{"ASUSTeK COMPUTER INC.", "ROG Strix G15 G513QM", "asus-rog-strix-g15-g513qm"},
		{"HP", "HP EliteBook 840 G8 Notebook PC", "hp-elitebook-840-g8-notebook-pc"},
		{"", "", "zainium"},
	} {
		if got := hostFrom(c.vendor, c.product); got != c.want {
			t.Errorf("%q %q: got %q want %q", c.vendor, c.product, got, c.want)
		}
	}
}

func TestZonesExclude(t *testing.T) {
	zs, err := Zones(staging + "zoneinfo")
	if err != nil {
		t.Skip(err)
	}
	pk := false
	for _, z := range zs {
		if z.Country == "IL" || strings.Contains(z.Name, "Jerusalem") || strings.Contains(z.CountryName, "Israel") {
			t.Fatalf("excluded zone offered: %+v", z)
		}
		pk = pk || z.Name == "Asia/Karachi" && z.CountryName == "Pakistan"
		if z.Country == "US" && z.Primary && z.Name != "America/New_York" {
			t.Errorf("US primary zone %s", z.Name)
		}
	}
	if !pk || len(zs) < 300 {
		t.Fatalf("zones: %d, Karachi %v", len(zs), pk)
	}
}

func TestLayoutsExclude(t *testing.T) {
	ls, err := Layouts(staging + "xkeyboard-config-2/rules/evdev.lst")
	if err != nil {
		t.Skip(err)
	}
	de := false
	for _, l := range ls {
		if l.ID == "il" {
			t.Fatalf("excluded layout offered: %+v", l)
		}
		de = de || l.ID == "de" && l.Variant == "nodeadkeys"
	}
	if !de {
		t.Fatal("de nodeadkeys missing")
	}
}

func TestLanguages(t *testing.T) {
	for _, l := range Languages {
		if !AllowedLocale(l.Locale) {
			t.Errorf("excluded locale listed: %s", l.Locale)
		}
	}
	if AllowedLocale("he_IL.UTF-8") || AllowedLocale("ar_IL.UTF-8") || !AllowedLocale("ur_PK.UTF-8") {
		t.Error("AllowedLocale")
	}
}

func TestEveryLanguageHasLayout(t *testing.T) {
	ls, err := Layouts(staging + "xkeyboard-config-2/rules/evdev.lst")
	if err != nil {
		t.Skip(err)
	}
	have := map[string]bool{}
	for _, l := range ls {
		k := l.ID
		if l.Variant != "" {
			k += "+" + l.Variant
		}
		have[k] = true
	}
	for _, l := range Languages {
		if k := LayoutForLocale(l.Locale); !have[k] {
			t.Errorf("%s → %q is not an xkb layout", l.Locale, k)
		}
	}
}
