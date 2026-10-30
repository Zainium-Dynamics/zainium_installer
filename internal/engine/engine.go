// Package engine runs an install plan: partition, format, copy, boot loader, configure.
package engine

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/Zainium-Dynamics/zainium-installer/internal/boot"
	"github.com/Zainium-Dynamics/zainium-installer/internal/copytree"
	"github.com/Zainium-Dynamics/zainium-installer/internal/disk"
	"github.com/Zainium-Dynamics/zainium-installer/internal/fat"
	"github.com/Zainium-Dynamics/zainium-installer/internal/plan"
	"github.com/Zainium-Dynamics/zainium-installer/internal/progress"
	"github.com/Zainium-Dynamics/zainium-installer/internal/target"
)

const (
	workDir  = "/run/zainium-install"
	espSize  = 1 << 30
	espLabel = "ZAINIUMEFI"
	// RootLabel is what the old installer and docs call the root file system.
	RootLabel = "zainium-root"
	rootFS    = "ext4"
	// headroom the root file system needs beyond the copied base
	headroom = 4 << 30
)

type Engine struct {
	Plan plan.Plan
	R    *progress.Reporter
	// Source is a directory holding the live image tree; empty finds the live erofs.
	Source string
	DryRun bool

	mounts   []string // unmounted in reverse on exit
	disk     disk.Disk
	layout   *disk.Layout
	espDev   string
	rootDev  string
	rootUUID string
	srcBytes int64
}

func (e *Engine) logf(format string, a ...any) { e.R.Logf(fmt.Sprintf(format, a...)) }

// Run installs, always leaving nothing mounted behind.
func (e *Engine) Run() (err error) {
	defer func() {
		if cerr := e.cleanup(); err == nil && cerr != nil {
			err = cerr
		}
	}()
	steps := []struct {
		name    string
		percent float64
		run     func() error
	}{
		{progress.Check, 0, e.check},
		{progress.Partition, 3, e.partition},
		{progress.Format, 5, e.format},
		{progress.Mount, 8, e.mount},
		{progress.Copy, 10, e.copy},
		{progress.Boot, 86, e.boot},
		{progress.Configure, 91, e.configure},
		{progress.Finish, 97, e.finish},
	}
	for _, s := range steps {
		e.R.Step(s.name, s.percent, progress.Titles[s.name])
		if e.DryRun && s.name != progress.Check {
			continue
		}
		if err := s.run(); err != nil {
			return fmt.Errorf("%s: %w", progress.Titles[s.name], err)
		}
	}
	return nil
}

func (e *Engine) check() error {
	if err := e.Plan.Validate(); err != nil {
		return err
	}
	if os.Geteuid() != 0 && !e.DryRun {
		return errors.New("the installer engine must run as root")
	}
	disks, err := disk.List()
	if err != nil {
		return err
	}
	found := false
	for _, d := range disks {
		if d.Path == e.Plan.Disk {
			e.disk, found = d, true
		}
	}
	if !found {
		return fmt.Errorf("%s is not a disk on this machine", e.Plan.Disk)
	}
	if ok, why := e.disk.Usable(); !ok {
		return fmt.Errorf("cannot install to %s: %s", e.disk.Title(), why)
	}
	e.logf("target: %s, %s", e.disk.Title(), disk.HumanSize(e.disk.Size))

	if e.Source == "" {
		if e.Source, err = e.findSource(); err != nil {
			return err
		}
	}
	for _, p := range []string{"overlayer/syshub/etc/passwd", "overlayer/zaisys/limine/limine.install.conf"} {
		if _, err := os.Stat(filepath.Join(e.Source, p)); err != nil {
			return fmt.Errorf("the live image at %s is incomplete: %s missing", e.Source, p)
		}
	}
	if _, err := boot.Kernel(filepath.Join(e.Source, "overlayer/zaisys")); err != nil {
		return fmt.Errorf("the live image at %s has no kernel to boot: %w", e.Source, err)
	}
	e.srcBytes = 0
	for _, dir := range append(overlayerDirs(e.Source), "root", "home/"+target.LiveUser) {
		n, _, err := copytree.Measure(filepath.Join(e.Source, dir), nil)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		e.srcBytes += n
	}
	need := int64(espSize) + 2<<20 + e.srcBytes + headroom
	if e.disk.Size < need {
		return fmt.Errorf("%s is too small: %s needed", e.disk.Title(), disk.HumanSize(need))
	}
	e.logf("source: %s (%s to copy)", e.Source, disk.HumanSize(e.srcBytes))
	return nil
}

