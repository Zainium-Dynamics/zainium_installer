package boot

import (
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/unix"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
)

const (
	efivars    = "/sys/firmware/efi/efivars"
	globalGUID = "8be4df61-93ca-11d2-aa0d-00e098032b8c"
	// non-volatile | boot service | runtime access
	varAttrs = 0x7
	// FS_IMMUTABLE_FL from linux/fs.h
	fsImmutableFL = 0x10
)

// UEFI reports whether the machine booted through UEFI.
func UEFI() bool {
	_, err := os.Stat("/sys/firmware/efi")
	return err == nil
}

func utf16z(s string) []byte {
	u := utf16.Encode([]rune(s + "\x00"))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		binary.LittleEndian.PutUint16(b[2*i:], c)
	}
	return b
}

// loadOption builds an EFI_LOAD_OPTION for a file on a GPT partition.
func loadOption(desc string, partNum uint32, p disk.Partition, file string) []byte {
	hd := make([]byte, 42)
	hd[0], hd[1] = 4, 1 // media device path, hard drive
	binary.LittleEndian.PutUint16(hd[2:], 42)
	binary.LittleEndian.PutUint32(hd[4:], partNum)
	binary.LittleEndian.PutUint64(hd[8:], p.FirstLBA)
	binary.LittleEndian.PutUint64(hd[16:], p.LastLBA-p.FirstLBA+1)
	copy(hd[24:40], p.GUID[:])
	hd[40], hd[41] = 2, 2 // GPT, GUID signature

	name := utf16z(strings.ReplaceAll(file, "/", `\`))
	fp := make([]byte, 4+len(name))
	fp[0], fp[1] = 4, 4 // media device path, file path
	binary.LittleEndian.PutUint16(fp[2:], uint16(len(fp)))
	copy(fp[4:], name)

	end := []byte{0x7f, 0xff, 4, 0}
	paths := append(append(hd, fp...), end...)

	out := make([]byte, 6)
	binary.LittleEndian.PutUint32(out[0:], 1) // LOAD_OPTION_ACTIVE
	binary.LittleEndian.PutUint16(out[4:], uint16(len(paths)))
	out = append(out, utf16z(desc)...)
	return append(out, paths...)
}

func varPath(name string) string { return filepath.Join(efivars, name+"-"+globalGUID) }

func readVar(name string) ([]byte, error) {
	b, err := os.ReadFile(varPath(name))
	if err != nil {
		return nil, err
	}
	if len(b) < 4 {
		return nil, fmt.Errorf("%s: short variable", name)
	}
	return b[4:], nil
}

// writeVar replaces a variable; efivarfs files are immutable until the flag is cleared.
func writeVar(name string, data []byte) error {
	p := varPath(name)
	if f, err := os.Open(p); err == nil {
		if flags, err := unix.IoctlGetUint32(int(f.Fd()), unix.FS_IOC_GETFLAGS); err == nil && flags&fsImmutableFL != 0 {
			unix.IoctlSetPointerInt(int(f.Fd()), unix.FS_IOC_SETFLAGS, int(flags&^fsImmutableFL))
		}
		f.Close()
	}
	buf := make([]byte, 4, 4+len(data))
	binary.LittleEndian.PutUint32(buf, varAttrs)
	buf = append(buf, data...)
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	// efivarfs needs the whole variable in a single write
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// mountEfivars mounts efivarfs when the live system has not (writes then reach the firmware).
func mountEfivars() error {
	var st unix.Statfs_t
	if err := unix.Statfs(efivars, &st); err == nil && st.Type == unix.EFIVARFS_MAGIC {
		return nil
	}
	if err := unix.Mount("efivarfs", efivars, "efivarfs", unix.MS_NOSUID|unix.MS_NODEV|unix.MS_NOEXEC, ""); err != nil {
		return fmt.Errorf("mount efivarfs: %w", err)
	}
	return nil
}

// description reads the description of a Boot#### load option.
func description(opt []byte) string {
	if len(opt) < 6 {
		return ""
	}
	var u []uint16
	for i := 6; i+1 < len(opt); i += 2 {
		c := binary.LittleEndian.Uint16(opt[i:])
		if c == 0 {
			break
		}
		u = append(u, c)
	}
	return string(utf16.Decode(u))
}

// AddBootEntry creates (or replaces) the "Zainium OS" entry and puts it first in BootOrder.
func AddBootEntry(desc string, partNum uint32, esp disk.Partition, file string) (string, error) {
	if !UEFI() {
		return "", errors.New("not booted with UEFI")
	}
	if err := mountEfivars(); err != nil {
		return "", err
	}
	entries, err := os.ReadDir(efivars)
	if err != nil {
		return "", err
	}
	used := map[uint16]bool{}
	num := -1
	for _, e := range entries {
		var n uint16
		if _, err := fmt.Sscanf(e.Name(), "Boot%04X-"+globalGUID, &n); err != nil || len(e.Name()) != len("Boot0000-"+globalGUID) {
			continue
		}
		used[n] = true
		if opt, err := readVar(fmt.Sprintf("Boot%04X", n)); err == nil && description(opt) == desc && num < 0 {
			num = int(n) // reinstall: reuse our old entry
		}
	}
	for n := 0; num < 0 && n <= 0xffff; n++ {
		if !used[uint16(n)] {
			num = n
		}
	}
	name := fmt.Sprintf("Boot%04X", num)
	if err := writeVar(name, loadOption(desc, partNum, esp, file)); err != nil {
		return "", fmt.Errorf("writing %s: %w", name, err)
	}

	order := []uint16{uint16(num)}
	if old, err := readVar("BootOrder"); err == nil {
		for i := 0; i+1 < len(old); i += 2 {
			if n := binary.LittleEndian.Uint16(old[i:]); n != uint16(num) {
				order = append(order, n)
			}
		}
	}
	buf := make([]byte, 2*len(order))
	for i, n := range order {
		binary.LittleEndian.PutUint16(buf[2*i:], n)
	}
	if err := writeVar("BootOrder", buf); err != nil {
		return name, fmt.Errorf("writing BootOrder: %w", err)
	}
	return name, nil
}
