// Package target turns the copied live image into an installed system.
//
// Everything is written into the base (overlayer/syshub, overlayer/zaisys): live-only files are
// removed and machine settings (accounts, host name, locale, keyboard, time zone) are written
// there, so they survive a zexlib reset. `zex syshub -u` never replaces these settings (its
// PROTECTED list), and zexlib/union starts empty for the user's own changes.
package target

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// LiveUser is the account the live ISO logs in with.
const LiveUser = "zainium"

// InstallerFiles are live-only files removed from the installed system (paths relative to
// overlayer/zaisys and overlayer/syshub).
var InstallerFiles = []string{
	"bin/zainium-installer",
	"bin/zainium-install",
	"bin/carve",
	"bin/cosmic-session-start",             // live autologin launcher (HOME in /tmp)
	"etc/elevators/elevate.toml.d/10-live", // live user's elevate without a password
	"share/applications/tech.zainiumdynamics.Installer.desktop",
	"share/icons/hicolor/scalable/apps/tech.zainiumdynamics.Installer.svg",
	"share/polkit-1/actions/tech.zainiumdynamics.installer.policy",
	"etc/xdg/autostart/tech.zainiumdynamics.Installer.desktop",
}

type Target struct {
	// Root is where the installed root partition is mounted.
	Root string
	// Log receives one line per change.
	Log func(string)
}

func (t Target) logf(format string, a ...any) {
	if t.Log != nil {
		t.Log(fmt.Sprintf(format, a...))
	}
}

func (t Target) Syshub(rel string) string { return filepath.Join(t.Root, "overlayer/syshub", rel) }
func (t Target) Zaisys(rel string) string { return filepath.Join(t.Root, "overlayer/zaisys", rel) }
func (t Target) Union(rel string) string {
	return filepath.Join(t.Root, "overlayer/zexlib/union", rel)
}

// RemoveInstaller deletes the installer and other live-only files, then empty parent dirs.
func (t Target) RemoveInstaller() error {
	for _, rel := range InstallerFiles {
		for _, base := range []string{t.Zaisys(""), t.Syshub("")} {
			p := filepath.Join(base, rel)
			if err := os.Remove(p); err == nil {
				t.logf("removed %s", p)
				pruneEmpty(filepath.Dir(p), base)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}

// pruneEmpty removes now-empty directories up to (not including) stop.
func pruneEmpty(dir, stop string) {
	for dir != stop && strings.HasPrefix(dir, stop) {
		if os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}

// StripAutologin removes greetd's [initial_session] (live autologin) from the base config.
func (t Target) StripAutologin() error {
	p := t.Syshub("etc/greetd/config.toml")
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var out []string
	skipping, changed := false, false
	for _, l := range strings.Split(string(b), "\n") {
		s := strings.TrimSpace(l)
		if s == "[initial_session]" {
			skipping, changed = true, true
			continue
		}
		if skipping && strings.HasPrefix(s, "[") {
			skipping = false
		}
		if !skipping {
			out = append(out, l)
		}
	}
	if !changed {
		return nil
	}
	t.logf("removed live autologin from %s", p)
	return writeLike(p, []byte(strings.Join(out, "\n")), p)
}

// writeLike writes data to path keeping the mode and owner of ref (if ref exists), else 0644 root.
func writeLike(path string, data []byte, ref string) error {
	mode, uid, gid := os.FileMode(0o644), 0, 0
	if fi, err := os.Stat(ref); err == nil {
		mode = fi.Mode().Perm()
		uid, gid = owner(fi)
	}
	return writeFile(path, data, mode, uid, gid)
}

// writeFile replaces path atomically with the given mode and owner.
func writeFile(path string, data []byte, mode os.FileMode, uid, gid int) error {
	tmp := path + ".zainium-tmp"
	err := os.WriteFile(tmp, data, 0o600)
	if err == nil {
		err = chown(tmp, uid, gid)
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// chown is skipped when not root (tests); the engine always runs as root.
func chown(path string, uid, gid int) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return os.Lchown(path, uid, gid)
}

// baseDir creates rel's directories inside syshub (root, 0755) when missing.
func (t Target) baseDir(rel string) error {
	return os.MkdirAll(t.Syshub(filepath.Dir(rel)), 0o755)
}

// WriteSetting writes a machine setting at rel (a path inside syshub).
func (t Target) WriteSetting(rel string, data []byte, mode os.FileMode) error {
	if err := t.baseDir(rel); err != nil {
		return err
	}
	t.logf("wrote %s", rel)
	return writeFile(t.Syshub(rel), data, mode, 0, 0)
}

// LinkSetting makes rel inside syshub a symlink.
func (t Target) LinkSetting(rel, target string) error {
	if err := t.baseDir(rel); err != nil {
		return err
	}
	p := t.Syshub(rel)
	os.Remove(p)
	t.logf("linked %s -> %s", rel, target)
	if err := os.Symlink(target, p); err != nil {
		return err
	}
	return chown(p, 0, 0)
}

func (t Target) read(rel string) ([]byte, error) { return os.ReadFile(t.Syshub(rel)) }
