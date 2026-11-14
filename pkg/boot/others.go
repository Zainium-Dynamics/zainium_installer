package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// OtherSystem is a boot loader of another system on the disk, started from Limine's menu.
type OtherSystem struct {
	Name string
	// PartGUID is the GPT partition GUID of the EFI partition that holds it.
	PartGUID string
	// Path is the loader on that partition, e.g. /EFI/Microsoft/Boot/bootmgfw.efi.
	Path string
}

// Linux loaders in a distribution's EFI directory, in the order to prefer them.
var linuxLoaders = []string{"shimx64.efi", "grubx64.efi", "systemd-bootx64.efi"}

// Known directory names and how to show them.
var names = map[string]string{
	"ubuntu": "Ubuntu", "debian": "Debian", "fedora": "Fedora", "opensuse": "openSUSE",
	"arch": "Arch Linux", "manjaro": "Manjaro", "pop": "Pop!_OS", "linuxmint": "Linux Mint",
	"centos": "CentOS", "redhat": "Red Hat", "elementary": "elementary OS", "systemd": "Linux (systemd-boot)",
}

// FindOtherSystems lists the boot loaders on a mounted EFI partition: Windows Boot Manager
// and one loader for each Linux distribution's EFI directory.
func FindOtherSystems(esp, partGUID string) []OtherSystem {
	var found []OtherSystem
	if isFile(filepath.Join(esp, "EFI/Microsoft/Boot/bootmgfw.efi")) {
		found = append(found, OtherSystem{"Windows", partGUID, "/EFI/Microsoft/Boot/bootmgfw.efi"})
	}
	dirs, _ := os.ReadDir(filepath.Join(esp, "EFI"))
	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Name() < dirs[j].Name() })
	for _, d := range dirs {
		dir := strings.ToLower(d.Name())
		if !d.IsDir() || dir == "boot" || dir == "microsoft" || dir == "zainium" {
			continue
		}
		for _, loader := range linuxLoaders {
			if isFile(filepath.Join(esp, "EFI", d.Name(), loader)) {
				name := names[dir]
				if name == "" {
					name = strings.ToUpper(dir[:1]) + dir[1:]
				}
				found = append(found, OtherSystem{name, partGUID, "/EFI/" + d.Name() + "/" + loader})
				break
			}
		}
	}
	return found
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

// WithOtherSystems adds a menu entry for each other system to limine.conf, and shows the
// menu at every start (quiet: no) so there is a choice to make.
func WithOtherSystems(conf []byte, others []OtherSystem) []byte {
	if len(others) == 0 {
		return conf
	}
	s := strings.Replace(string(conf), "quiet: yes", "quiet: no", 1)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	for _, o := range others {
		s += fmt.Sprintf("\n/%s\n    comment: Start %s\n    protocol: efi\n    path: guid(%s):%s\n",
			o.Name, o.Name, strings.ToLower(o.PartGUID), o.Path)
	}
	return []byte(s)
}
