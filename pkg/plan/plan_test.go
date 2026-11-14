package plan

import (
	"strings"
	"testing"
)

func good() Plan {
	return Plan{Disk: "/dev/vdb", Hostname: "zbox", Language: "ur_PK.UTF-8", Keyboard: "us", Timezone: "Asia/Karachi",
		User: User{FullName: "Ali Zain", Name: "ali", Password: "secret12", Admin: true}}
}

func TestPasswordFor(t *testing.T) {
	if why := PasswordProblemFor("tulip-7-cloud", "ali", "Ali Zain"); why != "" {
		t.Errorf("good password rejected: %s", why)
	}
	if PasswordProblemFor("abcde", "x", "") == "" {
		t.Error("5 characters accepted")
	}
	if PasswordProblemFor("SaraKhan99", "sara", "Sara Khan") == "" {
		t.Error("name in password accepted")
	}
}

func TestValidate(t *testing.T) {
	if err := (&Plan{}).Validate(); err == nil {
		t.Fatal("empty plan accepted")
	}
	p := good()
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*Plan){
		"live user":   func(p *Plan) { p.User.Name = "zainium" },
		"root":        func(p *Plan) { p.User.Name = "root" },
		"upper":       func(p *Plan) { p.User.Name = "Ali" },
		"short pw":    func(p *Plan) { p.User.Password = "abc" },
		"pw has user": func(p *Plan) { p.User.Password = "xxAli123" },
		"pw has name": func(p *Plan) { p.User.Password = "zain2026!" },
		"root has me": func(p *Plan) { p.RootPassword = "ALI-root-1" },
		"root root":   func(p *Plan) { p.RootPassword = "myroot99" },
		"colon":       func(p *Plan) { p.User.FullName = "a:b" },
		"host":        func(p *Plan) { p.Hostname = "-x" },
		"zone":        func(p *Plan) { p.Timezone = "Asia/Jerusalem" },
		"zone alias":  func(p *Plan) { p.Timezone = "Israel" },
		"locale":      func(p *Plan) { p.Language = "he_IL.UTF-8" },
		"layout":      func(p *Plan) { p.Keyboard = "il" },
		"path escape": func(p *Plan) { p.Timezone = "../../etc/shadow" },
	} {
		q := good()
		mutate(&q)
		if err := q.Validate(); err == nil {
			t.Errorf("%s accepted", name)
		} else if strings.Contains(err.Error(), "disk") {
			t.Errorf("%s: %v", name, err)
		}
	}
}
