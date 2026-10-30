package boot

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
)

func TestConfigFromStaging(t *testing.T) {
	tmpl, err := os.ReadFile("/home/alizain/os/zairoot/overlayer/zaisys/limine/limine.install.conf")
	if err != nil {
		t.Skip("no staging tree")
	}
	conf, err := Config(tmpl, "d5198031-6720-4d08-aae8-0915abdede4d", "ext4", "some-kernel")
	if err != nil {
		t.Fatal(err)
	}
	s := string(conf)
	if strings.Contains(s, "${") || !strings.Contains(s, "root=UUID=d5198031-6720-4d08-aae8-0915abdede4d rootfstype=ext4") {
		t.Fatalf("config:\n%s", s)
	}
	for _, p := range []string{"boot():/overlayer/zaisys/kernel/some-kernel", "boot():/overlayer/zaisys/kernel/quantra-ramfs.img"} {
		if !strings.Contains(s, p) {
			t.Errorf("missing %s", p)
		}
	}
}

func TestKernelIsFoundByWhatIsThere(t *testing.T) {
	z := t.TempDir()
	if err := os.MkdirAll(filepath.Join(z, "kernel"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"linux-anything", FallbackKernel, "quantra-ramfs.img", "quantra-ramfs-panic.img"} {
		if err := os.WriteFile(filepath.Join(z, "kernel", f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	k, err := Kernel(z)
	if err != nil || k != "linux-anything" {
		t.Fatalf("Kernel() = %q, %v", k, err)
	}
	if err := os.WriteFile(filepath.Join(z, "kernel", "linux-other"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Kernel(z); err == nil {
		t.Fatal("two kernels: want an error")
	}
}

func TestLoadOption(t *testing.T) {
	p := disk.Partition{GUID: disk.MustGUID("43079136-4627-4032-947C-6280F35C91D9"), FirstLBA: 2048, LastLBA: 2048 + 2097152 - 1}
	opt := loadOption("Zainium OS", 1, p, "/EFI/BOOT/BOOTX64.EFI")
	if description(opt) != "Zainium OS" {
		t.Fatalf("description %q", description(opt))
	}
	pathLen := int(binary.LittleEndian.Uint16(opt[4:]))
	paths := opt[len(opt)-pathLen:]
	if paths[0] != 4 || paths[1] != 1 || binary.LittleEndian.Uint64(paths[8:]) != 2048 || binary.LittleEndian.Uint64(paths[16:]) != 2097152 {
		t.Fatalf("hard drive node % x", paths[:42])
	}
	if !strings.HasSuffix(string(paths), "\x7f\xff\x04\x00") {
		t.Fatal("no end node")
	}
}