// findSource mounts the live erofs read-only: the device behind /overlayer/zaisys, else the image on the medium.
func (e *Engine) findSource() (string, error) {
	src := filepath.Join(workDir, "source")
	if e.DryRun {
		return "/", nil
	}
	if err := os.MkdirAll(src, 0o700); err != nil {
		return "", err
	}
	dev := ""
	for _, m := range disk.Mountinfo() {
		if m.FSType == "erofs" && (m.Target == "/overlayer/zaisys" || m.Target == "/zairoot") {
			dev = m.Source
			break
		}
	}
	if dev == "" {
		img := filepath.Join(disk.LiveMedium, "zaisys/zairoot.img")
		if _, err := os.Stat(img); err != nil {
			return "", errors.New("could not find the live system image (not running from the Zainium live ISO?)")
		}
		out, err := exec.Command("losetup", "--find", "--show", "--read-only", img).Output()
		if err != nil {
			return "", fmt.Errorf("losetup %s: %w", img, err)
		}
		dev = strings.TrimSpace(string(out))
	}
	if err := unix.Mount(dev, src, "erofs", unix.MS_RDONLY, ""); err != nil {
		return "", fmt.Errorf("mount %s: %w", dev, err)
	}
	e.mounts = append(e.mounts, src)
	e.logf("live image %s mounted at %s", dev, src)
	return src, nil
}

func (e *Engine) partition() error {
	if err := e.release(); err != nil {
		return err
	}
	// O_EXCL on a block device fails while any partition is mounted
	f, err := os.OpenFile(e.Plan.Disk, os.O_RDWR|unix.O_EXCL, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w (is it in use?)", e.Plan.Disk, err)
	}
	defer f.Close()
	ss, n, err := disk.Geometry(f)
	if err != nil {
		return err
	}
	if e.layout, err = disk.PlanLayout(ss, n, espSize); err != nil {
		return err
	}
	for i, p := range e.layout.Parts {
		e.logf("partition %d: %s, %s", i+1, p.Name, disk.HumanSize(p.Size(ss)))
	}
	if err := e.layout.Write(f); err != nil {
		return fmt.Errorf("writing partition table: %w", err)
	}
	if err := disk.Reread(f); err != nil {
		return err
	}
	e.espDev = disk.PartitionPath(e.Plan.Disk, disk.PartESP)
	e.rootDev = disk.PartitionPath(e.Plan.Disk, disk.PartRoot)
	exec.Command("udevadm", "settle", "--timeout=10").Run()
	for _, d := range []string{e.espDev, e.rootDev} {
		if err := disk.WaitFor(d, 15*time.Second); err != nil {
			return err
		}
	}
	return nil
}

// release unmounts the target disk's partitions and turns off swap on them.
func (e *Engine) release() error {
	d, err := e.refreshDisk()
	if err != nil {
		return err
	}
	for _, dev := range d.Mounted {
		for _, m := range disk.Mountinfo() {
			if m.Source == dev {
				e.logf("unmounting %s from %s", dev, m.Target)
				if err := unix.Unmount(m.Target, 0); err != nil {
					return fmt.Errorf("%s is in use at %s: %w", dev, m.Target, err)
				}
			}
		}
		swapoff(dev)
	}
	return nil
}

func swapoff(dev string) {
	if p, err := unix.BytePtrFromString(dev); err == nil {
		unix.Syscall(unix.SYS_SWAPOFF, uintptr(unsafe.Pointer(p)), 0, 0)
	}
}

func (e *Engine) refreshDisk() (disk.Disk, error) {
	disks, err := disk.List()
	if err != nil {
		return disk.Disk{}, err
	}
	for _, d := range disks {
		if d.Path == e.Plan.Disk {
			return d, nil
		}
	}
	return disk.Disk{}, fmt.Errorf("%s disappeared", e.Plan.Disk)
}

