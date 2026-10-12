// Package boot puts Limine, its config and the kernels on the ESP.
package boot

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Files copied from the installed zaisys onto the ESP (Limine reads only FAT).
var espFiles = map[string]string{
	"limine/BOOTX64.EFI":       "EFI/BOOT/BOOTX64.EFI",
	"limine/limine-bios.sys":   "limine-bios.sys",
	"kernel/linux-evl":         "overlayer/zaisys/kernel/linux-evl",
	"kernel/linux-panic":       "overlayer/zaisys/kernel/linux-panic",
	"kernel/quantra-ramfs.img": "overlayer/zaisys/kernel/quantra-ramfs.img",
}

// Config fills limine.install.conf's ${ROOT_UUID} and ${ROOT_FSTYPE}.
func Config(template []byte, rootUUID, fstype string) ([]byte, error) {
	s := string(template)
	if !strings.Contains(s, "${ROOT_UUID}") {
		return nil, fmt.Errorf("limine.install.conf has no ${ROOT_UUID}")
	}
	s = strings.ReplaceAll(s, "${ROOT_UUID}", rootUUID)
	s = strings.ReplaceAll(s, "${ROOT_FSTYPE}", fstype)
	return []byte(s), nil
}

// InstallESP copies Limine and the kernels from zaisys to the mounted ESP and writes limine.conf.
func InstallESP(zaisys, esp, rootUUID, fstype string, log func(string)) error {
	for src, dst := range espFiles {
		from, to := filepath.Join(zaisys, src), filepath.Join(esp, dst)
		if src == "limine/limine-bios.sys" {
			if _, err := os.Stat(from); os.IsNotExist(err) {
				continue // UEFI-only builds
			}
		}
		if err := copyFile(from, to); err != nil {
			return err
		}
		log("ESP: " + dst)
	}
	tmpl, err := os.ReadFile(filepath.Join(zaisys, "limine/limine.install.conf"))
	if err != nil {
		return err
	}
	conf, err := Config(tmpl, rootUUID, fstype)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(esp, "limine.conf"), conf, 0o644); err != nil {
		return err
	}
	log("ESP: limine.conf (root=UUID=" + rootUUID + ")")
	return nil
}

func copyFile(from, to string) error {
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return err
	}
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("copy %s: %w", from, err)
	}
	if err := out.Sync(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// Zainium's musl loader; runs binaries whose ELF interpreter points at a missing /lib path.
const muslLoader = "/overlayer/syshub/x86_64-zainium-linux-musl/lib/ld-musl-x86_64.so.1"

// BIOSInstall writes Limine's BIOS stage 1 into the protective MBR and stage 2 into the BIOS boot partition.
func BIOSInstall(diskPath string, biosPart int) (string, error) {
	limine, err := exec.LookPath("limine")
	if err != nil {
		limine = "/overlayer/syshub/bin/limine"
	}
	args := []string{"bios-install", diskPath, strconv.Itoa(biosPart)}
	out, err := exec.Command(limine, args...).CombinedOutput()
	if errors.Is(err, fs.ErrNotExist) {
		if _, lerr := os.Stat(muslLoader); lerr == nil {
			out, err = exec.Command(muslLoader, append([]string{limine}, args...)...).CombinedOutput()
		}
	}
	return strings.TrimSpace(string(out)), err
}
