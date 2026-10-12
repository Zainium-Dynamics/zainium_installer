package target

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zainium-Dynamics/zainium-installer/internal/crypt"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
)

const passwd = `root:x:0:0:root:/root:/overlayer/syshub/bin/bash
zainium:x:1000:1000:Zainium User:/home/zainium:/overlayer/syshub/bin/bash
cosmic-greeter:x:199:199:Cosmic Greeter Account:/:/overlayer/syshub/bin/nologin
`
const shadow = `root:$6$zainiumsalt$abc:20696:0:99999:7:::
zainium:$6$zainiumsalt$abc:20696:0:99999:7:::
cosmic-greeter:!:20696:0:99999:7:::
`
const group = `root:x:0:
zainium:x:1000:
wheel:x:11:zainium
input:x:19:cosmic-greeter,zainium
video:x:26:cosmic-greeter,zainium
`
const greetd = `[terminal]
vt = "1"

[initial_session]
command = "/overlayer/zaisys/bin/cosmic-session-start"
user = "zainium"

[default_session]
command = "/overlayer/syshub/bin/cosmic-greeter-start"
user = "cosmic-greeter"
`

func fakeRoot(t *testing.T) Target {
	root := t.TempDir()
	files := map[string]string{
		"overlayer/syshub/etc/passwd":                                                           passwd,
		"overlayer/syshub/etc/shadow":                                                           shadow,
		"overlayer/syshub/etc/group":                                                            group,
		"overlayer/syshub/etc/greetd/config.toml":                                               greetd,
		"overlayer/syshub/etc/hosts":                                                            "127.0.0.1   localhost\n127.0.1.1   zainium\n::1         localhost zainium\n",
		"overlayer/syshub/share/zoneinfo/Asia/Karachi":                                          "TZif",
		"overlayer/zaisys/bin/zainium-installer":                                                "x",
		"overlayer/zaisys/bin/cosmic-session-start":                                             "x",
		"overlayer/zaisys/share/applications/tech.zainiumdynamics.Installer.desktop":            "x",
		"overlayer/zaisys/share/icons/hicolor/scalable/apps/tech.zainiumdynamics.Installer.svg": "x",
		"overlayer/zaisys/kernel/linux-evl":                                                     "k",
	}
	for p, c := range files {
		os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755)
		if err := os.WriteFile(filepath.Join(root, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	os.MkdirAll(filepath.Join(root, "overlayer/zexlib/union"), 0o755)
	return Target{Root: root, Log: func(s string) { t.Log(s) }}
}

func read(t *testing.T, p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallFlow(t *testing.T) {
	tg := fakeRoot(t)
	if err := tg.RemoveInstaller(); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"bin/zainium-installer", "bin/cosmic-session-start", "share/applications", "share/icons"} {
		if _, err := os.Stat(tg.Zaisys(p)); !os.IsNotExist(err) {
			t.Errorf("%s still there", p)
		}
	}
	if _, err := os.Stat(tg.Zaisys("kernel/linux-evl")); err != nil {
		t.Error("kernel removed")
	}
	if err := tg.StripAutologin(); err != nil {
		t.Fatal(err)
	}
	if g := read(t, tg.Syshub("etc/greetd/config.toml")); strings.Contains(g, "initial_session") || !strings.Contains(g, "[default_session]") {
		t.Errorf("greetd:\n%s", g)
	}

	groups, err := tg.RemoveLiveUser()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(groups, ",") != "wheel,input,video" {
		t.Errorf("groups %v", groups)
	}
	base := read(t, tg.Syshub("etc/shadow"))
	if strings.Contains(base, "zainium") || !strings.HasPrefix(base, "root:!:") {
		t.Errorf("base shadow:\n%s", base)
	}
	defer func() {
		// nothing may land in the union: a zexlib reset must keep the installed settings
		if es, _ := os.ReadDir(tg.Root + "/overlayer/zexlib/union"); len(es) != 0 {
			t.Errorf("union is not empty: %v", es)
		}
	}()
	if fi, _ := os.Stat(tg.Syshub("etc/shadow")); fi.Mode().Perm() != 0o600 {
		t.Errorf("shadow mode %v", fi.Mode())
	}

	uid, gid, err := tg.AddUser(plan.User{Name: "ali", FullName: "Ali Zain", Password: "secret1", Admin: true}, "", groups)
	if err != nil {
		t.Fatal(err)
	}
	if uid != 1000 || gid != 1000 {
		t.Errorf("uid %d gid %d", uid, gid)
	}
	pw := read(t, tg.Syshub("etc/passwd"))
	if !strings.Contains(pw, "ali:x:1000:1000:Ali Zain:/home/ali:/overlayer/syshub/bin/bash") || strings.Contains(pw, "zainium") {
		t.Errorf("passwd:\n%s", pw)
	}
	gr := read(t, tg.Syshub("etc/group"))
	for _, want := range []string{"wheel:x:11:ali", "input:x:19:cosmic-greeter,ali", "ali:x:1000:"} {
		if !strings.Contains(gr, want) {
			t.Errorf("group missing %q:\n%s", want, gr)
		}
	}
	sh := read(t, tg.Syshub("etc/shadow"))
	var hash string
	for _, l := range strings.Split(sh, "\n") {
		if strings.HasPrefix(l, "ali:") {
			hash = strings.Split(l, ":")[1]
		}
	}
	if crypt.SHA512Salt("secret1", hash) != hash || !strings.HasPrefix(sh, "root:!:") {
		t.Errorf("shadow:\n%s", sh)
	}

	if _, _, err := tg.AddUser(plan.User{Name: "ali", Password: "secret1"}, "", nil); err == nil {
		t.Error("duplicate user accepted")
	}

	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(tg.SetHostname("zbox"))
	if h := read(t, tg.Syshub("etc/hosts")); !strings.Contains(h, "127.0.1.1   zbox") || !strings.Contains(h, "::1         localhost zbox") {
		t.Errorf("hosts:\n%s", h)
	}
	must(tg.SetTimezone("Asia/Karachi"))
	if l, _ := os.Readlink(tg.Syshub("etc/localtime")); l != "/overlayer/syshub/share/zoneinfo/Asia/Karachi" {
		t.Errorf("localtime -> %s", l)
	}
	if tg.SetTimezone("Mars/Base") == nil {
		t.Error("bad tz accepted")
	}
	must(tg.SetLanguage("de_DE.UTF-8"))
	must(tg.SetKeyboard("de", "nodeadkeys"))
	if x := read(t, tg.Syshub("share/cosmic/com.system76.CosmicComp/v1/xkb_config")); !strings.Contains(x, `layout: "de"`) || !strings.Contains(x, `variant: "nodeadkeys"`) {
		t.Errorf("xkb:\n%s", x)
	}
}