func (e *Engine) format() error {
	esp := e.layout.Parts[disk.PartESP-1]
	f, err := os.OpenFile(e.espDev, os.O_RDWR|unix.O_EXCL, 0)
	if err != nil {
		return err
	}
	err = fat.Format(f, fat.Options{
		SectorSize:    int(e.layout.SectorSize),
		Size:          esp.Size(e.layout.SectorSize),
		Label:         espLabel,
		HiddenSectors: uint32(esp.FirstLBA),
	})
	if err == nil {
		err = f.Sync()
	}
	f.Close()
	if err != nil {
		return fmt.Errorf("FAT32 on %s: %w", e.espDev, err)
	}
	e.logf("%s: FAT32 %s", e.espDev, espLabel)
	e.R.Percent(6, "")

	e.rootUUID = newUUID()
	// verity: quantra-ramfs turns on fs-verity for every base file, so its content cannot be rewritten
	out, err := exec.Command("mkfs.ext4", "-F", "-q", "-O", "verity", "-L", RootLabel, "-U", e.rootUUID, e.rootDev).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.ext4 %s: %v: %s", e.rootDev, err, strings.TrimSpace(string(out)))
	}
	e.logf("%s: ext4 %s UUID=%s", e.rootDev, RootLabel, e.rootUUID)
	return nil
}

func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func (e *Engine) root() string { return filepath.Join(workDir, "target") }
func (e *Engine) esp() string  { return filepath.Join(workDir, "esp") }

func (e *Engine) mount() error {
	for _, m := range []struct{ dev, dir, fs string }{
		{e.rootDev, e.root(), rootFS},
		{e.espDev, e.esp(), "vfat"},
	} {
		if err := os.MkdirAll(m.dir, 0o700); err != nil {
			return err
		}
		if err := unix.Mount(m.dev, m.dir, m.fs, 0, ""); err != nil {
			return fmt.Errorf("mount %s (%s): %w", m.dev, m.fs, err)
		}
		e.mounts = append(e.mounts, m.dir)
		e.logf("mounted %s at %s", m.dev, m.dir)
	}
	return nil
}

// baseOwner fixes the build host's user (uid 1000+) leaking into the image: the base belongs to root.
func baseOwner(uid, gid uint32) (uint32, uint32) {
	if uid >= 1000 && uid != 65534 {
		uid = 0
	}
	if gid >= 1000 && gid != 65534 {
		gid = 0
	}
	return uid, gid
}

// overlayerDirs lists the live image's overlayer directories (syshub, zaisys, agents, ...); zexlib starts empty.
func overlayerDirs(src string) []string {
	dirs := []string{"overlayer/syshub", "overlayer/zaisys"}
	entries, _ := os.ReadDir(filepath.Join(src, "overlayer"))
	for _, en := range entries {
		if n := en.Name(); en.IsDir() && !strings.HasPrefix(n, ".") && n != "syshub" && n != "zaisys" && n != "zexlib" {
			dirs = append(dirs, "overlayer/"+n)
		}
	}
	return dirs
}

func (e *Engine) copy() error {
	t := e.root()
	for _, d := range []struct {
		path string
		mode os.FileMode
	}{
		{"overlayer", 0o755}, {"overlayer/zexlib", 0o755}, {"overlayer/zexlib/union", 0o755},
		{"overlayer/zexlib/work", 0o700}, {"home", 0o755},
	} {
		p := filepath.Join(t, d.path)
		if err := os.MkdirAll(p, d.mode); err != nil {
			return err
		}
		os.Chmod(p, d.mode)
	}

	var done int64
	last := time.Now()
	report := func(base int64) func(int64) {
		return func(n int64) {
			if time.Since(last) < 300*time.Millisecond {
				return
			}
			last = time.Now()
			pct := 10 + 76*float64(base+n)/float64(max(e.srcBytes, 1))
			e.R.Percent(min(pct, 86), fmt.Sprintf("Copying Zainium OS (%s of %s)", disk.HumanSize(base+n), disk.HumanSize(e.srcBytes)))
		}
	}
	for _, dir := range append(overlayerDirs(e.Source), "root") {
		src := filepath.Join(e.Source, dir)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		n, _, _ := copytree.Measure(src, nil)
		e.logf("copying %s", dir)
		if err := copytree.Copy(src, filepath.Join(t, dir), copytree.Options{Owner: baseOwner, Copied: report(done)}); err != nil {
			return err
		}
		done += n
	}
	return copyOverlayerFiles(e.Source, t)
}

