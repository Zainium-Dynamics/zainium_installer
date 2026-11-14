// Package fat formats a FAT32 file system (Zainium ships no mkfs.vfat); files are then
// copied in through the kernel's vfat driver.
package fat

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
)

type Options struct {
	// SectorSize must be at least the device's logical block size (512 or 4096).
	SectorSize int
	// Size of the file system in bytes.
	Size int64
	// Label is up to 11 characters, stored upper-case.
	Label string
	// HiddenSectors is the partition's start in sectors (BIOS boot code uses it).
	HiddenSectors uint32
}

const (
	reservedSectors = 32
	numFATs         = 2
	minClusters     = 65525
	maxClusters     = 0x0FFFFFF5
)

// clusterBytes follows Microsoft's FAT32 defaults.
func clusterBytes(size int64) int {
	switch {
	case size <= 8<<30:
		return 4 << 10
	case size <= 16<<30:
		return 8 << 10
	case size <= 32<<30:
		return 16 << 10
	}
	return 32 << 10
}

// Format writes the boot sectors, FSInfo, both FATs and an empty root directory.
func Format(w io.WriterAt, o Options) error {
	bps := o.SectorSize
	if bps == 0 {
		bps = 512
	}
	if bps != 512 && bps != 1024 && bps != 2048 && bps != 4096 {
		return fmt.Errorf("unsupported sector size %d", bps)
	}
	total := uint64(o.Size / int64(bps))
	if total > 0xFFFFFFFF {
		return errors.New("partition too large for FAT32")
	}
	spc := uint64(max(clusterBytes(o.Size)/bps, 1))

	// FAT size per the Microsoft FAT specification (RootDirSectors is 0 on FAT32).
	tmp1 := total - reservedSectors
	tmp2 := (256*spc + numFATs) / 2
	fatSz := (tmp1 + tmp2 - 1) / tmp2
	// that formula assumes 512-byte sectors; make sure every cluster has a FAT entry
	for {
		clusters := (total - reservedSectors - numFATs*fatSz) / spc
		if (clusters+2)*4 <= fatSz*uint64(bps) {
			break
		}
		fatSz++
	}
	dataStart := reservedSectors + numFATs*fatSz
	clusters := (total - dataStart) / spc
	if clusters < minClusters {
		return fmt.Errorf("partition too small for FAT32 (%d clusters)", clusters)
	}
	if clusters > maxClusters {
		return errors.New("partition too large for FAT32")
	}

	label := strings.ToUpper(o.Label)
	if label == "" {
		label = "NO NAME"
	}
	if len(label) > 11 {
		label = label[:11]
	}
	label = fmt.Sprintf("%-11s", label)
	var volID [4]byte
	_, _ = rand.Read(volID[:])

	bs := make([]byte, bps)
	copy(bs[0:3], []byte{0xEB, 0x58, 0x90})
	copy(bs[3:11], "ZAINIUM ")
	le := binary.LittleEndian
	le.PutUint16(bs[11:], uint16(bps))
	bs[13] = byte(spc)
	le.PutUint16(bs[14:], reservedSectors)
	bs[16] = numFATs
	bs[21] = 0xF8
	le.PutUint16(bs[24:], 63)
	le.PutUint16(bs[26:], 255)
	le.PutUint32(bs[28:], o.HiddenSectors)
	le.PutUint32(bs[32:], uint32(total))
	le.PutUint32(bs[36:], uint32(fatSz))
	le.PutUint32(bs[44:], 2) // root directory cluster
	le.PutUint16(bs[48:], 1) // FSInfo sector
	le.PutUint16(bs[50:], 6) // backup boot sector
	bs[64] = 0x80
	bs[66] = 0x29
	copy(bs[67:71], volID[:])
	copy(bs[71:82], label)
	copy(bs[82:90], "FAT32   ")
	// boot code: print a message and halt if someone boots the partition itself
	msg := "This is the Zainium OS EFI partition, not a bootable disk.\r\n"
	code := []byte{0x0E, 0x1F, 0xBE, 0x77, 0x7C, 0xAC, 0x22, 0xC0, 0x74, 0x0B, 0x56, 0xB4, 0x0E, 0xBB, 0x07, 0x00, 0xCD, 0x10, 0x5E, 0xEB, 0xF0, 0xFA, 0xF4, 0xEB, 0xFD}
	copy(bs[90:], code)
	copy(bs[90+len(code):], msg+"\x00")
	bs[510], bs[511] = 0x55, 0xAA

	fsinfo := make([]byte, bps)
	le.PutUint32(fsinfo[0:], 0x41615252)
	le.PutUint32(fsinfo[484:], 0x61417272)
	le.PutUint32(fsinfo[488:], uint32(clusters-1))
	le.PutUint32(fsinfo[492:], 3)
	le.PutUint32(fsinfo[508:], 0xAA550000)

	// zero the reserved area, the FATs and the root cluster
	zero := make([]byte, 1<<20)
	end := int64(dataStart+spc) * int64(bps)
	for off := int64(0); off < end; off += int64(len(zero)) {
		n := min(int64(len(zero)), end-off)
		if _, err := w.WriteAt(zero[:n], off); err != nil {
			return err
		}
	}

	at := func(sector uint64, b []byte) error {
		_, err := w.WriteAt(b, int64(sector)*int64(bps))
		return err
	}
	for _, s := range []struct {
		sector uint64
		data   []byte
	}{{0, bs}, {1, fsinfo}, {6, bs}, {7, fsinfo}} {
		if err := at(s.sector, s.data); err != nil {
			return err
		}
	}

	fat := make([]byte, 12)
	le.PutUint32(fat[0:], 0x0FFFFFF8) // media descriptor
	le.PutUint32(fat[4:], 0x0FFFFFFF) // clean shut-down, no errors
	le.PutUint32(fat[8:], 0x0FFFFFFF) // root directory: one cluster, end of chain
	for i := uint64(0); i < numFATs; i++ {
		if err := at(reservedSectors+i*fatSz, fat); err != nil {
			return err
		}
	}

	root := make([]byte, 32)
	copy(root[0:11], label)
	root[11] = 0x08 // volume label
	return at(dataStart, root)
}
