package disk

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/unix"
)

// GUID is stored in GPT's mixed-endian layout.
type GUID [16]byte

var (
	TypeESP       = MustGUID("C12A7328-F81F-11D2-BA4B-00A0C93EC93B")
	TypeLinuxRoot = MustGUID("4F68BCE3-E8CD-4DB1-96E7-FBCAF984B709") // root x86-64, discoverable partitions spec
	TypeBIOSBoot  = MustGUID("21686148-6449-6E6F-744E-656564454649")
)

// Partition numbers (1-based) in the installer's layout.
const (
	PartESP      = 1
	PartRoot     = 2
	PartBIOSBoot = 3
)

func MustGUID(s string) GUID {
	g, err := ParseGUID(s)
	if err != nil {
		panic(err)
	}
	return g
}

func ParseGUID(s string) (GUID, error) {
	var g GUID
	h := strings.ReplaceAll(s, "-", "")
	if len(h) != 32 {
		return g, fmt.Errorf("bad GUID %q", s)
	}
	var raw [16]byte
	for i := range raw {
		var b byte
		if _, err := fmt.Sscanf(h[2*i:2*i+2], "%02x", &b); err != nil {
			return g, fmt.Errorf("bad GUID %q", s)
		}
		raw[i] = b
	}
	// first three fields are little-endian on disk
	g[0], g[1], g[2], g[3] = raw[3], raw[2], raw[1], raw[0]
	g[4], g[5] = raw[5], raw[4]
	g[6], g[7] = raw[7], raw[6]
	copy(g[8:], raw[8:])
	return g, nil
}

func (g GUID) String() string {
	return fmt.Sprintf("%02X%02X%02X%02X-%02X%02X-%02X%02X-%02X%02X-%02X%02X%02X%02X%02X%02X",
		g[3], g[2], g[1], g[0], g[5], g[4], g[7], g[6], g[8], g[9], g[10], g[11], g[12], g[13], g[14], g[15])
}

// RandomGUID is a version 4 GUID.
func RandomGUID() GUID {
	var g GUID
	if _, err := rand.Read(g[:]); err != nil {
		panic(err)
	}
	g[7] = g[7]&0x0f | 0x40 // version 4 (byte 7 holds the high byte of field 3 on disk)
	g[8] = g[8]&0x3f | 0x80
	return g
}

type Partition struct {
	Type       GUID
	GUID       GUID
	Name       string
	FirstLBA   uint64
	LastLBA    uint64
	Attributes uint64
}

func (p Partition) Size(sector int64) int64 { return int64(p.LastLBA-p.FirstLBA+1) * sector }

// Layout is what Partition writes: ESP and root (1 MiB aligned), plus Limine's BIOS boot
// partition in the gap before the ESP.
type Layout struct {
	SectorSize int64
	Sectors    uint64
	DiskGUID   GUID
	Parts      []Partition
}

const (
	numEntries = 128
	entrySize  = 128
	mib        = 1 << 20
)

// Plan computes the ESP + root layout for a disk of the given geometry.
func PlanLayout(sectorSize int64, sectors uint64, espBytes int64) (*Layout, error) {
	ss := uint64(sectorSize)
	align := uint64(mib) / ss
	entrySectors := uint64(numEntries*entrySize) / ss
	lastUsable := sectors - 1 - entrySectors - 1
	espFirst := align
	espLast := espFirst + uint64(espBytes)/ss - 1
	rootFirst := (espLast/align + 1) * align
	rootLast := (lastUsable+1)/align*align - 1
	if rootLast <= rootFirst || rootFirst <= espLast {
		return nil, errors.New("disk is too small")
	}
	return &Layout{
		SectorSize: sectorSize,
		Sectors:    sectors,
		DiskGUID:   RandomGUID(),
		Parts: []Partition{
			{Type: TypeESP, GUID: RandomGUID(), Name: "EFI System Partition", FirstLBA: espFirst, LastLBA: espLast},
			{Type: TypeLinuxRoot, GUID: RandomGUID(), Name: "Zainium OS", FirstLBA: rootFirst, LastLBA: rootLast},
			{Type: TypeBIOSBoot, GUID: RandomGUID(), Name: "BIOS boot", FirstLBA: 2 + entrySectors, LastLBA: espFirst - 1},
		},
	}, nil
}

