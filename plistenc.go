// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-macos/dsstore authors

package dsstore

import (
	"bytes"

	"howett.net/plist"
)

// encodeBinaryPlist writes a binary plist. Both records this package builds
// are one, and neither can fail for the dictionaries built here — a map of
// strings, numbers, booleans and byte slices. It is a variable so a test can
// reach the failure branches anyway: an error that cannot be produced is an
// error that has never been read.
var encodeBinaryPlist = func(v any) ([]byte, error) {
	var buf bytes.Buffer
	if err := plist.NewBinaryEncoder(&buf).Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
