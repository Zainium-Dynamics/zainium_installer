// terrarium puts a complete, installed Zainium OS on a USB drive, straight from the ISO: no
// live session, no second stick. The drive then starts any computer (UEFI or BIOS) and keeps
// your files and apps, optionally encrypted.
//
//	terrarium [options] <zainium-live.iso> <usb drive>
//	terrarium --list
//
// It runs as root and uses the installer's own engine, so the result is the same system the
// installer makes. At the end it checks the drive once.
package main

import (
	"bufio"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/engine"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
	"github.com/Zainium-Dynamics/zainium-installer/internal/progress"
)

const work = "/run/terrarium"

var in = bufio.NewReader(os.Stdin)

func main() {
	signal.Ignore(syscall.SIGPIPE, syscall.SIGHUP)
	flag.Usage = func() {
		fmt.Fprint(os.Stderr, "Usage: terrarium [--encrypt] <zainium-live.iso> <usb drive>\n       terrarium --list\n\nPuts an installed Zainium OS on a USB drive. Everything on the drive is erased.\n")
	}
	list := flag.Bool("list", false, "show the USB drives")
	encrypt := flag.Bool("encrypt", false, "encrypt the drive")
	flag.Parse()

	if *list {
		listUSB()
		return
	}
	if flag.NArg() != 2 {
		flag.Usage()
		os.Exit(2)
	}
	iso, dev := flag.Arg(0), flag.Arg(1)
	if os.Geteuid() != 0 {
		fail(errors.New("run it as root (elevate or sudo)"))
	}

	d, err := findUSB(dev)
	if err != nil {
		fail(err)
	}
	if m := mountedParts(d.Name); len(m) > 0 {
		fail(fmt.Errorf("%s is in use (%s mounted): unmount it first", d.Path, strings.Join(m, ", ")))
	}

	p := plan.Plan{
		Disk: d.Path, Hostname: "zainium", Language: "en_US.UTF-8", Keyboard: "us", Timezone: hostTimezone(),
		Portable: true,
	}
	p.User.FullName = ask("Your name", "")
	p.User.Name = ask("User name", "")
	p.User.Password = askSecret("Password")
	p.User.Admin = true
	if *encrypt {
		p.Encrypt = askSecret("Encryption passphrase (asked at every start)")
	}
	if err := p.Validate(); err != nil {
		fail(err)
	}

	fmt.Printf("\n  Drive:  %s, %s\n  Image:  %s\n  User:   %s\n  Encrypt: %v\n\n", d.Title(), disk.HumanSize(d.Size), iso, p.User.Name, *encrypt)
	fmt.Printf("Everything on %s will be erased. Type the drive name (%s) to go on: ", d.Path, d.Name)
	if a, _ := in.ReadString('\n'); strings.TrimSpace(a) != d.Name && strings.TrimSpace(a) != d.Path {
		fail(errors.New("stopped, nothing was changed"))
	}

	src, cleanup, err := openImage(iso)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		fail(err)
	}

	r := progress.NewReporter(&progress.TextWriter{W: os.Stdout})
	e := &engine.Engine{Plan: p, R: r, Source: src}
	if err := e.Run(); err != nil {
		r.Fail(err)
		cleanup()
		os.Exit(1)
	}
	r.Done()

	fmt.Println("\nChecking the drive")
	if err := check(d.Path, src, p.Encrypt); err != nil {
		cleanup()
		fail(fmt.Errorf("check: %w", err))
	}
	fmt.Printf("\nDone. %s now starts Zainium OS on any computer: choose it in the boot menu.\n", d.Path)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "terrarium:", err)
	os.Exit(1)
}

func listUSB() {
	ds, err := disk.List()
	if err != nil {
		fail(err)
	}
	n := 0
	for _, d := range ds {
		if d.USB || d.Removable {
			fmt.Printf("  %-10s %-9s %s\n", d.Path, disk.HumanSize(d.Size), d.Title())
			n++
		}
	}
	if n == 0 {
		fmt.Println("  no USB drive found")
	}
}

