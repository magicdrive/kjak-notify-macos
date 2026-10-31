# Changelog

## v0.1.0 (2026-10-31)

First release. Experimental.

- `kjak-feather-notify-macos`: kjak external feather plugin (protocol v1,
  kjak v1.1.0 and later) posting macOS notifications with terminal-notifier
  3.1.0.
- Released with Hephaestus v1.5.0: `darwin_amd64` and `darwin_arm64`
  archives, `kjak-notify-macos_checksums.txt`, GitHub Release, and the
  formula `kjak-notify-macos` on `magicdrive/tap` (depends on
  terminal-notifier). Prereleases never update the tap.
- Installed by Homebrew, it runs under kjak v1.2.0 or later with its
  SHA-256 pinned (Homebrew's bin and Cellar are writable by group admin);
  kjak v1.1.0 refuses it. See the README.