// copyOverlayerFiles copies the files and symlinks that sit beside the overlayer directories
// (README.zainium, architecture.md, zex.md, ...), owned by root like the rest of the base.
func copyOverlayerFiles(src, dst string) error {
	entries, err := os.ReadDir(filepath.Join(src, "overlayer"))
	if err != nil {
		return err
	}
	for _, en := range entries {
		from := filepath.Join(src, "overlayer", en.Name())
		to := filepath.Join(dst, "overlayer", en.Name())
		switch {
		case en.Type()&os.ModeSymlink != 0:
			target, err := os.Readlink(from)
			if err != nil {
				return err
			}
			os.Remove(to)
			if err := os.Symlink(target, to); err != nil {
				return err
			}
		case en.Type().IsRegular():
			info, err := en.Info()
			if err != nil {
				return err
			}
			data, err := os.ReadFile(from)
			if err != nil {
				return err
			}
			if err := os.WriteFile(to, data, info.Mode().Perm()); err != nil {
				return err
			}
			if err := os.Chown(to, 0, 0); err != nil {
				return err
			}
		default:
			continue
		}
	}
	return nil
}

func (e *Engine) boot() error {
	zaisys := filepath.Join(e.root(), "overlayer/zaisys")
	if err := boot.InstallESP(zaisys, e.esp(), e.rootUUID, rootFS, func(s string) { e.R.Logf(s) }); err != nil {
		return err
	}
	if out, err := boot.BIOSInstall(e.Plan.Disk, disk.PartBIOSBoot); err != nil {
		if e.Plan.BIOS || !boot.UEFI() {
			return fmt.Errorf("limine bios-install: %v: %s", err, out)
		}
		e.logf("BIOS boot not installed (UEFI still works): %v %s", err, out)
	} else {
		e.logf("BIOS boot code installed")
	}
	if boot.UEFI() {
		name, err := boot.AddBootEntry("Zainium OS", disk.PartESP, e.layout.Parts[disk.PartESP-1], "/EFI/BOOT/BOOTX64.EFI")
		if err != nil {
			// the firmware still finds the removable-media path EFI/BOOT/BOOTX64.EFI
			e.logf("UEFI boot entry not added: %v", err)
		} else {
			e.logf("UEFI boot entry %s added first in BootOrder", name)
		}
	}
	return nil
}

func (e *Engine) configure() error {
	t := target.Target{Root: e.root(), Log: func(s string) { e.R.Logf(s) }}
	if err := t.RemoveInstaller(); err != nil {
		return err
	}
	if err := t.StripAutologin(); err != nil {
		return err
	}
	groups, err := t.RemoveLiveUser()
	if err != nil {
		return err
	}
	uid, gid, err := t.AddUser(e.Plan.User, e.Plan.RootPassword, groups)
	if err != nil {
		return err
	}
	e.R.Percent(93, "Creating your home folder")
	home := filepath.Join(e.root(), "home", e.Plan.User.Name)
	skel := filepath.Join(e.Source, "home", target.LiveUser)
	owner := func(uint32, uint32) (uint32, uint32) { return uint32(uid), uint32(gid) }
	if _, err := os.Stat(skel); err == nil {
		if err := copytree.Copy(skel, home, copytree.Options{Owner: owner}); err != nil {
			return err
		}
	} else if err := os.Mkdir(home, 0o700); err != nil {
		return err
	}
	if err := os.Chown(home, uid, gid); err != nil {
		return err
	}
	if err := os.Chmod(home, 0o700); err != nil {
		return err
	}
	e.R.Percent(95, "Applying your settings")
	for _, step := range []func() error{
		func() error { return t.SetHostname(e.Plan.Hostname) },
		func() error { return t.SetTimezone(e.Plan.Timezone) },
		func() error { return t.SetLanguage(e.Plan.Language) },
		func() error { return t.SetKeyboard(e.Plan.Keyboard, e.Plan.KeyboardVariant) },
		t.ResetMachineID,
	} {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) finish() error {
	unix.Sync()
	return e.cleanup()
}

func (e *Engine) cleanup() error {
	var first error
	for i := len(e.mounts) - 1; i >= 0; i-- {
		m := e.mounts[i]
		err := unix.Unmount(m, 0)
		for try := 0; err == unix.EBUSY && try < 20; try++ {
			time.Sleep(250 * time.Millisecond)
			err = unix.Unmount(m, 0)
		}
		if err != nil && first == nil {
			first = fmt.Errorf("unmount %s: %w", m, err)
			unix.Unmount(m, unix.MNT_DETACH)
		}
		os.Remove(m)
	}
	e.mounts = nil
	return first
}
