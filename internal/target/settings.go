package target

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var liveHostRe = regexp.MustCompile(`(^|\s)` + LiveUser + `(\s|$)`)

// SetHostname writes etc/hostname and points the 127.0.1.1 / ::1 aliases at it.
func (t Target) SetHostname(name string) error {
	if err := t.WriteUnion("etc/hostname", []byte(name+"\n"), 0o644); err != nil {
		return err
	}
	b, err := t.readMerged("etc/hosts")
	if err != nil {
		b = []byte("127.0.0.1   localhost\n::1         localhost\n")
	}
	lines := strings.Split(strings.TrimRight(string(b), "\n"), "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "127.0.1.1") {
			lines[i] = "127.0.1.1   " + name
			found = true
		} else if strings.HasPrefix(l, "::1") {
			lines[i] = liveHostRe.ReplaceAllString(l, "${1}"+name+"${2}")
		}
	}
	if !found {
		lines = append(lines, "127.0.1.1   "+name)
	}
	return t.WriteUnion("etc/hosts", []byte(strings.Join(lines, "\n")+"\n"), 0o644)
}

// SetTimezone links etc/localtime to the zoneinfo file.
func (t Target) SetTimezone(tz string) error {
	if tz == "" {
		tz = "UTC"
	}
	zone := "/overlayer/syshub/share/zoneinfo/" + tz
	if _, err := os.Stat(t.Syshub("share/zoneinfo/" + tz)); err != nil {
		return fmt.Errorf("time zone %q is not in zoneinfo", tz)
	}
	return t.SymlinkUnion("etc/localtime", zone)
}

// SetLanguage writes locale.conf and makes login shells and systemd use it.
func (t Target) SetLanguage(lang string) error {
	if lang == "" {
		lang = "en_US.UTF-8"
	}
	if err := t.WriteUnion("etc/locale.conf", []byte("LANG="+lang+"\n"), 0o644); err != nil {
		return err
	}
	// etc/profile exports en_US.UTF-8 first, then sources profile.d
	if err := t.WriteUnion("etc/profile.d/zainium-locale.sh", []byte("export LANG=\""+lang+"\"\n"), 0o644); err != nil {
		return err
	}
	return t.WriteUnion("etc/systemd/system.conf.d/50-zainium-locale.conf",
		[]byte("[Manager]\nDefaultEnvironment=LANG="+lang+"\n"), 0o644)
}

// SetKeyboard sets the console and COSMIC keyboard layout.
func (t Target) SetKeyboard(layout, variant string) error {
	if layout == "" {
		layout = "us"
	}
	vc := fmt.Sprintf("XKBLAYOUT=%s\nXKBVARIANT=%s\n", layout, variant)
	options := "None"
	if strings.Contains(layout, ",") {
		vc += "XKBOPTIONS=" + SwitchOption + "\n"
		options = fmt.Sprintf("Some(%q)", SwitchOption)
	}
	if _, err := os.Stat(t.Syshub("share/kbd/keymaps")); err == nil {
		first, _, _ := strings.Cut(layout, ",")
		vc = "KEYMAP=" + first + "\n" + vc
	}
	if err := t.WriteUnion("etc/vconsole.conf", []byte(vc), 0o644); err != nil {
		return err
	}
	// system-wide default for cosmic-comp (greeter and new users), cosmic-config RON format
	return t.WriteUnion("share/cosmic/com.system76.CosmicComp/v1/xkb_config", []byte(XkbRON(layout, variant, options)), 0o644)
}

// SwitchOption switches between several keyboard layouts with Alt+Shift.
const SwitchOption = "grp:alt_shift_toggle"

// XkbRON is cosmic-comp's xkb_config in cosmic-config's RON format; options is "None" or Some("...").
func XkbRON(layout, variant, options string) string {
	return fmt.Sprintf("(\n    rules: \"\",\n    model: \"\",\n    layout: %q,\n    variant: %q,\n    options: %s,\n    repeat_delay: 600,\n    repeat_rate: 25,\n)", layout, variant, options)
}

// ResetMachineID leaves an empty machine-id so systemd makes a new one on first boot.
func (t Target) ResetMachineID() error {
	p := t.Syshub("etc/machine-id")
	if _, err := os.Stat(p); err != nil {
		return nil
	}
	return writeLike(p, nil, p)
}
