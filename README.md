<p align="center"><img src="https://raw.githubusercontent.com/go-macos/brand/main/social/go-macos.png" alt="go-macos/dsstore" width="720"></p>

# go-macos/dsstore

[![Go Reference](https://pkg.go.dev/badge/github.com/go-macos/dsstore.svg)](https://pkg.go.dev/github.com/go-macos/dsstore)
[![License: BSD-3-Clause](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)
[![CI](https://github.com/go-macos/dsstore/actions/workflows/ci.yml/badge.svg)](https://github.com/go-macos/dsstore/actions/workflows/ci.yml)

**The `.DS_Store` the Finder keeps in a directory — a window's background
picture and where its icons sit — written in pure Go, `CGO_ENABLED=0`, no
shelling out.**

A disk image that opens showing a background with the application on the left
and a link to `/Applications` on the right is not doing anything clever with
the volume, the image format, or the filesystem. All of it lives in one file
in the volume's root, and until now nothing in Go could write one.

```go
var s dsstore.Store
s.SetIconView(dsstore.IconView{
    Background: "/.background/bg.png", // hidden folder, conventional
    VolumeName: "MyApp",
    IconSize:   96,
})
s.SetIconPosition("MyApp.app", 160, 220)
s.SetIconPosition("Applications", 440, 220)

b, err := s.Bytes()          // write b to <volume>/.DS_Store
```

`Parse` reads one back, which is how the tests check this package against a
file the Finder itself wrote rather than against its own output.

## What the format is

A buddy-allocator image (magic `Bud1`) holding one B-tree named `DSDB`, whose
records are (filename, four-character structure id, typed value) triples.
Everything is **big-endian**, and every stored offset is relative to byte 4.

It is an allocator *image*, not a container with blocks placed in it, and the
Finder reads it as one — a file whose blocks are laid out by hand parses
perfectly and is silently ignored.

Two placements are load-bearing and no published description mentions either:

- **block 0 must be the bookkeeping block itself**, 1 the `DSDB` master, 2 the
  root node;
- **relative offsets 0–63 are the header**; the Finder never places a block
  below 64.

Three corrections to the descriptions that do exist, each measured against a
file the Finder wrote on macOS 26 and each noted at the line it matters:

- alias tags 14 and 15 carry a `uint16` count of UTF-16 code units **before**
  the text (`mac_alias`'s documentation omits it);
- `icvl` is not written at all, though one account lists it;
- the width of an allocator block is bounded on `n-1`, so a 4096-byte page
  does not claim 8192 (the Python reference has that bug).

## What it deliberately does not do

The B-tree is written as a **single leaf node**. A window with a background
has a handful of entries, which fits a 4 KiB page many times over; a store
that needs more returns `ErrTooLarge` rather than emitting a file with a
half-implemented split in it.

## Verifying against macOS

The Finder is the only judge that matters and it is easy to get a wrong answer
out of it — a fresh volume proves nothing, because the Finder ignores its own
file on one too, and `icon size` alone is not a discriminator because the
Finder carries the last window's settings over. [VERIFY.md](VERIFY.md) records
the harness that does work, the positive control it needs, and how the
background picture is proven without a screenshot.

## Standards

Pure Go, `CGO_ENABLED=0`, no shelling out to a command-line tool in place of a
library. Built and tested on amd64, arm64, riscv64, loong64, ppc64le and
s390x — the last being big-endian, which for this format is the lane that
matters. BSD-3-Clause.

Coverage is 96.6%, below the fleet's 100%, and the missing statements are
named in the CI file rather than waved at: guards that no input reaching
`Parse` can provoke.
