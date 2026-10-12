// Package disk finds installable disks and lays out the GPT the installer uses.
package disk

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// LiveMedium is where quantra-ramfs mounts the ISO / USB stick it booted from.
const LiveMedium = "/run/live/medium"

// MinSize is the smallest disk the installer accepts.
const MinSize = 16 << 30

type Disk struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Model     string `json:"model"`
	Vendor    string `json:"vendor"`
	Removable bool   `json:"removable"`
	USB       bool   `json:"usb"`
	ReadOnly  bool   `json:"read_only"`
	// Live is true for the disk Zainium is running from; it can never be a target.
	Live bool `json:"live"`
	// Mounted lists the disk's partitions that are currently mounted.
	Mounted    []string `json:"mounted,omitempty"`
	Partitions []Part   `json:"partitions,omitempty"`
}

type Part struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	Label string `json:"label,omitempty"`
}

// Title is how the GUI names a disk: "Samsung SSD 980 (nvme0n1)".
func (d Disk) Title() string {
	vendor := d.Vendor
	if strings.HasPrefix(vendor, "0x") {
		vendor = "" // a PCI vendor id (virtio), not a name
	}
	name := strings.TrimSpace(vendor + " " + d.Model)
	switch {
	case name != "":
	case strings.HasPrefix(d.Name, "vd"):
		name = "VirtIO Disk"
	case strings.HasPrefix(d.Name, "nvme"):
		name = "NVMe Disk"
	case strings.HasPrefix(d.Name, "mmcblk"):
		name = "SD/eMMC Card"
	default:
		name = "Disk"
	}
	return fmt.Sprintf("%s (%s)", name, d.Name)
}

// Usable says whether the installer may erase this disk, and if not, why.
func (d Disk) Usable() (bool, string) {
	switch {
	case d.Live:
		return false, "Zainium OS is running from this disk"
	case strings.HasPrefix(d.Name, "zd") || strings.HasPrefix(d.Name, "ubd"):
		return false, "not a physical disk"
	case d.ReadOnly:
		return false, "the disk is read-only"
	case d.Size < MinSize:
		return false, fmt.Sprintf("at least %s is needed", HumanSize(MinSize))
	}
	return true, ""
}

// HumanSize formats bytes the way disk vendors do (GB = 10^9), which matches the label on the box.
func HumanSize(b int64) string {
	units := []string{"B", "kB", "MB", "GB", "TB", "PB"}
	f, i := float64(b), 0
	for f >= 1000 && i < len(units)-1 {
		f /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d B", b)
	}
	return fmt.Sprintf("%.1f %s", f, units[i])
}

