package disk

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGUIDRoundTrip(t *testing.T) {
	s := "C12A7328-F81F-11D2-BA4B-00A0C93EC93B"
	if g := MustGUID(s); g.String() != s {
		t.Fatalf("got %s", g)
	}
	if g := MustGUID(s); g[0] != 0x28 || g[3] != 0xC1 || g[8] != 0xBA {
		t.Fatalf("mixed-endian layout wrong: % x", g[:])
	}
}

func TestPartitionPath(t *testing.T) {
	for in, want := range map[string]string{"/dev/sda": "/dev/sda2", "/dev/nvme0n1": "/dev/nvme0n1p2", "/dev/mmcblk0": "/dev/mmcblk0p2", "/dev/vdb": "/dev/vdb2"} {
		if got := PartitionPath(in, 2); got != want {
			t.Errorf("%s: got %s want %s", in, got, want)
		}
	}
}

// The written table must be accepted by sfdisk (util-linux) when it is installed.
func TestWriteImage(t *testing.T) {
	img := filepath.Join(t.TempDir(), "disk.img")
	f, err := os.Create(img)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(20 << 30); err != nil {
		t.Fatal(err)
	}
	ss, n, err := Geometry(f)
	if err != nil {
		t.Fatal(err)
	}
	l, err := PlanLayout(ss, n, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Write(f); err != nil {
		t.Fatal(err)
	}
	if l.Parts[0].FirstLBA != 2048 || l.Parts[0].Size(ss) != 1<<30 || l.Parts[1].FirstLBA%2048 != 0 || l.Parts[2].FirstLBA != 34 || l.Parts[2].LastLBA != 2047 {
		t.Fatalf("layout: %+v", l.Parts)
	}
	sfdisk, err := exec.LookPath("sfdisk")
	if err != nil {
		t.Skip("no sfdisk")
	}
	out, err := exec.Command(sfdisk, "--verify", img).CombinedOutput()
	if err != nil || strings.Contains(string(out), "corrupt") {
		t.Fatalf("sfdisk --verify: %v\n%s", err, out)
	}
	out, _ = exec.Command(sfdisk, "-d", img).CombinedOutput()
	t.Logf("\n%s", out)
	if !strings.Contains(string(out), strings.ToUpper(l.Parts[1].GUID.String())) || !strings.Contains(string(out), "Zainium OS") || !strings.Contains(string(out), "21686148-6449-6E6F-744E-656564454649") {
		t.Fatalf("sfdisk does not see our partitions:\n%s", out)
	}
}
