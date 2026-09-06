# testdata

`finder-macos26.DS_Store` was written by the Finder itself, on macOS 26, for
the exact case this package exists to reproduce: a volume with a background
picture in `.background/bg.png` and two icons placed on it.

It is committed because it is the only ground truth there is. The published
descriptions of this format disagree with what the Finder actually does in at
least three places, and the tests here check the package against the file
rather than against a document.

Nothing in it is private: a scratch volume named `GODSREF`, a synthetic PNG,
and two placeholder entries. It was regenerated once already, after the first
copy was lost with a purged scratch directory — the recipe is in
`reference_test.go`.
