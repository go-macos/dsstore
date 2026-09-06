// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"encoding/binary"
	"fmt"
	"unicode/utf16"
)

// Parse reads a .DS_Store.
//
// It exists as much for the tests as for callers: the only way to know this
// package writes what the Finder writes is to read the Finder's own file and
// compare. A writer checked against nothing but itself is checked against
// nothing.
func Parse(b []byte) (*Store, error) {
	if len(b) < 36 {
		return nil, fmt.Errorf("dsstore: too short to be a .DS_Store")
	}
	if string(b[4:8]) != "Bud1" {
		return nil, fmt.Errorf("dsstore: bad magic %q", b[4:8])
	}
	infoOff := binary.BigEndian.Uint32(b[8:12])
	infoLen := binary.BigEndian.Uint32(b[12:16])
	if infoOff != binary.BigEndian.Uint32(b[16:20]) {
		return nil, fmt.Errorf("dsstore: the two bookkeeping offsets disagree")
	}
	info, err := slice(b, abs(infoOff), int(infoLen))
	if err != nil {
		return nil, fmt.Errorf("dsstore: bookkeeping block: %w", err)
	}
	if len(info) < 8 {
		return nil, fmt.Errorf("dsstore: bookkeeping block too short")
	}
	n := int(binary.BigEndian.Uint32(info[0:4]))
	if 8+4*n > len(info) {
		return nil, fmt.Errorf("dsstore: address table of %d claims more than the block holds", n)
	}
	addrs := make([]uint32, n)
	for i := range addrs {
		addrs[i] = binary.BigEndian.Uint32(info[8+4*i:])
	}
	// The table of contents follows the PADDING, not the entries.
	p := 8 + 4*((n+255)/256*256)
	if p+4 > len(info) {
		return nil, fmt.Errorf("dsstore: no table of contents")
	}
	tocCount := int(binary.BigEndian.Uint32(info[p:]))
	p += 4
	master := -1
	for range tocCount {
		if p >= len(info) {
			return nil, fmt.Errorf("dsstore: truncated table of contents")
		}
		l := int(info[p])
		p++
		if p+l+4 > len(info) {
			return nil, fmt.Errorf("dsstore: truncated table-of-contents entry")
		}
		name := string(info[p : p+l])
		p += l
		blk := int(binary.BigEndian.Uint32(info[p:]))
		p += 4
		if name == "DSDB" {
			master = blk
		}
	}
	if master < 0 || master >= len(addrs) {
		return nil, fmt.Errorf("dsstore: no DSDB entry")
	}
	mb, err := blockAt(b, addrs[master])
	if err != nil || len(mb) < 20 {
		return nil, fmt.Errorf("dsstore: DSDB master block unreadable")
	}
	root := int(binary.BigEndian.Uint32(mb[0:4]))

	s := &Store{}
	if err := readNode(b, addrs, root, s); err != nil {
		return nil, err
	}
	return s, nil
}

// readNode walks one node, descending into children so that a file with a
// real B-tree still reads even though this package only writes one leaf.
func readNode(b []byte, addrs []uint32, idx int, s *Store) error {
	if idx < 0 || idx >= len(addrs) {
		return fmt.Errorf("dsstore: node %d out of range", idx)
	}
	nd, err := blockAt(b, addrs[idx])
	if err != nil || len(nd) < 8 {
		return fmt.Errorf("dsstore: node %d unreadable", idx)
	}
	next := int(binary.BigEndian.Uint32(nd[0:4]))
	count := int(binary.BigEndian.Uint32(nd[4:8]))
	p := 8
	for range count {
		if next != 0 {
			if p+4 > len(nd) {
				return fmt.Errorf("dsstore: truncated internal node")
			}
			child := int(binary.BigEndian.Uint32(nd[p:]))
			p += 4
			if err := readNode(b, addrs, child, s); err != nil {
				return err
			}
		}
		rec, used, err := decodeRecord(nd[p:])
		if err != nil {
			return err
		}
		p += used
		s.Add(rec)
	}
	if next != 0 {
		return readNode(b, addrs, next, s)
	}
	return nil
}

// decodeRecord reads one record from the start of b.
//
// Its length guards are unreachable through Parse: b is always the tail of a
// whole block, a block is always a power of two, and so a record ending at
// the last written byte still has padding behind it that reads back as a
// valid empty value. They are kept for a caller that passes something else,
// and they are the reason this file does not reach 100% coverage.
func decodeRecord(b []byte) (Record, int, error) {
	var r Record
	if len(b) < 4 {
		return r, 0, fmt.Errorf("dsstore: truncated record")
	}
	n := int(binary.BigEndian.Uint32(b[0:4]))
	p := 4
	if p+2*n+8 > len(b) {
		return r, 0, fmt.Errorf("dsstore: truncated record name")
	}
	u := make([]uint16, n)
	for i := range u {
		u[i] = binary.BigEndian.Uint16(b[p+2*i:])
	}
	r.Name = string(utf16.Decode(u))
	p += 2 * n
	r.ID = string(b[p : p+4])
	kind := string(b[p+4 : p+8])
	p += 8
	switch kind {
	case "long", "shor":
		if p+4 > len(b) {
			return r, 0, fmt.Errorf("dsstore: truncated %s", kind)
		}
		r.Val = Long(binary.BigEndian.Uint32(b[p:]))
		p += 4
	case "bool":
		if p+1 > len(b) {
			return r, 0, fmt.Errorf("dsstore: truncated bool")
		}
		r.Val = Bool(b[p] != 0)
		p++
	case "type":
		if p+4 > len(b) {
			return r, 0, fmt.Errorf("dsstore: truncated type")
		}
		r.Val = Type(b[p : p+4])
		p += 4
	case "blob":
		if p+4 > len(b) {
			return r, 0, fmt.Errorf("dsstore: truncated blob length")
		}
		l := int(binary.BigEndian.Uint32(b[p:]))
		p += 4
		if p+l > len(b) {
			return r, 0, fmt.Errorf("dsstore: blob claims %d bytes", l)
		}
		r.Val = Blob(b[p : p+l])
		p += l
	default:
		// Refuse rather than skip: the length of an unknown type is unknown,
		// so every record after it would be read from the wrong offset.
		return r, 0, fmt.Errorf("dsstore: unsupported value type %q in record %q/%s", kind, r.Name, r.ID)
	}
	return r, p, nil
}

// blockAt resolves one packed address into the bytes it names.
func blockAt(b []byte, addr uint32) ([]byte, error) {
	off := addr &^ 0x1F
	size := 1 << (addr & 0x1F)
	return slice(b, abs(off), size)
}

func slice(b []byte, off, n int) ([]byte, error) {
	if off < 0 || n < 0 || off+n > len(b) {
		return nil, fmt.Errorf("block at %d+%d is outside a %d-byte file", off, n, len(b))
	}
	return b[off : off+n], nil
}