func readSys(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// uevent reads one KEY= value from a sysfs uevent file.
func uevent(dir, key string) string {
	for _, l := range strings.Split(readSys(dir+"/uevent"), "\n") {
		if v, ok := strings.CutPrefix(l, key+"="); ok {
			return v
		}
	}
	return ""
}

func sysInt(path string) int64 {
	n, _ := strconv.ParseInt(readSys(path), 10, 64)
	return n
}

// skip reports block devices that are never install targets.
func skip(name string) bool {
	for _, p := range []string{"loop", "ram", "zram", "sr", "dm-", "md", "nbd", "fd", "mtdblock"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return strings.HasPrefix(name, "mmcblk") && strings.Contains(name, "boot") || strings.Contains(name, "rpmb")
}

// List returns the whole disks on this machine, largest first.
func List() ([]Disk, error) {
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	mounts := mountedDevices()
	live := liveDisks(mounts)
	var out []Disk
	for _, e := range entries {
		name := e.Name()
		if skip(name) {
			continue
		}
		sys := filepath.Join("/sys/block", name)
		size := sysInt(sys+"/size") * 512
		if size == 0 {
			continue
		}
		link, _ := filepath.EvalSymlinks(sys)
		d := Disk{
			Name:      name,
			Path:      "/dev/" + name,
			Size:      size,
			Model:     readSys(sys + "/device/model"),
			Vendor:    readSys(sys + "/device/vendor"),
			Removable: readSys(sys+"/removable") == "1",
			USB:       strings.Contains(link, "/usb"),
			ReadOnly:  readSys(sys+"/ro") == "1",
			Live:      live[name],
		}
		if d.Vendor == "ATA" {
			d.Vendor = ""
		}
		parts, _ := filepath.Glob(sys + "/" + name + "*")
		for _, p := range parts {
			pn := filepath.Base(p)
			if _, err := os.Stat(p + "/partition"); err != nil {
				continue
			}
			d.Partitions = append(d.Partitions, Part{
				Name:  pn,
				Size:  sysInt(p+"/size") * 512,
				Label: uevent(p, "PARTNAME"),
			})
			if mounts["/dev/"+pn] != "" {
				d.Mounted = append(d.Mounted, "/dev/"+pn)
			}
		}
		if mounts[d.Path] != "" {
			d.Mounted = append(d.Mounted, d.Path)
		}
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out, nil
}

// mountedDevices maps /dev paths to one of their mount points.
func mountedDevices() map[string]string {
	m := map[string]string{}
	for _, mi := range Mountinfo() {
		if strings.HasPrefix(mi.Source, "/dev/") {
			m[mi.Source] = mi.Target
		}
	}
	if f, err := os.Open("/proc/swaps"); err == nil {
		defer f.Close()
		s := bufio.NewScanner(f)
		for s.Scan() {
			if fs := strings.Fields(s.Text()); len(fs) > 0 && strings.HasPrefix(fs[0], "/dev/") {
				m[fs[0]] = "[swap]"
			}
		}
	}
	return m
}

// liveDisks finds the disks the running system lives on: the live medium, or on an installed
// system the disk behind / and /overlayer (loop devices are followed to their backing file).
func liveDisks(mounts map[string]string) map[string]bool {
	live := map[string]bool{}
	for _, mi := range Mountinfo() {
		t := mi.Target
		if t != LiveMedium && t != "/zairoot" && t != "/" && t != "/home" && t != "/root" &&
			t != "/overlayer" && !strings.HasPrefix(t, "/overlayer/") {
			continue
		}
		d := mountDisk(mi)
		if strings.HasPrefix(d, "loop") {
			if back := readSys("/sys/block/" + d + "/loop/backing_file"); back != "" {
				d = diskOfPath(back)
			}
		}
		if d != "" {
			live[d] = true
		}
	}
	return live
}

// diskOfPath finds the disk holding a file, through the longest mount point containing it.
func diskOfPath(path string) string {
	best, majmin := "", ""
	for _, mi := range Mountinfo() {
		if (path == mi.Target || strings.HasPrefix(path, strings.TrimSuffix(mi.Target, "/")+"/")) && len(mi.Target) > len(best) {
			best, majmin = mi.Target, mi.MajMin
		}
	}
	for _, mi := range Mountinfo() {
		if mi.Target == best {
			return mountDisk(mi)
		}
	}
	return ParentDisk(majmin)
}

// mountDisk is the disk behind a mount; btrfs and others show a virtual dev number, so the /dev source wins.
func mountDisk(mi MountInfo) string {
	if strings.HasPrefix(mi.Source, "/dev/") {
		var st unix.Stat_t
		if unix.Stat(mi.Source, &st) == nil && st.Mode&unix.S_IFMT == unix.S_IFBLK {
			return ParentDisk(fmt.Sprintf("%d:%d", unix.Major(st.Rdev), unix.Minor(st.Rdev)))
		}
	}
	return ParentDisk(mi.MajMin)
}

// ParentDisk turns a "major:minor" into the name of its whole disk (sda for sda2).
func ParentDisk(majmin string) string {
	p, err := filepath.EvalSymlinks("/sys/dev/block/" + majmin)
	if err != nil {
		return ""
	}
	if _, err := os.Stat(p + "/partition"); err == nil {
		p = filepath.Dir(p)
	}
	return filepath.Base(p)
}

type MountInfo struct {
	MajMin, Root, Target, FSType, Source, Options string
}

// Mountinfo parses /proc/self/mountinfo.
func Mountinfo() []MountInfo {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []MountInfo
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 64<<10), 1<<20)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		sep := -1
		for i, x := range fields {
			if x == "-" {
				sep = i
				break
			}
		}
		if sep < 6 || len(fields) < sep+3 {
			continue
		}
		out = append(out, MountInfo{
			MajMin:  fields[2],
			Root:    unescape(fields[3]),
			Target:  unescape(fields[4]),
			FSType:  fields[sep+1],
			Source:  unescape(fields[sep+2]),
			Options: fields[5],
		})
	}
	return out
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+3 < len(s) {
			if n, err := strconv.ParseUint(s[i+1:i+4], 8, 8); err == nil {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// PartitionPath gives the device of partition n: sda → sda1, nvme0n1 → nvme0n1p1.
func PartitionPath(disk string, n int) string {
	if last := disk[len(disk)-1]; last >= '0' && last <= '9' {
		return fmt.Sprintf("%sp%d", disk, n)
	}
	return fmt.Sprintf("%s%d", disk, n)
}

// WaitFor waits until the kernel has created the device node.
func WaitFor(path string, timeout time.Duration) error {
	end := time.Now().Add(timeout)
	for {
		if fi, err := os.Stat(path); err == nil && fi.Mode()&os.ModeDevice != 0 {
			return nil
		}
		if time.Now().After(end) {
			return fmt.Errorf("%s did not appear", path)
		}
		time.Sleep(100 * time.Millisecond)
	}
}
