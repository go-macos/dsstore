package dsstore

import (
	"bytes"
	"encoding/binary"
	"testing"

	"howett.net/plist"
)

// findersAlias digs the alias out of the reference's icvp record, so the
// generated one can be compared with the real thing rather than with itself.
func findersAlias(t *testing.T) []byte {
	t.Helper()
	s, err := Parse(reference(t))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, r := range s.Records() {
		if r.Name != "." || r.ID != "icvp" {
			continue
		}
		blob, ok := r.Val.(Blob)
		if !ok {
			t.Fatalf("icvp is %T, want Blob", r.Val)
		}
		var d map[string]any
		if _, err := plist.Unmarshal(blob, &d); err != nil {
			t.Fatalf("icvp is not a plist: %v", err)
		}
		a, ok := d["backgroundImageAlias"].([]byte)
		if !ok {
			t.Fatalf("icvp has no backgroundImageAlias (keys: %v)", keys(d))
		}
		return a
	}
	t.Fatal("the reference has no icvp record")
	return nil
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// The generated alias must agree with the Finder's, field by field, for the
// fields a synthesised one can know. The dates and catalog ids it cannot know
// are left zero on purpose and are not compared.
func TestAliasMatchesTheFinders(t *testing.T) {
	ref := findersAlias(t)
	got, err := BuildAlias("GODSREF", "/.background/bg.png")
	if err != nil {
		t.Fatalf("BuildAlias: %v", err)
	}
	for _, tc := range []struct {
		what     string
		off, n   int
		wantSame bool
	}{
		{"version", 6, 2, true},
		{"kind", 8, 2, true},
		{"volume name", 10, 28, true},
		{"filesystem type", 42, 2, true},
		{"disk type", 44, 2, true},
		{"target name", 50, 64, true},
		{"levels up/down", 130, 4, true},
	} {
		if tc.off+tc.n > len(ref) || tc.off+tc.n > len(got) {
			t.Fatalf("%s: one of the aliases is too short", tc.what)
		}
		same := bytes.Equal(ref[tc.off:tc.off+tc.n], got[tc.off:tc.off+tc.n])
		if same != tc.wantSame {
			t.Errorf("%s: finder=% x ours=% x", tc.what, ref[tc.off:tc.off+tc.n], got[tc.off:tc.off+tc.n])
		}
	}
	// The tagged section is where the substance is: the two paths that make
	// the reference relocatable, and the counted UTF-16 the published
	// description gets wrong.
	refTags, ourTags := parseTags(t, ref), parseTags(t, got)
	for _, tag := range []int16{tagPOSIXPath, tagMountPoint, tagTargetUnicode, tagVolumeUnicode, tagParentName} {
		r, ok1 := refTags[tag]
		g, ok2 := ourTags[tag]
		if !ok1 {
			t.Errorf("the Finder's alias has no tag %d — the layout assumption is wrong", tag)
			continue
		}
		if !ok2 {
			t.Errorf("our alias is missing tag %d", tag)
			continue
		}
		if !bytes.Equal(r, g) {
			t.Errorf("tag %d: finder=%q ours=%q", tag, r, g)
		}
	}
}

// Tags 14 and 15 carry a uint16 count of code units before the UTF-16BE text.
// mac_alias's documentation says otherwise; the Finder's file says this.
func TestCountedUTF16HasItsCount(t *testing.T) {
	ref := findersAlias(t)
	tags := parseTags(t, ref)
	v, ok := tags[tagVolumeUnicode]
	if !ok || len(v) < 2 {
		t.Fatal("no volume-name tag in the reference")
	}
	n := binary.BigEndian.Uint16(v[:2])
	if int(n)*2+2 != len(v) {
		t.Errorf("tag 15 is %d bytes with a count of %d: the count is not there", len(v), n)
	}
	if got := countedUTF16("GODSREF"); !bytes.Equal(got, v) {
		t.Errorf("countedUTF16 = % x, finder = % x", got, v)
	}
}

func parseTags(t *testing.T, a []byte) map[int16][]byte {
	t.Helper()
	out := map[int16][]byte{}
	p := aliasHeaderLen
	for p+4 <= len(a) {
		tag := int16(binary.BigEndian.Uint16(a[p:]))
		n := int(binary.BigEndian.Uint16(a[p+2:]))
		p += 4
		if tag == tagEnd {
			break
		}
		if p+n > len(a) {
			t.Fatalf("tag %d claims %d bytes past the end", tag, n)
		}
		out[tag] = a[p : p+n]
		p += n + n%2
	}
	return out
}