// findUSB accepts only a whole USB or removable drive that the installer may erase.
func findUSB(dev string) (disk.Disk, error) {
	ds, err := disk.List()
	if err != nil {
		return disk.Disk{}, err
	}
	for _, d := range ds {
		if d.Path != dev && d.Name != dev {
			continue
		}
		if !d.USB && !d.Removable {
			return d, fmt.Errorf("%s is not a USB drive; terrarium only writes to USB drives (use the installer for internal disks)", d.Path)
		}
		if ok, why := d.Usable(); !ok {
			return d, fmt.Errorf("cannot use %s: %s", d.Path, why)
		}
		return d, nil
	}
	return disk.Disk{}, fmt.Errorf("%s is not a disk on this computer (terrarium --list shows the USB drives)", dev)
}

func mountedParts(name string) []string {
	var m []string
	for _, mi := range disk.Mountinfo() {
		if strings.HasPrefix(filepath.Base(mi.Source), name) {
			m = append(m, mi.Target)
		}
	}
	return m
}

func ask(label, val string) string {
	for val == "" {
		fmt.Printf("%s: ", label)
		s, err := in.ReadString('\n')
		if err != nil && s == "" {
			fail(errors.New("no input"))
		}
		val = strings.TrimSpace(s)
	}
	return val
}

// askSecret reads a secret twice, without echo when stdin is a terminal.
func askSecret(label string) string {
	for {
		a := readHidden(label + ": ")
		b := readHidden("Again: ")
		if a == "" {
			fmt.Println("It can not be empty.")
			continue
		}
		if a != b {
			fmt.Println("They do not match, try again.")
			continue
		}
		return a
	}
}

func readHidden(prompt string) string {
	fmt.Print(prompt)
	fd := int(os.Stdin.Fd())
	old, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err == nil {
		t := *old
		t.Lflag &^= unix.ECHO
		unix.IoctlSetTermios(fd, unix.TCSETS, &t)
		defer func() {
			unix.IoctlSetTermios(fd, unix.TCSETS, old)
			fmt.Println()
		}()
	}
	s, _ := in.ReadString('\n')
	return strings.TrimRight(s, "\r\n")
}

func hostTimezone() string {
	for _, l := range []string{"/etc/localtime", "/overlayer/syshub/etc/localtime"} {
		if t, err := os.Readlink(l); err == nil {
			if i := strings.Index(t, "zoneinfo/"); i >= 0 {
				return t[i+len("zoneinfo/"):]
			}
		}
	}
	return "UTC"
}

// openImage mounts the ISO and the system image inside it, read-only, and returns the tree.
func openImage(iso string) (string, func(), error) {
	isoDir, sysDir := filepath.Join(work, "iso"), filepath.Join(work, "system")
	var loops, mounts []string
	cleanup := func() {
		for i := len(mounts) - 1; i >= 0; i-- {
			unix.Unmount(mounts[i], unix.MNT_DETACH)
		}
		for _, l := range loops {
			exec.Command("losetup", "-d", l).Run()
		}
		os.RemoveAll(work)
		mounts, loops = nil, nil
	}
	loop := func(file string) (string, error) {
		out, err := exec.Command("losetup", "--find", "--show", "--read-only", file).Output()
		if err != nil {
			return "", fmt.Errorf("losetup %s: %w", file, err)
		}
		l := strings.TrimSpace(string(out))
		loops = append(loops, l)
		return l, nil
	}
	if err := os.MkdirAll(isoDir, 0o700); err != nil {
		return "", cleanup, err
	}
	os.MkdirAll(sysDir, 0o700)
	l, err := loop(iso)
	if err != nil {
		return "", cleanup, err
	}
	if err := unix.Mount(l, isoDir, "iso9660", unix.MS_RDONLY, ""); err != nil {
		return "", cleanup, fmt.Errorf("%s is not a Zainium ISO (mount: %w)", iso, err)
	}
	mounts = append(mounts, isoDir)
	img := filepath.Join(isoDir, "zaisys/zairoot.img")
	if _, err := os.Stat(img); err != nil {
		return "", cleanup, fmt.Errorf("%s is not a Zainium live ISO (no zaisys/zairoot.img)", iso)
	}
	l, err = loop(img)
	if err != nil {
		return "", cleanup, err
	}
	if err := unix.Mount(l, sysDir, "erofs", unix.MS_RDONLY, ""); err != nil {
		return "", cleanup, fmt.Errorf("mount the system image (does this kernel have erofs?): %w", err)
	}
	mounts = append(mounts, sysDir)
	return sysDir, cleanup, nil
}

