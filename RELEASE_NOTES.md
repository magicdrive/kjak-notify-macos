kjak-notify-macos v0.1.0 — the first release. **Experimental.**

`kjak-feather-notify-macos` is an external feather plugin for
[kjak](https://github.com/magicdrive/kjak) v1.1.0 (plugin protocol v1) that
shows a kjak message as a macOS notification by running
[terminal-notifier](https://github.com/julienXX/terminal-notifier) 3.1.0.

- macOS only: `darwin_amd64` (Intel) and `darwin_arm64` (Apple silicon)
  archives, their SHA-256 sums in `kjak-notify-macos_checksums.txt`, and a
  build provenance attestation.
- Homebrew: `brew install magicdrive/tap/kjak-notify-macos` (installs
  terminal-notifier as its dependency, built from source).
- **Known issue with Homebrew:** kjak v1.1.0 refuses a plugin under
  Homebrew's Cellar, which its group (admin) can write to. Copy the plugin
  to a directory of your own and pin it with `path =` (the formula's
  caveats and the README show how), or install from the archive.
- Checked on a real Mac: Intel, macOS 15.7.9. **Not checked on an Apple
  silicon Mac** (the release's CI runs the tests and the Homebrew install
  on GitHub's Apple silicon runners).
- `delivered` means that macOS accepted the notification; its display is
  not guaranteed (Focus, notification settings).

Install, configuration and limits: README.md and docs/tutorial.en.md.
