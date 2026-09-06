// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"path"
	"strings"
	"unicode/utf16"
)

// The Alias Manager record, version 2 — what icvp's backgroundImageAlias
// holds, and the reason a background survives the volume being mounted at a
// different /Volumes path: the relativity lives inside the alias, in tag 18.
//
// The layout was read off an alias the Finder itself wrote on macOS 26 for a
// background picture, not taken from a document. The one published
// description of the tagged section (mac_alias/docs/alias_fmt.rst) is wrong
// about tags 14 and 15: they carry a uint16 count of UTF-16 code units BEFORE
// the text. The reference has the count, so this does.
//
// Everything a synthesised alias cannot know — catalog node ids, volume
// creation dates, the volume's attribute word — is left zero.

const (
	aliasVersion2  = 2
	aliasKindFile  = 0
	aliasHeaderLen = 150 // the fixed part, before the tagged section
)

// The tags the Finder writes for a background picture.
const (
	tagParentName    = 0
	tagParentCNID    = 1
	tagCarbonPath    = 2
	tagTargetUnicode = 14
	tagVolumeUnicode = 15
	tagPOSIXPath     = 18 // relative to the volume: "/.background/bg.png"
	tagMountPoint    = 19 // "/Volumes/NAME"
	tagEnd           = -1
)

// BuildAlias encodes an alias v2 pointing at relPath on the named volume.
//
// relPath is the path INSIDE the volume, leading slash and all, because that
// is what tag 18 carries and what lets the reference survive being mounted
// somewhere else.
func BuildAlias(volumeName, relPath string) ([]byte, error) {
	if volumeName == "" {
		return nil, fmt.Errorf("dsstore: an alias needs a volume name")
	}
	if !strings.HasPrefix(relPath, "/") {
		return nil, fmt.Errorf("dsstore: alias path %q must start at the volume root", relPath)
	}
	target := path.Base(relPath)
	parent := path.Base(path.Dir(relPath))
	if parent == "/" || parent == "." {
		parent = volumeName
	}

	var b bytes.Buffer
	b.Write(make([]byte, 4))                          // appInfo
	_ = binary.Write(&b, binary.BigEndian, uint16(0)) // recSize, patched below
	_ = binary.Write(&b, binary.BigEndian, uint16(aliasVersion2))
	_ = binary.Write(&b, binary.BigEndian, uint16(aliasKindFile))
	writePascal(&b, volumeName, 28)                   // 10 volume name
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // 38 volume date
	b.WriteString("H+")                               // 42 filesystem type
	_ = binary.Write(&b, binary.BigEndian, uint16(5)) // 44 disk type
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // 46 parent CNID
	writePascal(&b, target, 64)                       // 50 target name
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // 114 target CNID
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // 118 create date
	b.Write(make([]byte, 4))                          // 122 creator
	b.Write(make([]byte, 4))                          // 126 type
	_ = binary.Write(&b, binary.BigEndian, int16(-1)) // 130 levels up
	_ = binary.Write(&b, binary.BigEndian, int16(-1)) // 132 levels down
	_ = binary.Write(&b, binary.BigEndian, uint32(0)) // 134 volume attributes
	_ = binary.Write(&b, binary.BigEndian, uint16(0)) // 138 filesystem id
	b.Write(make([]byte, 10))                         // 140 reserved
	// The header's length is an invariant of the code just above, not a
	// property of the input, so it is asserted in a test rather than checked
	// here: a branch no input can reach is a branch no test can check.

	// The Carbon path the Finder writes is colon-separated and keeps a NUL
	// where the last separator would be: "VOL:dir:\x00file".
	carbon := volumeName + ":" + parent + ":\x00" + target

	writeTag(&b, tagParentName, []byte(parent))
	writeTag(&b, tagParentCNID, be32(0))
	writeTag(&b, tagCarbonPath, []byte(carbon))
	writeTag(&b, tagTargetUnicode, countedUTF16(target))
	writeTag(&b, tagVolumeUnicode, countedUTF16(volumeName))
	writeTag(&b, tagPOSIXPath, []byte(relPath))
	writeTag(&b, tagMountPoint, []byte("/Volumes/"+volumeName))
	_ = binary.Write(&b, binary.BigEndian, int16(tagEnd))
	_ = binary.Write(&b, binary.BigEndian, uint16(0))

	out := b.Bytes()
	binary.BigEndian.PutUint16(out[4:6], uint16(len(out)))
	return out, nil
}

// writePascal writes a length-prefixed string padded to a fixed field.
func writePascal(b *bytes.Buffer, s string, field int) {
	raw := []byte(s)
	if len(raw) > field-1 {
		raw = raw[:field-1]
	}
	b.WriteByte(byte(len(raw)))
	b.Write(raw)
	b.Write(make([]byte, field-1-len(raw)))
}

// writeTag appends one tagged value, padded to an even length.
func writeTag(b *bytes.Buffer, tag int16, val []byte) {
	_ = binary.Write(b, binary.BigEndian, tag)
	_ = binary.Write(b, binary.BigEndian, uint16(len(val)))
	b.Write(val)
	if len(val)%2 == 1 {
		b.WriteByte(0)
	}
}

// countedUTF16 is the shape tags 14 and 15 actually use: a uint16 count of
// code units, then UTF-16BE. The published description omits the count, and
// an alias without it is silently not resolved.
func countedUTF16(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 2+2*len(u))
	binary.BigEndian.PutUint16(out[:2], uint16(len(u)))
	for i, c := range u {
		binary.BigEndian.PutUint16(out[2+2*i:], c)
	}
	return out
}

func be32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}
