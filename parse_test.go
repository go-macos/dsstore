package dsstore

import (
	"encoding/binary"
	"strings"
	"testing"

	"howett.net/plist"
)

// A parser's error branches are the half that decides what happens to a
// damaged file, so they are tested rather than assumed.
func TestParseRejectsDamagedFiles(t *testing.T) {
	good := func() []byte {
		s := &Store{}
		s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
		b, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	for _, tc := range []struct {
		name string
		want string
		make func() []byte
	}{
		{"too short", "too short", func() []byte { return []byte("Bud1") }},
		{"bad magic", "bad magic", func() []byte {
			b := good()
			copy(b[4:8], "Nope")
			return b
		}},
		{"the two offsets disagree", "disagree", func() []byte {
			b := good()
			binary.BigEndian.PutUint32(b[16:20], 999)
			return b
		}},
		{"bookkeeping past the end", "bookkeeping block", func() []byte {
			b := good()
			binary.BigEndian.PutUint32(b[8:12], uint32(len(b)))
			binary.BigEndian.PutUint32(b[16:20], uint32(len(b)))
			return b
		}},
		{"address table larger than its block", "address table", func() []byte {
			b := good()
			off := binary.BigEndian.Uint32(b[8:12])
			binary.BigEndian.PutUint32(b[abs(off):], 1<<20)
			return b
		}},
		{"no DSDB entry", "no DSDB", func() []byte {
			b := good()
			off := binary.BigEndian.Uint32(b[8:12])
			info := b[abs(off):]
			n := int(binary.BigEndian.Uint32(info))
			p := 8 + 4*((n+255)/256*256)
			binary.BigEndian.PutUint32(info[p:], 0) // zero TOC entries
			return b
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.make())
			if err == nil {
				t.Fatalf("expected a refusal mentioning %q", tc.want)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// An unknown value type is refused rather than skipped: its length is
// unknown, so every record after it would be read from the wrong offset.
func TestParseRefusesAnUnknownValueType(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
	b, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// The node lives in block 2; find it and corrupt the type tag in place.
	off := binary.BigEndian.Uint32(b[8:12])
	info := b[abs(off):]
	nodeAddr := binary.BigEndian.Uint32(info[8+4*nodeBlock:])
	node := b[abs(nodeAddr&^0x1F):]
	i := indexOf(node[:64], []byte("vSrnlong"))
	if i < 0 {
		t.Fatal("could not find the record to corrupt")
	}
	copy(node[i+4:i+8], "zzzz")
	if _, err := Parse(b); err == nil || !strings.Contains(err.Error(), "unsupported value type") {
		t.Errorf("err = %v, want a refusal of the unknown type", err)
	}
}

func indexOf(h, n []byte) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if string(h[i:i+len(n)]) == string(n) {
			return i
		}
	}
	return -1
}

// The exhaustion branch, reachable only because the width is a parameter.
func TestAllocatorExhaustion(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("a full allocator should panic rather than hand out a block twice")
		}
	}()
	a := newAllocatorOfWidth(blockMin) // one 32-byte block, and nothing else
	a.alloc(1 << blockMin)
	a.alloc(1 << blockMin) // no space left
}

// The fixed part of an alias is exactly 150 bytes. It is an invariant of the
// writer rather than of any input, so it is asserted here instead of guarded
// in code that nothing could ever make fail.
func TestAliasHeaderIsFixedLength(t *testing.T) {
	a, err := BuildAlias("V", "/x.png")
	if err != nil {
		t.Fatal(err)
	}
	// tag 0 is the first thing after the header, and it is two bytes of zero
	// followed by its length.
	if len(a) <= aliasHeaderLen {
		t.Fatalf("alias is %d bytes, shorter than its own header", len(a))
	}
	if tag := binary.BigEndian.Uint16(a[aliasHeaderLen:]); tag != tagParentName {
		t.Errorf("the tagged section starts with tag %d at offset %d, want %d", tag, aliasHeaderLen, tagParentName)
	}
}

// Every truncation guard in the parser, driven by cutting a good file short
// at the offsets that matter.
func TestParseRefusesTruncation(t *testing.T) {
	s := &Store{}
	s.Add(Record{Name: "MyApp.app", ID: "Iloc", Val: Blob(make([]byte, 16))})
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
	s.Add(Record{Name: ".", ID: "flag", Val: Bool(true)})
	s.Add(Record{Name: ".", ID: "kind", Val: Type("icnv")})
	full, err := s.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// Cutting anywhere inside the file must produce an error, never a panic
	// and never a half-read store.
	for n := 1; n < len(full); n += 37 {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("truncating to %d bytes panicked: %v", n, r)
				}
			}()
			if _, err := Parse(full[:n]); err == nil {
				t.Errorf("truncating to %d bytes was accepted", n)
			}
		}()
	}
}

