// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

// Package dsstore writes the .DS_Store file the Finder keeps in a directory,
// in pure Go with CGO_ENABLED=0 and no shelling out.
//
// It exists for one job the rest of this fleet could not finish: a disk image
// that opens showing a background picture with its icons arranged on it. The
// picture is not a property of the volume, the image, or the filesystem — it
// is a record inside .DS_Store, and nothing in Go could write one.
//
// # The format
//
// A .DS_Store is a "buddy allocator" image (magic "Bud1") holding one B-tree
// named DSDB, whose records are (filename, four-character structure id, typed
// value) triples sorted by case-insensitive filename. EVERYTHING is
// big-endian, and every stored offset is relative to byte 4 of the file.
//
// It is an allocator IMAGE, not a container with blocks placed in it, and the
// Finder reads it as one: a file whose blocks are laid out by hand parses
// perfectly and is silently ignored.
//
// The layout here was read off files the Finder itself wrote on macOS 26, for
// exactly this case — a volume with a background picture and positioned
// icons. Where the published descriptions disagree with what the Finder does,
// the comments say so at the point it matters.
//
// # What it deliberately does not do
//
// The B-tree is written as a SINGLE leaf node. A window with a background has
// a handful of entries, which fits a 4 KiB page many times over; a store that
// needs more returns [ErrTooLarge] rather than emitting a file with a
// half-implemented split in it. Growing to a real B-tree is a change to this
// package, not to its callers.
package dsstore