// Geometry reads the logical sector size and sector count of an open block device or image.
func Geometry(f *os.File) (int64, uint64, error) {
	fi, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	if fi.Mode()&os.ModeDevice == 0 {
		return 512, uint64(fi.Size() / 512), nil
	}
	ss, err := unix.IoctlGetInt(int(f.Fd()), unix.BLKSSZGET)
	if err != nil {
		return 0, 0, fmt.Errorf("BLKSSZGET: %w", err)
	}
	var size uint64
	if _, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKGETSIZE64, uintptr(unsafePointer(&size))); e != 0 {
		return 0, 0, fmt.Errorf("BLKGETSIZE64: %w", e)
	}
	return int64(ss), size / uint64(ss), nil
}

// Write erases the old partition tables and writes a protective MBR and both GPT copies.
func (l *Layout) Write(f *os.File) error {
	ss := l.SectorSize
	total := int64(l.Sectors) * ss
	// Old GPT/MBR, filesystem signatures in the first MiB, and the backup GPT at the end.
	zero := make([]byte, mib)
	if _, err := f.WriteAt(zero, 0); err != nil {
		return err
	}
	if _, err := f.WriteAt(zero, total-mib); err != nil {
		return err
	}
	for _, p := range l.Parts {
		if _, err := f.WriteAt(zero, int64(p.FirstLBA)*ss); err != nil {
			return err
		}
	}

	entries := make([]byte, numEntries*entrySize)
	for i, p := range l.Parts {
		e := entries[i*entrySize:]
		copy(e[0:16], p.Type[:])
		copy(e[16:32], p.GUID[:])
		binary.LittleEndian.PutUint64(e[32:], p.FirstLBA)
		binary.LittleEndian.PutUint64(e[40:], p.LastLBA)
		binary.LittleEndian.PutUint64(e[48:], p.Attributes)
		for j, c := range utf16.Encode([]rune(p.Name)) {
			if j >= 36 {
				break
			}
			binary.LittleEndian.PutUint16(e[56+2*j:], c)
		}
	}
	entriesCRC := crc32.ChecksumIEEE(entries)
	entrySectors := uint64(len(entries)) / uint64(ss)
	last := l.Sectors - 1

	header := func(self, alt, entriesLBA uint64) []byte {
		h := make([]byte, ss)
		copy(h[0:8], "EFI PART")
		binary.LittleEndian.PutUint32(h[8:], 0x00010000)
		binary.LittleEndian.PutUint32(h[12:], 92)
		binary.LittleEndian.PutUint64(h[24:], self)
		binary.LittleEndian.PutUint64(h[32:], alt)
		binary.LittleEndian.PutUint64(h[40:], 2+entrySectors)
		binary.LittleEndian.PutUint64(h[48:], last-entrySectors-1)
		copy(h[56:72], l.DiskGUID[:])
		binary.LittleEndian.PutUint64(h[72:], entriesLBA)
		binary.LittleEndian.PutUint32(h[80:], numEntries)
		binary.LittleEndian.PutUint32(h[84:], entrySize)
		binary.LittleEndian.PutUint32(h[88:], entriesCRC)
		binary.LittleEndian.PutUint32(h[16:], crc32.ChecksumIEEE(h[:92]))
		return h
	}

	mbr := make([]byte, ss)
	pe := mbr[446:]
	pe[1], pe[2], pe[3] = 0x00, 0x02, 0x00 // CHS of LBA 1
	pe[4] = 0xEE
	pe[5], pe[6], pe[7] = 0xFF, 0xFF, 0xFF
	binary.LittleEndian.PutUint32(pe[8:], 1)
	binary.LittleEndian.PutUint32(pe[12:], uint32(min(l.Sectors-1, 0xFFFFFFFF)))
	mbr[510], mbr[511] = 0x55, 0xAA

	writes := []struct {
		lba  uint64
		data []byte
	}{
		{0, mbr},
		{2, entries},
		{1, header(1, last, 2)},
		{last - entrySectors, entries},
		{last, header(last, 1, last-entrySectors)},
	}
	for _, w := range writes {
		if _, err := f.WriteAt(w.data, int64(w.lba)*ss); err != nil {
			return err
		}
	}
	return f.Sync()
}

// Reread asks the kernel to load the new partition table.
func Reread(f *os.File) error {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeDevice == 0 {
		return err
	}
	var last error
	for range 20 {
		if _, _, e := unix.Syscall(unix.SYS_IOCTL, f.Fd(), unix.BLKRRPART, 0); e == 0 {
			return nil
		} else {
			last = e
		}
		sleep100ms()
	}
	return fmt.Errorf("re-reading the partition table: %w (is something on the disk still in use?)", last)
}
