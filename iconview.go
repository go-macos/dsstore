// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"howett.net/plist"
)

// IconView is the window's icon-view settings — the "icvp" record, which is
// what carries the background picture.
//
// The keys and their types were read off the Finder's own record: the numbers
// are REALS, not integers, and backgroundType is 2 for "a picture". The older
// BKGD and pict records are dead on modern macOS: a file carrying a valid
// BKGD/pict pair and no icvp made the Finder write a fresh icvp with
// backgroundType 0, ignoring them.
type IconView struct {
	// Background names the picture inside the volume, leading slash and all
	// (".background/bg.png" is conventional and hidden). Empty means no
	// picture, and backgroundType drops to 1 — a plain colour.
	Background string
	// VolumeName is the volume the picture lives on. The alias records it,
	// which is how the reference survives a different mount point.
	VolumeName string

	IconSize    float64 // 96 is what a disk-image window usually wants
	TextSize    float64 // 12 in the Finder's own file
	GridSpacing float64 // 100
	LabelBottom bool
	ShowPreview bool
	ShowInfo    bool
}

// icvp assembles the record's binary plist.
func (v IconView) icvp() ([]byte, error) {
	d := map[string]any{
		"viewOptionsVersion":   1,
		"arrangeBy":            "none",
		"backgroundColorRed":   1.0,
		"backgroundColorGreen": 1.0,
		"backgroundColorBlue":  1.0,
		"gridOffsetX":          0.0,
		"gridOffsetY":          0.0,
		"gridSpacing":          orDefault(v.GridSpacing, 100),
		"iconSize":             orDefault(v.IconSize, 96),
		"textSize":             orDefault(v.TextSize, 12),
		"labelOnBottom":        v.LabelBottom,
		"showIconPreview":      v.ShowPreview,
		"showItemInfo":         v.ShowInfo,
		"scrollPositionX":      0.0,
		"scrollPositionY":      0.0,
		"backgroundType":       1,
	}
	if v.Background != "" {
		alias, err := BuildAlias(v.VolumeName, v.Background)
		if err != nil {
			return nil, err
		}
		d["backgroundType"] = 2
		d["backgroundImageAlias"] = alias
	}
	var buf bytes.Buffer
	enc := plist.NewBinaryEncoder(&buf)
	if err := enc.Encode(d); err != nil {
		return nil, fmt.Errorf("dsstore: encoding icvp: %w", err)
	}
	return buf.Bytes(), nil
}

func orDefault(v, def float64) float64 {
	if v == 0 {
		return def
	}
	return v
}

// SetIconView records the window's icon-view settings.
func (s *Store) SetIconView(v IconView) error {
	b, err := v.icvp()
	if err != nil {
		return err
	}
	s.Add(Record{Name: ".", ID: "icvp", Val: Blob(b)})
	s.Add(Record{Name: ".", ID: "vSrn", Val: Long(1)})
	return nil
}

// SetIconPosition places one entry's icon. x and y are the icon's CENTRE, in
// the window's coordinates.
func (s *Store) SetIconPosition(name string, x, y uint32) {
	b := make([]byte, 16)
	binary.BigEndian.PutUint32(b[0:4], x)
	binary.BigEndian.PutUint32(b[4:8], y)
	// The trailing six 0xFF bytes and two zeros are what the Finder writes;
	// their meaning is not documented anywhere I could find, and a reader
	// that expects them is a reader this has to satisfy.
	copy(b[8:], []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00})
	s.Add(Record{Name: name, ID: "Iloc", Val: Blob(b)})
}