// A file whose tree has an internal node must still read: this package writes
// one leaf, but it should not choke on a file the Finder grew.
func TestParseWalksAnInternalNode(t *testing.T) {
	leafA := buildLeaf(t, Record{Name: "a", ID: "vSrn", Val: Long(1)})
	leafB := buildLeaf(t, Record{Name: "z", ID: "vSrn", Val: Long(2)})
	mid := Record{Name: "m", ID: "vSrn", Val: Long(3)}
	midEnc, err := encodeRecord(mid)
	if err != nil {
		t.Fatal(err)
	}
	// internal node: P = the right-hand child, one (childBlock, record) pair
	internal := make([]byte, pageSize)
	binary.BigEndian.PutUint32(internal[0:4], 4)  // P -> block 4 (leafB)
	binary.BigEndian.PutUint32(internal[4:8], 1)  // one entry
	binary.BigEndian.PutUint32(internal[8:12], 3) // child -> block 3 (leafA)
	copy(internal[12:], midEnc)

	master := make([]byte, 20)
	binary.BigEndian.PutUint32(master[0:4], 2) // root is block 2
	binary.BigEndian.PutUint32(master[4:8], 1) // one level
	binary.BigEndian.PutUint32(master[8:12], 3)
	binary.BigEndian.PutUint32(master[12:16], 3)
	binary.BigEndian.PutUint32(master[16:20], pageSize)

	a := newAllocator()
	mOff, mW := a.alloc(len(master))
	iOff, iW := a.alloc(len(internal))
	aOff, aW := a.alloc(len(leafA))
	bOff, bW := a.alloc(len(leafB))
	addrs := []uint32{0, mOff | uint32(mW), iOff | uint32(iW), aOff | uint32(aW), bOff | uint32(bW)}
	trial := encodeBookkeeping(addrs, a)
	infoOff, infoW := a.alloc(len(trial))
	addrs[0] = infoOff | uint32(infoW)
	info := encodeBookkeeping(addrs, a)

	out := make([]byte, int(a.allocatedEnd())+4)
	binary.BigEndian.PutUint32(out[0:4], 1)
	copy(out[4:8], "Bud1")
	binary.BigEndian.PutUint32(out[8:12], infoOff)
	binary.BigEndian.PutUint32(out[12:16], uint32(1)<<uint(infoW))
	binary.BigEndian.PutUint32(out[16:20], infoOff)
	copy(out[abs(mOff):], master)
	copy(out[abs(iOff):], internal)
	copy(out[abs(aOff):], leafA)
	copy(out[abs(bOff):], leafB)
	copy(out[abs(infoOff):], info)

	got, err := Parse(out)
	if err != nil {
		t.Fatalf("Parse of a two-level tree: %v", err)
	}
	var names []string
	for _, r := range got.Records() {
		names = append(names, r.Name)
	}
	if strings.Join(names, ",") != "a,m,z" {
		t.Errorf("records = %v, want a,m,z in order", names)
	}
}

func buildLeaf(t *testing.T, recs ...Record) []byte {
	t.Helper()
	node := make([]byte, pageSize)
	binary.BigEndian.PutUint32(node[0:4], 0)
	binary.BigEndian.PutUint32(node[4:8], uint32(len(recs)))
	p := 8
	for _, r := range recs {
		enc, err := encodeRecord(r)
		if err != nil {
			t.Fatal(err)
		}
		copy(node[p:], enc)
		p += len(enc)
	}
	return node
}

// The with-a-picture arm of the icon view, and the non-default sizes.
func TestIconViewWithABackground(t *testing.T) {
	s := &Store{}
	if err := s.SetIconView(IconView{
		Background: "/.background/bg.png", VolumeName: "VOL",
		IconSize: 128, TextSize: 14, GridSpacing: 120,
	}); err != nil {
		t.Fatal(err)
	}
	for _, r := range s.Records() {
		if r.ID != "icvp" {
			continue
		}
		var d map[string]any
		if _, err := plist.Unmarshal([]byte(r.Val.(Blob)), &d); err != nil {
			t.Fatal(err)
		}
		if d["backgroundType"] != uint64(2) {
			t.Errorf("backgroundType = %v, want 2 with a picture", d["backgroundType"])
		}
		if _, ok := d["backgroundImageAlias"].([]byte); !ok {
			t.Error("no alias was written for the picture")
		}
		if d["iconSize"] != 128.0 || d["textSize"] != 14.0 || d["gridSpacing"] != 120.0 {
			t.Errorf("explicit sizes were not kept: %v %v %v", d["iconSize"], d["textSize"], d["gridSpacing"])
		}
		return
	}
	t.Error("no icvp record")
}

