// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"bytes"
	"encoding/binary"
	"slices"
)

const pageSize = 0x1000 // the DSDB node size, and what the master block records

// Store is a set of records destined for one directory's .DS_Store.
type Store struct{ recs []Record }

// Add appends a record, replacing any earlier one with the same name and id
// so that a builder layering defaults over a caller's wishes works.
func (s *Store) Add(r Record) {
	for i := range s.recs {
		if s.recs[i].Name == r.Name && s.recs[i].ID == r.ID {
			s.recs[i] = r
			return
		}
	}
	s.recs = append(s.recs, r)
}

// Records returns the records in the order they will be written.
func (s *Store) Records() []Record {
	out := slices.Clone(s.recs)
	slices.SortStableFunc(out, byFinderOrder)
	return out
}

// Bytes serialises the store.
func (s *Store) Bytes() ([]byte, error) {
	node, err := s.encodeLeaf()
	if err != nil {
		return nil, err
	}
	master := make([]byte, 20)
	binary.BigEndian.PutUint32(master[0:4], nodeBlock)
	binary.BigEndian.PutUint32(master[4:8], 0) // levels: a lone leaf
	binary.BigEndian.PutUint32(master[8:12], uint32(len(s.recs)))
	binary.BigEndian.PutUint32(master[12:16], 1) // nodes
	binary.BigEndian.PutUint32(master[16:20], pageSize)

	a := newAllocator()
	masterOff, masterW := a.alloc(len(master))
	nodeOff, nodeW := a.alloc(len(node))

	// THE ORDER OF THIS TABLE IS LOAD-BEARING, and nothing documents it.
	//
	// Block 0 must be the bookkeeping block itself, 1 the DSDB master, 2 the
	// root node — the order hdiutil's Finder writes. With the bookkeeping
	// block last, the Finder reads the file, a lenient parser finds every
	// record in it, and it is ignored: 48-pixel icons instead of 96, measured
	// on a fresh volume against a paired control. Putting it first is the
	// single change that made the Finder honour a file written here.
	addrs := []uint32{
		0, // placeholder: the bookkeeping block describes itself
		masterOff | uint32(masterW),
		nodeOff | uint32(nodeW),
	}
	// Its own size has to be settled before it can be allocated, so one trial
	// pass gives the length and the second writes the real thing.
	trial := encodeBookkeeping(addrs, a)
	infoOff, infoW := a.alloc(len(trial))
	addrs[infoBlock] = infoOff | uint32(infoW)
	info := encodeBookkeeping(addrs, a)

	out := make([]byte, int(a.allocatedEnd())+4)
	binary.BigEndian.PutUint32(out[0:4], 1) // alignment / version
	copy(out[4:8], "Bud1")
	binary.BigEndian.PutUint32(out[8:12], infoOff)
	// The SIZE is the allocated block's, not the bytes used inside it: the
	// Finder writes 2048 for a block holding 1277.
	binary.BigEndian.PutUint32(out[12:16], uint32(1)<<uint(infoW))
	// Not redundancy a reader may skip: a mismatch between these two is how
	// the file is judged damaged.
	binary.BigEndian.PutUint32(out[16:20], infoOff)

	copy(out[abs(masterOff):], master)
	copy(out[abs(nodeOff):], node)
	copy(out[abs(infoOff):], info)
	return out, nil
}

// Block indices in the address table. See the note in Bytes: these are not
// free choices.
const (
	infoBlock   = 0
	masterBlock = 1
	nodeBlock   = 2
)

// abs turns a stored (relative) offset back into a file offset.
func abs(relOffset uint32) int { return int(relOffset) + 4 }

// encodeBookkeeping writes the address table, the "DSDB" table of contents
// and the thirty-two free lists.
func encodeBookkeeping(addrs []uint32, a *allocator) []byte {
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(addrs)))
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // unknown, always zero
	for _, addr := range addrs {
		_ = binary.Write(&b, binary.BigEndian, addr)
	}
	// The table is padded to a multiple of 256 entries; the table of contents
	// starts after the PADDING, not after the entries.
	for i := len(addrs); i < (len(addrs)+255)/256*256; i++ {
		_ = binary.Write(&b, binary.BigEndian, uint32(0))
	}
	_ = binary.Write(&b, binary.BigEndian, uint32(1)) // one TOC entry
	b.WriteByte(4)
	b.WriteString("DSDB")
	// …naming the MASTER block, not the root node. Pointing it at the node
	// made a reader parse record bytes as the master: pageSize came back as
	// 0x7370626c, which is "spbl" — the middle of a blob type tag.
	_ = binary.Write(&b, binary.BigEndian, uint32(masterBlock))
	// Thirty-two free lists, one per width. Writing them all empty is what a
	// hand-placed file does, and such a file is ignored: the lists are how
	// the image says which space is unused.
	for w := 0; w < 32; w++ {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(a.free[w])))
		for _, off := range a.free[w] {
			_ = binary.Write(&b, binary.BigEndian, off)
		}
	}
	return b.Bytes()
}

// encodeLeaf writes the single leaf node: a zero "next node" pointer marking
// it a leaf, the record count, then the records back to back.
func (s *Store) encodeLeaf() ([]byte, error) {
	var body bytes.Buffer
	for _, r := range s.Records() {
		enc, err := encodeRecord(r)
		if err != nil {
			return nil, err
		}
		body.Write(enc)
	}
	if 8+body.Len() > pageSize {
		return nil, ErrTooLarge
	}
	node := make([]byte, pageSize)
	binary.BigEndian.PutUint32(node[0:4], 0) // P == 0: this is a leaf
	binary.BigEndian.PutUint32(node[4:8], uint32(len(s.recs)))
	copy(node[8:], body.Bytes())
	return node, nil
}
