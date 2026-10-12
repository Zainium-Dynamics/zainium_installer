// Package plan is what the GUI collects and the engine installs: one JSON document.
package plan

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Zainium-Dynamics/zainium-installer/internal/catalog"
)

type User struct {
	FullName string `json:"full_name"`
	Name     string `json:"name"`
	Password string `json:"password"`
	// Admin puts the user in wheel, so elevate works for them.
	Admin bool `json:"admin"`
}

type Plan struct {
	// Disk is the whole disk to erase, e.g. /dev/nvme0n1.
	Disk     string `json:"disk"`
	Hostname string `json:"hostname"`
	// Language is a locale such as en_US.UTF-8.
	Language string `json:"language"`
	// Keyboard is an xkb layout ("us", "de") or a list ("ara,us"), variants matching it.
	Keyboard        string `json:"keyboard"`
	KeyboardVariant string `json:"keyboard_variant,omitempty"`
	// Timezone is a zoneinfo name such as Asia/Karachi.
	Timezone string `json:"timezone"`
	User     User   `json:"user"`
	// RootPassword empty leaves root locked; admins use elevate.
	RootPassword string `json:"root_password,omitempty"`
	// BIOS also installs Limine's legacy BIOS stage (UEFI is always installed).
	BIOS bool `json:"bios"`
}

var (
	userRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	hostRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?$`)
	tzRe   = regexp.MustCompile(`^[A-Za-z0-9_+-]+(/[A-Za-z0-9_+-]+)*$`)
)

// Names that already belong to system accounts or would confuse tools.
var reserved = map[string]bool{
	"root": true, "bin": true, "daemon": true, "nobody": true, "wheel": true, "admin": true,
	"zainium": true, "cosmic-greeter": true, "greeter": true, "polkitd": true, "messagebus": true,
	"systemd-network": true, "systemd-resolve": true, "systemd-timesync": true, "systemd-journal": true,
}

func ValidUsername(n string) error {
	switch {
	case !userRe.MatchString(n):
		return errors.New("use lowercase letters, digits, - and _, starting with a letter (max 32)")
	case reserved[n]:
		return fmt.Errorf("%q is reserved by the system", n)
	}
	return nil
}

func ValidHostname(h string) error {
	if len(h) > 253 {
		return errors.New("host name is too long")
	}
	for _, label := range strings.Split(h, ".") {
		if !hostRe.MatchString(label) {
			return errors.New("use letters, digits and -, not starting or ending with -")
		}
	}
	return nil
}

// MinPassword is the shortest password accepted.
const MinPassword = 6

// PasswordProblem is empty for an acceptable password, else a short reason.
func PasswordProblem(p string) string {
	switch {
	case len([]rune(p)) < MinPassword:
		return fmt.Sprintf("at least %d characters", MinPassword)
	case strings.ContainsAny(p, ":\n"):
		return "no ':' or line breaks"
	}
	return ""
}

// PasswordProblemFor also rejects a password containing the user name or a part of the full name.
func PasswordProblemFor(p, user, fullName string) string {
	if why := PasswordProblem(p); why != "" {
		return why
	}
	lp := strings.ToLower(p)
	names := append([]string{user}, strings.FieldsFunc(fullName, func(r rune) bool {
		return r == ' ' || r == '-' || r == '.' || r == '_' || r == '\''
	})...)
	for _, n := range names {
		// very short names (2 letters) would reject too many good passwords
		if n = strings.ToLower(strings.TrimSpace(n)); len([]rune(n)) >= 3 && strings.Contains(lp, n) {
			return "must not contain your name"
		}
	}
	return ""
}

func (p *Plan) Validate() error {
	var errs []string
	if !strings.HasPrefix(p.Disk, "/dev/") {
		errs = append(errs, "no installation disk selected")
	}
	if err := ValidHostname(p.Hostname); err != nil {
		errs = append(errs, "host name: "+err.Error())
	}
	if p.Timezone != "" && (!tzRe.MatchString(p.Timezone) || !catalog.AllowedZone(p.Timezone)) {
		errs = append(errs, "invalid time zone")
	}
	if p.Language != "" && !catalog.AllowedLocale(p.Language) {
		errs = append(errs, "language is not available")
	}
	for _, l := range strings.Split(p.Keyboard, ",") {
		if !catalog.AllowedLayout(l) || strings.ContainsAny(l, " \"\n") {
			errs = append(errs, "keyboard layout is not available")
			break
		}
	}
	if err := ValidUsername(p.User.Name); err != nil {
		errs = append(errs, "user name: "+err.Error())
	}
	if why := PasswordProblemFor(p.User.Password, p.User.Name, p.User.FullName); why != "" {
		errs = append(errs, "user password: "+why)
	}
	if strings.ContainsAny(p.User.FullName, ":\n") {
		errs = append(errs, "full name: no ':' or line breaks")
	}
	if p.RootPassword != "" {
		if why := PasswordProblemFor(p.RootPassword, p.User.Name, p.User.FullName+" root"); why != "" {
			errs = append(errs, "root password: "+why)
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}
