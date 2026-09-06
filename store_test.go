package dsstore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"

	"howett.net/plist"
)

func TestAddReplacesTheSameRecord(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(2)})
	s.Add(Record{Name: "a", ID: "vSrn", Val: Long(3)})
	recs := s.Records()
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2 — the second vSrn should replace the first", len(recs))
	}
	if v := recs[0].Val.(Long); v != 2 {
		t.Errorf("'.' vSrn = %d, want the later 2", v)
	}
}

// Records sort by name case-insensitively, then by structure id — which is
// what makes the leaf a B-tree node rather than a list.
func TestRecordOrder(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: "beta", ID: "Iloc", Val: Blob{1}})
	s.Add(Record{Name: "Alpha", ID: "Iloc", Val: Blob{2}})
	s.Add(Record{Name: "Alpha", ID: "Bloc", Val: Blob{3}})
	var got []string
	for _, r := range s.Records() {
		got = append(got, r.Name+"/"+r.ID)
	}
	want := []string{"Alpha/Bloc", "Alpha/Iloc", "beta/Iloc"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("order = %v, want %v", got, want)
	}
}

func TestEveryValueTypeRoundTrips(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: ".", ID: "lng1", Val: Long(0xDEADBEEF)})
	s.Add(Record{Name: ".", ID: "bltr", Val: Bool(true)})
	s.Add(Record{Name: ".", ID: "blfl", Val: Bool(false)})
	s.Add(Record{Name: ".", ID: "type", Val: Type("icnv")})
	s.Add(Record{Name: ".", ID: "blob", Val: Blob("hello")})
	raw, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	back, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Value{}
	for _, r := range back.Records() {
		byID[r.ID] = r.Val
	}
	if v, ok := byID["lng1"].(Long); !ok || v != 0xDEADBEEF {
		t.Errorf("lng1 = %v", byID["lng1"])
	}
	if v, ok := byID["bltr"].(Bool); !ok || !bool(v) {
		t.Errorf("bool true = %v", byID["bltr"])
	}
	if v, ok := byID["blfl"].(Bool); !ok || bool(v) {
		t.Errorf("bool false = %v", byID["blfl"])
	}
	if v, ok := byID["type"].(Type); !ok || v != "icnv" {
		t.Errorf("type = %v", byID["type"])
	}
	if v, ok := byID["blob"].(Blob); !ok || string(v) != "hello" {
		t.Errorf("blob = %v", byID["blob"])
	}
}

func TestRecordRefusesWhatItCannotEncode(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: ".", ID: "toolong!", Val: Long(1)})
	if _, err := s.Bytes(); err == nil {
		t.Error("a structure id that is not four characters should be refused")
	}
	s = &Store{}
	s.Add(Record{Name: ".", ID: "vSrn"})
	if _, err := s.Bytes(); err == nil {
		t.Error("a record with no value should be refused")
	}
}

// A store that does not fit one leaf is refused rather than written half
// right: see the package comment.
func TestTooLargeIsRefused(t *testing.T) {
	s := &Store{}
	for i := range 200 {
		s.Add(Record{Name: strings.Repeat("x", 20) + string(rune('a'+i%26)) + string(rune('a'+i/26)), ID: "blob", Val: Blob(make([]byte, 64))})
	}
	if _, err := s.Bytes(); !errors.Is(err, ErrTooLarge) {
		t.Errorf("err = %v, want ErrTooLarge", err)
	}
}

// A name longer than the Pascal field is truncated rather than overflowing it.
func TestLongNamesAreTruncatedNotOverflowed(t *testing.T) {
	long := strings.Repeat("v", 40)
	a, err := BuildAlias(long, "/"+strings.Repeat("t", 80)+".png")
	if err != nil {
		t.Fatal(err)
	}
	if n := a[10]; int(n) != 27 {
		t.Errorf("volume name length byte = %d, want the field's 27", n)
	}
	if n := a[50]; int(n) != 63 {
		t.Errorf("target name length byte = %d, want the field's 63", n)
	}
}

func TestBuildAliasRefusesTheImpossible(t *testing.T) {
	if _, err := BuildAlias("", "/x.png"); err == nil {
		t.Error("an alias with no volume name should be refused")
	}
	if _, err := BuildAlias("V", "relative/x.png"); err == nil {
		t.Error("a path that does not start at the volume root should be refused")
	}
	// A file directly in the root has the volume as its parent.
	a, err := BuildAlias("VOL", "/x.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a, []byte("VOL:VOL:")) {
		t.Errorf("a root-level target should carry VOL:VOL: in its Carbon path")
	}
}

func TestIconViewDefaultsAndBackgroundOff(t *testing.T) {
	s := &Store{}
	if err := s.SetIconView(IconView{}); err != nil { // no background at all
		t.Fatal(err)
	}
	var d map[string]any
	for _, r := range s.Records() {
		if r.ID == "icvp" {
			if _, err := plist.Unmarshal([]byte(r.Val.(Blob)), &d); err != nil {
				t.Fatal(err)
			}
		}
	}
	if d["backgroundType"] != uint64(1) {
		t.Errorf("backgroundType = %v, want 1 when there is no picture", d["backgroundType"])
	}
	if _, ok := d["backgroundImageAlias"]; ok {
		t.Error("no picture, yet an alias was written")
	}
	// The defaults are the ones a disk-image window wants, not zeroes.
	if d["iconSize"] != 96.0 || d["textSize"] != 12.0 || d["gridSpacing"] != 100.0 {
		t.Errorf("defaults = icon %v text %v grid %v", d["iconSize"], d["textSize"], d["gridSpacing"])
	}
}

func TestIconViewRejectsABadBackgroundPath(t *testing.T) {
	s := &Store{}
	if err := s.SetIconView(IconView{Background: "no-leading-slash.png", VolumeName: "V"}); err == nil {
		t.Error("a background path that is not volume-absolute should be refused")
	}
}

func TestSetIconPosition(t *testing.T) {
	s := &Store{}
	s.SetIconPosition("MyApp.app", 160, 220)
	for _, r := range s.Records() {
		if r.ID != "Iloc" {
			continue
		}
		b := []byte(r.Val.(Blob))
		if len(b) != 16 {
			t.Fatalf("Iloc is %d bytes, want 16", len(b))
		}
		if x, y := binary.BigEndian.Uint32(b), binary.BigEndian.Uint32(b[4:]); x != 160 || y != 220 {
			t.Errorf("position = %d,%d", x, y)
		}
		for i, v := range b[8:14] {
			if v != 0xFF {
				t.Errorf("byte %d = %#x, want 0xFF", 8+i, v)
			}
		}
		return
	}
	t.Error("no Iloc record was added")
}
