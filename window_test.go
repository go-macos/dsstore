package dsstore

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"howett.net/plist"
)

// bwspOf decodes a bwsp blob out of a store.
func bwspOf(t *testing.T, s *Store) map[string]any {
	t.Helper()
	for _, r := range s.Records() {
		if r.ID != "bwsp" {
			continue
		}
		blob, ok := r.Val.(Blob)
		if !ok {
			t.Fatalf("bwsp is a %T, want a Blob", r.Val)
		}
		var d map[string]any
		if _, err := plist.Unmarshal([]byte(blob), &d); err != nil {
			t.Fatalf("decoding bwsp: %v", err)
		}
		return d
	}
	t.Fatal("no bwsp record")
	return nil
}

// The window record we write must be the record the Finder writes. Fed the
// bounds read out of the Finder's own file, ours must decode to the same
// dictionary — every key, every value, and no key of our own invention.
func TestWindowMatchesTheFindersOwnRecord(t *testing.T) {
	ref, err := Parse(reference(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := bwspOf(t, ref)

	bounds, ok := want["WindowBounds"].(string)
	if !ok {
		t.Fatalf("the reference WindowBounds is a %T, want a string", want["WindowBounds"])
	}
	var x, y, w, h int
	if _, err := fmt.Sscanf(bounds, "{{%d, %d}, {%d, %d}}", &x, &y, &w, &h); err != nil {
		t.Fatalf("the reference bounds %q do not parse: %v", bounds, err)
	}

	var s Store
	if err := s.SetWindow(Window{X: x, Y: y, Width: w, Height: h}); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}
	got := bwspOf(t, &s)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("bwsp mismatch\n got %#v\nwant %#v", got, want)
	}
}

// The bounds are the caller's, not the reference's.
func TestWindowBoundsAreFormattedAsTheFinderWritesThem(t *testing.T) {
	var s Store
	if err := s.SetWindow(Window{X: 200, Y: 120, Width: 640, Height: 480}); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}
	if got := bwspOf(t, &s)["WindowBounds"]; got != "{{200, 120}, {640, 480}}" {
		t.Errorf("WindowBounds = %q", got)
	}
}

// A window record has to survive the writer, like every other record.
func TestWindowRoundTrips(t *testing.T) {
	var s Store
	if err := s.SetWindow(Window{X: 1, Y: 2, Width: 3, Height: 4}); err != nil {
		t.Fatalf("SetWindow: %v", err)
	}
	raw, err := s.Bytes()
	if err != nil {
		t.Fatalf("Bytes: %v", err)
	}
	out, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a, b := s.Records(), out.Records()
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("record counts: %d in, %d out", len(a), len(b))
	}
	_, ad := a[0].Val.encode()
	_, bd := b[0].Val.encode()
	if !bytes.Equal(ad, bd) {
		t.Error("the bwsp blob changed in the round trip")
	}
}

// Both records report an encoder failure rather than writing a truncated
// plist. The encoder cannot fail on the dictionaries built here, so the seam
// is what makes the branch reachable.
func TestPlistEncoderFailureIsReported(t *testing.T) {
	saved := encodeBinaryPlist
	encodeBinaryPlist = func(any) ([]byte, error) { return nil, errors.New("no") }
	defer func() { encodeBinaryPlist = saved }()

	var s Store
	if err := s.SetWindow(Window{}); err == nil || !strings.Contains(err.Error(), "bwsp") {
		t.Errorf("SetWindow error = %v, want one naming bwsp", err)
	}
	if err := s.SetIconView(IconView{}); err == nil || !strings.Contains(err.Error(), "icvp") {
		t.Errorf("SetIconView error = %v, want one naming icvp", err)
	}
	if len(s.Records()) != 0 {
		t.Errorf("a failed encode still added %d records", len(s.Records()))
	}
}
