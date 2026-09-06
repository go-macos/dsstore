package dsstore

import (
	"encoding/binary"
	"os"
	"testing"
)

// testdata/finder-macos26.DS_Store was written by the Finder itself on
// macOS 26, for a volume with a background picture and two placed icons.
//
// To regenerate it: create an HFS+ volume, put a PNG at .background/bg.png,
// then drive the Finder over AppleScript —
//
//	tell application "Finder"
//	  set d to disk "GODSREF"
//	  open d
//	  set opts to the icon view options of the container window of d
//	  set icon size of opts to 96
//	  set background picture of opts to file ".background:bg.png" of d
//	  set position of item "MyApp.app" of d to {160, 220}
//	  set position of item "Applications" of d to {440, 220}
//	  update d without necessity
//	end tell
//
// — wait for the flush, then copy the volume's .DS_Store out. The delays
// matter: without them the Finder keeps the settings in memory and the file
// on disk holds only a vSrn record.
func reference(t *testing.T) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/finder-macos26.DS_Store")
	if err != nil {
		t.Fatalf("reading the reference: %v", err)
	}
	return b
}

// The Finder's own file must parse, and its contents must be what was asked
// of the Finder. This is the test that says the format is understood.
func TestParseTheFindersOwnFile(t *testing.T) {
	s, err := Parse(reference(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	byID := map[string]Record{}
	for _, r := range s.Records() {
		byID[r.Name+"/"+r.ID] = r
	}
	for _, want := range []string{"./bwsp", "./icvp", "./vSrn", "MyApp.app/Iloc", "Applications/Iloc"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("the reference has no %s record", want)
		}
	}
	// icvl is NOT among them, though one published account lists it.
	if _, ok := byID["./icvl"]; ok {
		t.Log("note: this reference does carry icvl after all")
	}
	if v, ok := byID["./vSrn"].Val.(Long); !ok || v != 1 {
		t.Errorf("vSrn = %v, want Long(1)", byID["./vSrn"].Val)
	}
	// Iloc is x, y, then six 0xFF bytes and two zeros — and the coordinates
	// are the ones the AppleScript set, which is what makes this a check
	// rather than a restatement.
	for _, tc := range []struct {
		name string
		x, y uint32
	}{{"MyApp.app", 160, 220}, {"Applications", 440, 220}} {
		blob, ok := byID[tc.name+"/Iloc"].Val.(Blob)
		if !ok || len(blob) != 16 {
			t.Errorf("%s Iloc is %T of %d bytes, want a 16-byte Blob", tc.name, byID[tc.name+"/Iloc"].Val, len(blob))
			continue
		}
		if x, y := binary.BigEndian.Uint32(blob[0:4]), binary.BigEndian.Uint32(blob[4:8]); x != tc.x || y != tc.y {
			t.Errorf("%s at (%d,%d), want (%d,%d)", tc.name, x, y, tc.x, tc.y)
		}
		for i, b := range blob[8:14] {
			if b != 0xFF {
				t.Errorf("%s Iloc byte %d = %#x, want 0xFF", tc.name, 8+i, b)
			}
		}
	}
}

// Everything this package writes must read back identically, including the
// records it did not invent — the Finder's own icvp and bwsp blobs.
func TestRoundTripThroughOurWriter(t *testing.T) {
	in, err := Parse(reference(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	raw, err := in.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	out, err := Parse(raw)
	if err != nil {
		t.Fatalf("re-Parse: %v", err)
	}
	a, b := in.Records(), out.Records()
	if len(a) != len(b) {
		t.Fatalf("round trip changed the record count: %d -> %d", len(a), len(b))
	}
	for i := range a {
		if a[i].Name != b[i].Name || a[i].ID != b[i].ID {
			t.Errorf("record %d: %q/%s -> %q/%s", i, a[i].Name, a[i].ID, b[i].Name, b[i].ID)
			continue
		}
		ak, ad := a[i].Val.encode()
		bk, bd := b[i].Val.encode()
		if ak != bk || string(ad) != string(bd) {
			t.Errorf("record %d (%q/%s) changed value", i, a[i].Name, a[i].ID)
		}
	}
}

// The block order is load-bearing and undocumented, so it is asserted rather
// than left to be rediscovered: bookkeeping first, DSDB master second, root
// node third. With the bookkeeping block last the Finder ignores the file.
func TestBookkeepingBlockComesFirst(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
	raw, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	infoOff := binary.BigEndian.Uint32(raw[8:12])
	info := raw[abs(infoOff):]
	n := binary.BigEndian.Uint32(info[0:4])
	if n != 3 {
		t.Fatalf("address table holds %d blocks, want 3", n)
	}
	first := binary.BigEndian.Uint32(info[8:12])
	if first&^0x1F != infoOff {
		t.Errorf("block 0 is at %d, want the bookkeeping block at %d", first&^0x1F, infoOff)
	}
	// …and the reference agrees.
	ref := reference(t)
	refInfoOff := binary.BigEndian.Uint32(ref[8:12])
	refFirst := binary.BigEndian.Uint32(ref[abs(refInfoOff)+8:])
	if refFirst&^0x1F != refInfoOff {
		t.Errorf("the Finder's own block 0 is not its bookkeeping block")
	}
}