// check mounts the finished drive read-only and compares it with the image: the boot files on
// the EFI partition, the kernel and initramfs, and that every base file was copied.
func check(dev, src, passphrase string) error {
	espDir, rootDir := filepath.Join(work, "check-esp"), filepath.Join(work, "check-root")
	os.MkdirAll(espDir, 0o700)
	os.MkdirAll(rootDir, 0o700)
	exec.Command("udevadm", "settle").Run()

	esp := disk.PartitionPath(dev, disk.PartESP)
	if err := unix.Mount(esp, espDir, "vfat", unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("mount the EFI partition %s: %w", esp, err)
	}
	defer unix.Unmount(espDir, 0)
	for _, f := range []string{"EFI/BOOT/BOOTX64.EFI", "limine.conf"} {
		if _, err := os.Stat(filepath.Join(espDir, f)); err != nil {
			return fmt.Errorf("%s missing on the EFI partition", f)
		}
	}
	ok("UEFI boot files (EFI/BOOT/BOOTX64.EFI, limine.conf)")

	rootPart := disk.PartitionPath(dev, disk.PartRoot)
	if passphrase != "" {
		c := exec.Command("cryptsetup", "open", "--readonly", "--key-file=-", rootPart, "terrarium-check")
		c.Stdin = strings.NewReader(passphrase)
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("open the encrypted root: %v: %s", err, out)
		}
		defer exec.Command("cryptsetup", "close", "terrarium-check").Run()
		rootPart = "/dev/mapper/terrarium-check"
		ok("encrypted root opens with the passphrase")
	}
	if err := unix.Mount(rootPart, rootDir, "ext4", unix.MS_RDONLY, ""); err != nil {
		return fmt.Errorf("mount the root %s: %w", rootPart, err)
	}
	defer unix.Unmount(rootDir, 0)

	for _, f := range []string{"overlayer/zaisys/kernel/linux-zainium", "overlayer/zaisys/kernel/quantra-ramfs.img"} {
		a, err := hashFile(filepath.Join(src, f))
		if err != nil {
			return err
		}
		b, err := hashFile(filepath.Join(rootDir, f))
		if err != nil {
			return err
		}
		if a != b {
			return fmt.Errorf("%s differs from the image", f)
		}
	}
	ok("kernel and initramfs match the image")

	want, got := 0, 0
	for _, top := range []string{"overlayer/syshub", "overlayer/zaisys"} {
		filepath.WalkDir(filepath.Join(src, top), func(p string, de os.DirEntry, err error) error {
			if err != nil || de.IsDir() {
				return nil
			}
			want++
			rel, _ := filepath.Rel(src, p)
			if _, err := os.Lstat(filepath.Join(rootDir, rel)); err == nil {
				got++
			}
			return nil
		})
	}
	if got < want {
		return fmt.Errorf("%d of %d base files are missing", want-got, want)
	}
	ok(fmt.Sprintf("all %d base files are on the drive", want))
	return nil
}

func ok(s string) { fmt.Printf("  ✓ %s\n", s) }

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}