// Truncating the FILE is caught by the block bounds check before any record
// is read, so the record-level guards need a different attack: a block that
// is intact but whose contents lie about their own lengths.
func TestParseRefusesRecordsThatLieAboutTheirLength(t *testing.T) {
	build := func() ([]byte, []byte) {
		s := &Store{}
		s.Add(Record{Name: "MyApp.app", ID: "Iloc", Val: Blob(make([]byte, 16))})
		b, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		off := binary.BigEndian.Uint32(b[8:12])
		info := b[abs(off):]
		nodeAddr := binary.BigEndian.Uint32(info[8+4*nodeBlock:])
		return b, b[abs(nodeAddr&^0x1F):]
	}
	for _, tc := range []struct {
		name string
		bend func(node []byte)
	}{
		{"a name longer than the node", func(n []byte) { binary.BigEndian.PutUint32(n[8:], 1<<20) }},
		{"a blob longer than the node", func(n []byte) {
			i := indexOf(n[:128], []byte("Ilocblob"))
			binary.BigEndian.PutUint32(n[i+8:], 1<<20)
		}},
		{"more records than fit", func(n []byte) { binary.BigEndian.PutUint32(n[4:], 1<<16) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, node := build()
			tc.bend(node)
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("panicked instead of refusing: %v", r)
				}
			}()
			if _, err := Parse(b); err == nil {
				t.Error("accepted a record that lies about its length")
			}
		})
	}
}

// The bookkeeping block's own guards: a block too short to hold the header,
// a table of contents cut off, and an address that names no block.
func TestParseRefusesADamagedBookkeepingBlock(t *testing.T) {
	build := func() []byte {
		s := &Store{}
		s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
		b, err := s.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	t.Run("block too short for its header", func(t *testing.T) {
		b := build()
		binary.BigEndian.PutUint32(b[12:16], 4) // claim a four-byte block
		if _, err := Parse(b); err == nil {
			t.Error("accepted a bookkeeping block too short to hold its own header")
		}
	})
	t.Run("table of contents cut off", func(t *testing.T) {
		b := build()
		off := binary.BigEndian.Uint32(b[8:12])
		info := b[abs(off):]
		n := int(binary.BigEndian.Uint32(info))
		p := 8 + 4*((n+255)/256*256)
		info[p+4] = 0xFF // a name length that runs past the block
		binary.BigEndian.PutUint32(b[12:16], uint32(p+6))
		if _, err := Parse(b); err == nil {
			t.Error("accepted a truncated table-of-contents entry")
		}
	})
	t.Run("DSDB names a block that does not exist", func(t *testing.T) {
		b := build()
		off := binary.BigEndian.Uint32(b[8:12])
		info := b[abs(off):]
		n := int(binary.BigEndian.Uint32(info))
		p := 8 + 4*((n+255)/256*256)
		p += 4     // past the TOC count
		p += 1 + 4 // past the name length and "DSDB"
		binary.BigEndian.PutUint32(info[p:], 99)
		if _, err := Parse(b); err == nil {
			t.Error("accepted a DSDB entry naming block 99 of 3")
		}
	})
	t.Run("the root node names a block that does not exist", func(t *testing.T) {
		b := build()
		off := binary.BigEndian.Uint32(b[8:12])
		info := b[abs(off):]
		masterAddr := binary.BigEndian.Uint32(info[8+4*masterBlock:])
		master := b[abs(masterAddr&^0x1F):]
		binary.BigEndian.PutUint32(master[0:4], 99)
		if _, err := Parse(b); err == nil {
			t.Error("accepted a root node naming block 99 of 3")
		}
	})
}

// There is deliberately NO test for "a record whose payload runs past the end
// of its block". It cannot happen: a block is always a power of two, so a
// record ending at the last written byte still has padding after it, and the
// guards in decodeRecord read that padding as a valid empty value. Three
// attempts to provoke them all read back as valid records.
//
// Those guards are defence for a future caller that hands decodeRecord a
// slice that is not a whole block. They are noted in parse.go as unreachable
// through Parse, and they are why coverage stops short of 100%: a branch no
// input can reach is a branch no test can honestly claim.

// wrapNode builds the smallest valid file around a node of the given bytes,
// with the node block sized to exactly that length.
func wrapNode(t *testing.T, node []byte) []byte {
	t.Helper()
	master := make([]byte, 20)
	binary.BigEndian.PutUint32(master[0:4], nodeBlock)
	binary.BigEndian.PutUint32(master[16:20], pageSize)
	a := newAllocator()
	mOff, mW := a.alloc(len(master))
	nOff, nW := a.alloc(len(node))
	addrs := []uint32{0, mOff | uint32(mW), nOff | uint32(nW)}
	trial := encodeBookkeeping(addrs, a)
	iOff, iW := a.alloc(len(trial))
	addrs[0] = iOff | uint32(iW)
	info := encodeBookkeeping(addrs, a)
	out := make([]byte, int(a.allocatedEnd())+4)
	binary.BigEndian.PutUint32(out[0:4], 1)
	copy(out[4:8], "Bud1")
	binary.BigEndian.PutUint32(out[8:12], iOff)
	binary.BigEndian.PutUint32(out[12:16], uint32(1)<<uint(iW))
	binary.BigEndian.PutUint32(out[16:20], iOff)
	copy(out[abs(mOff):], master)
	copy(out[abs(nOff):], node)
	copy(out[abs(iOff):], info)
	return out
}
