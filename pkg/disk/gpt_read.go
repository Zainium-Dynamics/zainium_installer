package disk

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"sort"
	"strings"
	"unicode/utf16"
)

func (g GUID) IsZero() bool { return g == GUID{} }

// ReadLayout reads the disk's GPT, keeping every entry in its slot so the existing
// partitions keep their numbers.
func ReadLayout(f *os.File, sectorSize int64, sectors uint64) (*Layout, error) {
	h := make([]byte, sectorSize)
	if _, err := f.ReadAt(h, sectorSize); err != nil {
		return nil, err
	}
	if string(h[0:8]) != "EFI PART" {
		return nil, errors.New("the disk has no GPT partition table")
	}
	hsize := binary.LittleEndian.Uint32(h[12:])
	if hsize < 92 || int64(hsize) > sectorSize {
		return nil, errors.New("the GPT header is damaged")
	}
	check := append([]byte(nil), h[:hsize]...)
	clear(check[16:20])
	if crc32.ChecksumIEEE(check) != binary.LittleEndian.Uint32(h[16:]) {
		return nil, errors.New("the GPT header is damaged (bad checksum)")
	}
	n, size := binary.LittleEndian.Uint32(h[80:]), binary.LittleEndian.Uint32(h[84:])
	if n != numEntries || size != entrySize {
		return nil, fmt.Errorf("unsupported GPT: %d entries of %d bytes", n, size)
	}
	entries := make([]byte, n*size)
	if _, err := f.ReadAt(entries, int64(binary.LittleEndian.Uint64(h[72:]))*sectorSize); err != nil {
		return nil, err
	}
	if crc32.ChecksumIEEE(entries) != binary.LittleEndian.Uint32(h[88:]) {
		return nil, errors.New("the GPT partition entries are damaged (bad checksum)")
	}
	l := &Layout{SectorSize: sectorSize, Sectors: sectors}
	copy(l.DiskGUID[:], h[56:72])
	for i := range int(n) {
		e := entries[i*entrySize:]
		var p Partition
		copy(p.Type[:], e[0:16])
		copy(p.GUID[:], e[16:32])
		p.FirstLBA = binary.LittleEndian.Uint64(e[32:])
		p.LastLBA = binary.LittleEndian.Uint64(e[40:])
		p.Attributes = binary.LittleEndian.Uint64(e[48:])
		name := make([]uint16, 36)
		for j := range name {
			name[j] = binary.LittleEndian.Uint16(e[56+2*j:])
		}
		p.Name = strings.TrimRight(string(utf16.Decode(name)), "\x00")
		l.Parts = append(l.Parts, p)
	}
	for len(l.Parts) > 0 && l.Parts[len(l.Parts)-1].Type.IsZero() {
		l.Parts = l.Parts[:len(l.Parts)-1]
	}
	return l, nil
}

// LargestFree is the biggest 1 MiB-aligned gap between the partitions (first and last LBA),
// or 0, 0 when there is none.
func (l *Layout) LargestFree() (uint64, uint64) {
	ss := uint64(l.SectorSize)
	align := uint64(mib) / ss
	entrySectors := uint64(numEntries*entrySize) / ss
	firstUsable, lastUsable := 2+entrySectors, l.Sectors-1-entrySectors-1
	var used []Partition
	for _, p := range l.Parts {
		if !p.Type.IsZero() {
			used = append(used, p)
		}
	}
	sort.Slice(used, func(i, j int) bool { return used[i].FirstLBA < used[j].FirstLBA })
	var bestFirst, bestLast uint64
	consider := func(from, to uint64) {
		from = (from + align - 1) / align * align
		to = (to+1)/align*align - 1
		if to > from && to-from > bestLast-bestFirst {
			bestFirst, bestLast = from, to
		}
	}
	next := firstUsable
	for _, p := range used {
		if p.FirstLBA > next {
			consider(next, p.FirstLBA-1)
		}
		next = max(next, p.LastLBA+1)
	}
	if lastUsable >= next {
		consider(next, lastUsable)
	}
	return bestFirst, bestLast
}

// AddPart puts p in the first empty slot and returns its partition number.
func (l *Layout) AddPart(p Partition) int {
	for i := range l.Parts {
		if l.Parts[i].Type.IsZero() {
			l.Parts[i] = p
			return i + 1
		}
	}
	l.Parts = append(l.Parts, p)
	return len(l.Parts)
}
