package boot

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
)

func TestConfigFromStaging(t *testing.T) {
	tmpl, err := os.ReadFile("/home/alizain/os/zairoot/overlayer/zaisys/limine/limine.install.conf")
	if err != nil {
		t.Skip("no staging tree")
	}
	conf, err := Config(tmpl, "d5198031-6720-4d08-aae8-0915abdede4d", "ext4")
	if err != nil {
		t.Fatal(err)
	}
	s := string(conf)
	if strings.Contains(s, "${") || !strings.Contains(s, "root=UUID=d5198031-6720-4d08-aae8-0915abdede4d rootfstype=ext4") {
		t.Fatalf("config:\n%s", s)
	}
	for _, p := range []string{"boot():/overlayer/zaisys/kernel/linux-evl", "boot():/overlayer/zaisys/kernel/quantra-ramfs.img"} {
		if !strings.Contains(s, p) {
			t.Errorf("missing %s", p)
		}
		dst := strings.TrimPrefix(p, "boot():/")
		found := false
		for _, v := range espFiles {
			found = found || v == dst
		}
		if !found {
			t.Errorf("%s is referenced by limine.conf but not copied to the ESP", dst)
		}
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
