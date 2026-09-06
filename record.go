// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode/utf16"
)

// ErrTooLarge is returned when the records do not fit the single leaf node
// this package writes. See the package comment: a partial B-tree is worse
// than a refusal.
var ErrTooLarge = errors.New("dsstore: records exceed one 4 KiB node")

// A Record is one (name, structure id, value) triple. Name is the entry the
// record is about; "." means the directory itself, which is where the
// window's own settings live.
type Record struct {
	Name string
	ID   string // four characters: "Iloc", "icvp", "bwsp", "vSrn", …
	Val  Value
}

// A Value is one of the typed payloads the format defines. Only the four this
// package needs are implemented; the rest are rejected rather than guessed at.
type Value interface{ encode() (kind string, data []byte) }

// Long is the "long" type: a 32-bit integer.
type Long uint32

func (v Long) encode() (string, []byte) {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(v))
	return "long", b
}

// Bool is the "bool" type: a single byte.
type Bool bool

func (v Bool) encode() (string, []byte) {
	if v {
		return "bool", []byte{1}
	}
	return "bool", []byte{0}
}

// Type is the "type" type: four characters, such as 'icnv'.
type Type string

func (v Type) encode() (string, []byte) { return "type", []byte(v) }

// Blob is the "blob" type: a length-prefixed byte string. Both the window
// settings (a binary plist) and an icon position are blobs.
type Blob []byte

func (v Blob) encode() (string, []byte) {
	b := make([]byte, 4+len(v))
	binary.BigEndian.PutUint32(b[:4], uint32(len(v)))
	copy(b[4:], v)
	return "blob", b
}

// encodeRecord lays out one record: a UTF-16BE name whose length is in CODE
// UNITS (not bytes, and not runes), the structure id, the type, the payload.
func encodeRecord(r Record) ([]byte, error) {
	if len(r.ID) != 4 {
		return nil, fmt.Errorf("dsstore: structure id %q is not four characters", r.ID)
	}
	if r.Val == nil {
		return nil, fmt.Errorf("dsstore: record %q/%s has no value", r.Name, r.ID)
	}
	kind, payload := r.Val.encode()
	name := utf16.Encode([]rune(r.Name))
	var b bytes.Buffer
	_ = binary.Write(&b, binary.BigEndian, uint32(len(name)))
	for _, u := range name {
		_ = binary.Write(&b, binary.BigEndian, u)
	}
	b.WriteString(r.ID)
	b.WriteString(kind)
	b.Write(payload)
	return b.Bytes(), nil
}

// byFinderOrder sorts records the way the tree expects: by name,
// case-insensitively, then by structure id.
func byFinderOrder(a, b Record) int {
	if c := strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}
