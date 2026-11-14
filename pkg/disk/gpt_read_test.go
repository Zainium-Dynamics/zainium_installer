package disk

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// A disk with Windows-like partitions and free space at the end keeps them when Zainium is added.
func TestUpdateKeepsOtherSystems(t *testing.T) {
	const ss, sectors = 512, 64 << 20 / 512 * 1024 // 32 GiB image (sparse)
	img := filepath.Join(t.TempDir(), "disk.img")
	f, err := os.Create(img)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := f.Truncate(int64(sectors) * ss); err != nil {
		t.Fatal(err)
	}
	other := &Layout{SectorSize: ss, Sectors: sectors, DiskGUID: RandomGUID(), Parts: []Partition{
		{Type: TypeESP, GUID: RandomGUID(), Name: "EFI system partition", FirstLBA: 2048, LastLBA: 2048 + 204800 - 1},
		{Type: MustGUID("E3C9E316-0B5C-4DB8-817D-F92DF00215AE"), GUID: RandomGUID(), Name: "Microsoft reserved", FirstLBA: 206848, LastLBA: 206848 + 32768 - 1},
		{Type: MustGUID("EBD0A0A2-B9E5-4433-87C0-68B6B72699C7"), GUID: RandomGUID(), Name: "Basic data partition", FirstLBA: 239616, LastLBA: 239616 + 20<<30/512 - 1},
	}}
	if err := other.Write(f); err != nil {
		t.Fatal(err)
	}
	mark := bytes.Repeat([]byte("windows"), 1000)
	winData := int64(other.Parts[2].FirstLBA) * ss
	if _, err := f.WriteAt(mark, winData); err != nil {
		t.Fatal(err)
	}
	mbr := make([]byte, 512)
	f.ReadAt(mbr, 0)

	l, err := ReadLayout(f, ss, sectors)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Parts) != 3 || l.Parts[2].Name != "Basic data partition" || l.DiskGUID != other.DiskGUID {
		t.Fatalf("read back %+v", l.Parts)
	}
	first, last := l.LargestFree()
	if first <= other.Parts[2].LastLBA || first%2048 != 0 || last < first {
		t.Fatalf("free %d-%d", first, last)
	}
	boot := l.AddPart(Partition{Type: TypeESP, GUID: RandomGUID(), Name: "Zainium boot", FirstLBA: first, LastLBA: first + 2048*512 - 1})
	root := l.AddPart(Partition{Type: TypeLinuxRoot, GUID: RandomGUID(), Name: "Zainium OS", FirstLBA: first + 2048*512, LastLBA: last})
	if boot != 4 || root != 5 {
		t.Fatalf("numbers %d %d", boot, root)
	}
	if err := l.Update(f, []int{boot, root}); err != nil {
		t.Fatal(err)
	}

	again, err := ReadLayout(f, ss, sectors)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Parts) != 5 || again.Parts[0] != other.Parts[0] || again.Parts[2] != other.Parts[2] || again.Parts[4].Name != "Zainium OS" {
		t.Fatalf("after update %+v", again.Parts)
	}
	got := make([]byte, len(mark))
	f.ReadAt(got, winData)
	if !bytes.Equal(got, mark) {
		t.Fatal("Windows data changed")
	}
	mbr2 := make([]byte, 512)
	f.ReadAt(mbr2, 0)
	if !bytes.Equal(mbr, mbr2) {
		t.Fatal("MBR changed")
	}
	if a, b := again.LargestFree(); b > a+2048 {
		t.Fatalf("free space left %d-%d", a, b)
	}
}
