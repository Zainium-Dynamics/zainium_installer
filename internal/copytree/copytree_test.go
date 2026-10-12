package copytree

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestCopy(t *testing.T) {
	src, dst := t.TempDir(), filepath.Join(t.TempDir(), "out")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(src, "bin"), 0o755))
	must(os.MkdirAll(filepath.Join(src, "skip/me"), 0o755))
	must(os.WriteFile(filepath.Join(src, "bin/tool"), []byte("#!/bin/sh\n"), 0o755))
	must(os.Chmod(filepath.Join(src, "bin/tool"), 0o755|os.ModeSetuid))
	must(os.Link(filepath.Join(src, "bin/tool"), filepath.Join(src, "bin/tool2")))
	must(os.Symlink("tool", filepath.Join(src, "bin/link")))
	must(os.WriteFile(filepath.Join(src, "big"), make([]byte, 20<<20), 0o640))
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	must(os.Chtimes(filepath.Join(src, "big"), old, old))
	must(os.Chtimes(filepath.Join(src, "bin"), old, old))

	bytes, _, err := Measure(src, func(r string) bool { return r == "skip" })
	must(err)
	var last int64
	must(Copy(src, dst, Options{Skip: func(r string) bool { return r == "skip" }, Copied: func(n int64) { last = n }}))
	if last != bytes {
		t.Errorf("progress %d, measured %d", last, bytes)
	}
	fi, err := os.Stat(filepath.Join(dst, "bin/tool"))
	must(err)
	if fi.Mode()&os.ModeSetuid == 0 || fi.Mode().Perm() != 0o755 {
		t.Errorf("mode %v", fi.Mode())
	}
	a, _ := os.Stat(filepath.Join(dst, "bin/tool"))
	b, _ := os.Stat(filepath.Join(dst, "bin/tool2"))
	if a.Sys().(*syscall.Stat_t).Ino != b.Sys().(*syscall.Stat_t).Ino {
		t.Error("hard link not kept")
	}
	if l, _ := os.Readlink(filepath.Join(dst, "bin/link")); l != "tool" {
		t.Errorf("symlink %q", l)
	}
	for _, p := range []string{"big", "bin"} {
		fi, _ := os.Stat(filepath.Join(dst, p))
		if !fi.ModTime().Equal(old) {
			t.Errorf("%s mtime %v", p, fi.ModTime())
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "skip")); !os.IsNotExist(err) {
		t.Error("skip was copied")
	}
}
