kjak-notify-macos v0.1.0 — the first release. **Experimental.**

`kjak-feather-notify-macos` is an external feather plugin for
[kjak](https://github.com/magicdrive/kjak) v1.1.0 and later (plugin protocol v1) that
shows a kjak message as a macOS notification by running
[terminal-notifier](https://github.com/julienXX/terminal-notifier) 3.1.0.

- macOS only: `darwin_amd64` (Intel) and `darwin_arm64` (Apple silicon)
  archives, their SHA-256 sums in `kjak-notify-macos_checksums.txt`, and a
  build provenance attestation.
- Homebrew: `brew install magicdrive/tap/kjak-notify-macos` (installs
  terminal-notifier as its dependency, built from source).
- **Installed by Homebrew, it needs kjak v1.2.0 or later and its SHA-256
  pinned**: Homebrew's `bin` and Cellar can be written by the admin group,
  so kjak runs a plugin there only when the feather's `sha256` is the
  file's (`kjak feather plugins` shows it; update it after each `brew
  upgrade`). kjak v1.1.0 refuses it; the archive works with both.
- Checked on a real Mac: Intel, macOS 15.7.9. **Not checked on an Apple
  silicon Mac** (the release's CI runs the tests and the Homebrew install
  on GitHub's Apple silicon runners).
- `delivered` means that macOS accepted the notification; its display is
  not guaranteed (Focus, notification settings).

Install, configuration and limits: README.md and docs/tutorial.en.md.
