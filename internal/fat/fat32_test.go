package fat

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Checks the result with dosfstools and mtools when they are installed.
func TestFormatImage(t *testing.T) {
	for _, tc := range []struct {
		size int64
		bps  int
	}{{1 << 30, 512}, {1 << 30, 4096}, {600 << 20, 512}} {
		img := filepath.Join(t.TempDir(), "esp.img")
		f, err := os.Create(img)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Truncate(tc.size); err != nil {
			t.Fatal(err)
		}
		if err := Format(f, Options{SectorSize: tc.bps, Size: tc.size, Label: "zainiumefi", HiddenSectors: 2048}); err != nil {
			t.Fatal(err)
		}
		f.Close()

		if fsck, err := exec.LookPath("fsck.fat"); err == nil {
			out, err := exec.Command(fsck, "-n", "-v", img).CombinedOutput()
			if err != nil {
				t.Fatalf("fsck.fat (%d/%d): %v\n%s", tc.size, tc.bps, err, out)
			}
			t.Logf("%d/%d\n%s", tc.size, tc.bps, out)
		}
		if tc.bps != 512 {
			continue // mtools only handles 512-byte sectors
		}
		if mcopy, err := exec.LookPath("mcopy"); err == nil {
			src := filepath.Join(t.TempDir(), "limine.conf")
			os.WriteFile(src, []byte("timeout: 5\n"), 0o644)
			if out, err := exec.Command("mmd", "-i", img, "::/EFI", "::/EFI/BOOT").CombinedOutput(); err != nil {
				t.Fatalf("mmd: %v %s", err, out)
			}
			if out, err := exec.Command(mcopy, "-i", img, src, "::/EFI/BOOT/limine.conf").CombinedOutput(); err != nil {
				t.Fatalf("mcopy: %v %s", err, out)
			}
			out, _ := exec.Command("mdir", "-i", img, "::/").CombinedOutput()
			if !strings.Contains(string(out), "ZAINIUMEFI") {
				t.Fatalf("label missing:\n%s", out)
			}
			if out, err := exec.Command("fsck.fat", "-n", img).CombinedOutput(); err != nil {
				t.Fatalf("fsck after copy: %v\n%s", err, out)
			}
		}
	}
}
