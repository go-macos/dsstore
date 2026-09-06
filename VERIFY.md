# How this package was verified against macOS, and how to do it again

The Finder is the only judge that matters, and it is easy to get a wrong
answer out of it. Two ways to be misled, both of which happened here:

## The judge has to be validated first

Creating a **fresh volume** and putting a `.DS_Store` on it proves nothing:
the Finder ignores it, and it ignores its own file the same way. A run where
your file "fails" that way is not evidence against your file.

The harness that does work keeps the volume's identity constant:

1. `hdiutil create -size 12m -fs HFS+ -volname JUDGE -layout NONE -ov judge.dmg`
2. attach, configure the window through the Finder over AppleScript, detach
3. attach again — the settings must come back. **This is the positive
   control.** Without it you cannot read anything into a negative result.
4. Now attach, replace `.DS_Store` with yours, detach, attach, and ask again.

## Ask for something that cannot be carried over

`icon size` alone is a poor discriminator: the Finder applies the last
window's settings to a new window, so 96 can appear without your file being
read at all. An early "proof" here rested on exactly that and was wrong.

**Icon positions cannot be carried over.** Ask for those:

```applescript
tell application "Finder"
  set p to position of item "MyApp.app" of disk "JUDGE"
  return (item 1 of p) & "," & (item 2 of p)
end tell
```

## What is proven, and what is not

Proven, with the harness above and a positive control:

- icon positions — the Finder read 160,220 and 440,220 out of a file this
  package wrote, where its own earlier configuration had 160,247, so the
  values are ours and not a leftover;
- icon size — 96, from the same file.

**Not proven:** the background picture. `background picture of icon view
options` raises an AppleScript error even for a window the Finder itself
configured with one, so this harness cannot measure it. The alias inside the
record matches the Finder's field by field (see `alias_test.go`), but that is
agreement with a reference, not an observation of the picture appearing.
