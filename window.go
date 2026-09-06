// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"fmt"
)

// A Window is where the window opens and how large it is — the "bwsp" record.
//
// It is separate from IconView because it is a different record and answers a
// different question: icvp says what the window shows behind its icons, bwsp
// says how big the window is. A background picture with no bwsp is a picture
// cropped to whatever size the Finder last used.
type Window struct {
	// X and Y are the window's top-left corner on screen; Width and Height
	// are its content area, which is the area the background covers.
	X, Y, Width, Height int
}

// bwsp assembles the record's binary plist.
//
// The five booleans are not fields: they are what the Finder wrote in the
// reference file, and none of them is a choice a disk-image window makes. A
// caller who wants a sidebar in a drag-to-install window wants something else.
func (w Window) bwsp() ([]byte, error) {
	d := map[string]any{
		"ContainerShowSidebar": false,
		"ShowSidebar":          false,
		"ShowStatusBar":        false,
		"ShowTabView":          true,
		"ShowToolbar":          false,
		"WindowBounds":         fmt.Sprintf("{{%d, %d}, {%d, %d}}", w.X, w.Y, w.Width, w.Height),
	}
	b, err := encodeBinaryPlist(d)
	if err != nil {
		return nil, fmt.Errorf("dsstore: encoding bwsp: %w", err)
	}
	return b, nil
}

// SetWindow records where the window opens and how large it is.
func (s *Store) SetWindow(w Window) error {
	b, err := w.bwsp()
	if err != nil {
		return err
	}
	s.Add(Record{Name: ".", ID: "bwsp", Val: Blob(b)})
	return nil
}
