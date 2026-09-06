// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import "slices"

// The buddy allocator the file is built out of.
//
// A .DS_Store is not a container with blocks placed in it — it IS an
// allocator image, and the Finder reads it as one. Measured on macOS 26, on
// fresh volumes with a paired control: a hand-placed file with the same
// records left the window at the default 48-pixel icons, while the Finder's
// own file gave the 96 the records ask for. The free lists and the
// self-describing bookkeeping block are what make the file legible.
//
// Addresses are relative to byte 4 of the file, and a block's offset is
// aligned to its own size — which is what lets one word carry both: the low
// five bits are the width, the rest is the offset.

const (
	addrSpaceWidth = 31 // the whole address space, as the Finder writes it
	blockMin       = 5  // the smallest block is 1<<5 = 32 bytes
)

type allocator struct {
	free [32][]uint32 // free[w] holds relative offsets of blocks of size 1<<w
	high uint32       // one past the end of the highest block handed out
}

func newAllocator() *allocator {
	a := &allocator{}
	a.free[addrSpaceWidth] = []uint32{0}
	// Relative offsets 0..63 are the buddy header — the magic, the two copies
	// of the bookkeeping offset, and the slack after them. Handing out offset
	// 0 puts a block on top of "Bud1" (a file whose own magic read
	// 00 00 00 01), and the Finder's own files never place a block below 64.
	a.alloc(1 << (blockMin + 1))
	return a
}

// alloc returns the relative offset of a block of at least n bytes, splitting
// larger free blocks until one of the right width exists.
func (a *allocator) alloc(n int) (offset uint32, width int) {
	w := widthFor(n)
	src := w
	for src < len(a.free) && len(a.free[src]) == 0 {
		src++
	}
	if src >= len(a.free) {
		panic("dsstore: address space exhausted") // 2 GiB; unreachable in practice
	}
	off := a.free[src][0]
	a.free[src] = a.free[src][1:]
	for src > w { // split down, each buddy onto its own list
		src--
		a.free[src] = append(a.free[src], off+1<<src)
		slices.Sort(a.free[src])
	}
	if end := off + 1<<w; end > a.high {
		a.high = end
	}
	return off, w
}

// allocatedEnd is one past the highest byte any allocated block occupies.
//
// It is tracked as blocks are handed out rather than inferred from the free
// lists. The allocated region is NOT a prefix: alloc takes the lowest free
// block of the width it needs, so a small block can sit below a large hole,
// and "the lowest free offset" is nowhere near the end. Inferring it that way
// produced a 36-byte file.
func (a *allocator) allocatedEnd() uint32 { return a.high }

// widthFor is the smallest power of two that holds n bytes, floored at the
// allocator's 32-byte minimum.
//
// The bound is on n-1, not n: rounding an exact power of two up to the next
// one would make a 4096-byte page claim an 8192-byte block. The reference
// Python implementation has that bug.
func widthFor(n int) int {
	w := blockMin
	for 1<<w < n {
		w++
	}
	return w
}
