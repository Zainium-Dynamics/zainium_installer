package target

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Zainium-Dynamics/zainium-installer/internal/crypt"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
)

// table is a colon-separated account file (passwd, shadow, group, gshadow).
type table struct {
	lines [][]string
	// trailing keeps comments and blank lines in place
	raw []string
}

func parseTable(b []byte) *table {
	t := &table{}
	for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		if l == "" || strings.HasPrefix(l, "#") {
			t.lines = append(t.lines, nil)
			t.raw = append(t.raw, l)
			continue
		}
		t.lines = append(t.lines, strings.Split(l, ":"))
		t.raw = append(t.raw, "")
	}
	return t
}

func (t *table) bytes() []byte {
	var sb strings.Builder
	for i, f := range t.lines {
		if f == nil {
			sb.WriteString(t.raw[i])
		} else {
			sb.WriteString(strings.Join(f, ":"))
		}
		sb.WriteByte('\n')
	}
	return []byte(sb.String())
}

func (t *table) find(name string) []string {
	for _, f := range t.lines {
		if f != nil && f[0] == name {
			return f
		}
	}
	return nil
}

func (t *table) remove(name string) {
	for i, f := range t.lines {
		if f != nil && f[0] == name {
			t.lines = slices.Delete(t.lines, i, i+1)
			t.raw = slices.Delete(t.raw, i, i+1)
			return
		}
	}
}

func (t *table) add(f ...string) {
	t.lines = append(t.lines, f)
	t.raw = append(t.raw, "")
}

// members edits the member list of every group (field idx); fn returns the new list.
func (t *table) members(idx int, fn func(group string, m []string) []string) {
	for _, f := range t.lines {
		if f == nil || len(f) <= idx {
			continue
		}
		var m []string
		if f[idx] != "" {
			m = strings.Split(f[idx], ",")
		}
		f[idx] = strings.Join(fn(f[0], m), ",")
	}
}

type accountFile struct {
	rel  string
	mode os.FileMode
	// member field index for group files, -1 otherwise
	members int
}

var accountFiles = []accountFile{
	{"etc/passwd", 0o644, -1},
	{"etc/shadow", 0o600, -1},
	{"etc/group", 0o644, 3},
	{"etc/gshadow", 0o600, 3},
}

// RemoveLiveUser deletes the live account from the base and locks root, so a zexlib reset can
// never bring back the ISO's well-known passwords. It returns the live user's extra groups.
func (t Target) RemoveLiveUser() ([]string, error) {
	var groups []string
	for _, af := range accountFiles {
		p := t.Syshub(af.rel)
		b, err := os.ReadFile(p)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, err
		}
		tb := parseTable(b)
		tb.remove(LiveUser)
		if af.members >= 0 {
			tb.members(af.members, func(g string, m []string) []string {
				if slices.Contains(m, LiveUser) {
					if af.rel == "etc/group" {
						groups = append(groups, g)
					}
					return slices.DeleteFunc(m, func(s string) bool { return s == LiveUser })
				}
				return m
			})
		}
		if af.rel == "etc/shadow" {
			if root := tb.find("root"); root != nil && len(root) > 1 {
				root[1] = "!"
			}
		}
		if err := writeFile(p, tb.bytes(), af.mode, 0, 0); err != nil {
			return nil, err
		}
		t.logf("removed live user from %s", af.rel)
	}
	os.Remove(t.Syshub("var/lib/AccountsService/users/" + LiveUser))
	return groups, nil
}

// AddUser adds the user to the base account files and returns the user's uid and gid.
func (t Target) AddUser(u plan.User, rootPassword string, groups []string) (int, int, error) {
	tabs := map[string]*table{}
	for _, af := range accountFiles {
		b, err := t.read(af.rel)
		if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return 0, 0, err
		}
		tabs[af.rel] = parseTable(b)
	}
	passwd, shadow, group := tabs["etc/passwd"], tabs["etc/shadow"], tabs["etc/group"]
	if passwd == nil || shadow == nil || group == nil {
		return 0, 0, errors.New("passwd, shadow or group missing in the copied system")
	}
	if passwd.find(u.Name) != nil || group.find(u.Name) != nil {
		return 0, 0, fmt.Errorf("an account or group named %q already exists", u.Name)
	}

	uid := freeID(passwd, 2)
	gid := uid
	if group.findID(gid) {
		gid = freeID(group, 2)
	}
	ids := strconv.Itoa(uid)
	gids := strconv.Itoa(gid)
	days := strconv.FormatInt(time.Now().Unix()/86400, 10)

	passwd.add(u.Name, "x", ids, gids, u.FullName, "/home/"+u.Name, "/overlayer/syshub/bin/bash")
	shadow.add(u.Name, crypt.SHA512(u.Password), days, "0", "99999", "7", "", "", "")
	group.add(u.Name, "x", gids, "")
	if gs := tabs["etc/gshadow"]; gs != nil {
		gs.add(u.Name, "!", "", "")
	}

	if !slices.Contains(groups, "wheel") && u.Admin {
		groups = append(groups, "wheel")
	}
	if !u.Admin {
		groups = slices.DeleteFunc(slices.Clone(groups), func(g string) bool { return g == "wheel" })
	}
	for _, rel := range []string{"etc/group", "etc/gshadow"} {
		if tb := tabs[rel]; tb != nil {
			tb.members(3, func(g string, m []string) []string {
				if slices.Contains(groups, g) && !slices.Contains(m, u.Name) {
					return append(m, u.Name)
				}
				return m
			})
		}
	}

	if root := shadow.find("root"); root != nil && len(root) > 1 {
		if rootPassword != "" {
			root[1] = crypt.SHA512(rootPassword)
			if len(root) > 2 {
				root[2] = days
			}
		} else {
			root[1] = "!"
		}
	}

	for _, af := range accountFiles {
		if tb := tabs[af.rel]; tb != nil {
			if err := t.WriteSetting(af.rel, tb.bytes(), af.mode); err != nil {
				return 0, 0, err
			}
		}
	}
	t.logf("user %s (uid %d) in groups %s", u.Name, uid, strings.Join(groups, ","))
	return uid, gid, nil
}

func (t *table) findID(id int) bool {
	s := strconv.Itoa(id)
	for _, f := range t.lines {
		if len(f) > 2 && f[2] == s {
			return true
		}
	}
	return false
}

// freeID returns the first id from 1000 not used in field idx.
func freeID(t *table, idx int) int {
	used := map[int]bool{}
	for _, f := range t.lines {
		if len(f) > idx {
			if n, err := strconv.Atoi(f[idx]); err == nil {
				used[n] = true
			}
		}
	}
	id := 1000
	for used[id] {
		id++
	}
	return id
}
