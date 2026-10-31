# kjak-notify-macos

**Experimental macOS Notification Plugin for kjak**

[日本語のチュートリアル](docs/tutorial.ja.md) · [Tutorial](docs/tutorial.en.md) · [Limitations](docs/design/limitations.md)

`kjak-feather-notify-macos` is an external feather plugin for
[kjak](https://github.com/magicdrive/kjak) (plugin protocol v1) that shows a
kjak message as a macOS notification by running
[terminal-notifier](https://github.com/julienXX/terminal-notifier). It is a
thin adapter: it implements no notification code of its own.

> **Experimental.**
> - macOS only.
> - Speaks kjak's plugin protocol v1 (kjak v1.1.0 and later; installed
>   with Homebrew, kjak v1.2.0 and later).
> - Depends on terminal-notifier (3.1.0), installed separately.
> - Not a built-in kjak feather: an independent project with its own
>   releases. kjak itself still has no OS notification feather.
> - Depends on macOS notification permission. Focus, Do Not Disturb and
>   notification settings can keep a notification off the screen.
> - `delivered` means that macOS accepted the notification. Its display,
>   and that anyone saw it, are not guaranteed.
> - The plugin's settings and messages may change while it is experimental.

| | |
|---|---|
| supported | macOS, kjak v1.1.0 or later (plugin protocol v1; v1.2.0 or later for the Homebrew formula), terminal-notifier 3.1.0, a logged-in GUI session |
| checked on a real Mac | Intel (x86_64), macOS 15.7.9, terminal-notifier 3.1.0 (Homebrew, built from source), kjak v1.1.0, from a terminal of the logged-in user (2026-10-31): the plugin alone and through kjak, Japanese text and emoji, a two-line body; shown as from terminal-notifier |
| not checked | Apple silicon, other macOS versions, terminals other than iTerm2 |

## Install

Pick one way and keep to it. Every way installs the one executable
`kjak-feather-notify-macos`, which kjak finds as plugin `notify-macos`.
terminal-notifier is needed in every case.

### GitHub Releases

Each [release](https://github.com/magicdrive/kjak-notify-macos/releases)
has `kjak-notify-macos_<version>_darwin_arm64.tar.gz` (Apple silicon),
`..._darwin_amd64.tar.gz` (Intel) and `kjak-notify-macos_checksums.txt`.
Each archive holds `kjak-feather-notify-macos`, `LICENSE` and `README.md`.

```sh
brew install terminal-notifier                 # 3.1.0
v=0.1.0 arch=arm64                             # arch=amd64 on an Intel Mac
base=https://github.com/magicdrive/kjak-notify-macos/releases/download/v$v
curl -fLO "$base/kjak-notify-macos_${v}_darwin_${arch}.tar.gz" &&
curl -fLO "$base/kjak-notify-macos_checksums.txt" &&
shasum -a 256 -c --ignore-missing kjak-notify-macos_checksums.txt &&
gh attestation verify "kjak-notify-macos_${v}_darwin_${arch}.tar.gz" --repo magicdrive/kjak-notify-macos &&
tar -xzf "kjak-notify-macos_${v}_darwin_${arch}.tar.gz" kjak-feather-notify-macos &&
install -d -m 755 ~/.local/bin &&
install -m 755 kjak-feather-notify-macos ~/.local/bin/
```

The checksum file comes from the same release as the archive: it shows
the download is intact. `gh attestation verify` shows that the archive
was built by this repository's release workflow (skip it only without
`gh`).

`~/.local/bin` must be in `PATH` (or pin it with `path =`, below). An
archive downloaded with a browser carries the quarantine mark and the
executable is not notarized: `xattr -d com.apple.quarantine
kjak-feather-notify-macos` before installing it; `curl` sets no mark.

### Homebrew

```sh
brew install magicdrive/tap/kjak-notify-macos  # terminal-notifier comes with it
```

The formula `kjak-notify-macos` installs `kjak-feather-notify-macos` into
`$(brew --prefix)/bin` (`/opt/homebrew` on Apple silicon, `/usr/local` on
an Intel Mac) and depends on `terminal-notifier`, which homebrew-core has
no bottle for: Homebrew builds it from source, with Xcode.

Homebrew's `bin`, `Cellar` and `opt` can be written by the `admin` group,
so **kjak v1.2.0 or later** runs the plugin from there only with its
SHA-256 pinned ([kjak's security notes](https://github.com/magicdrive/kjak/blob/v1.2.0/docs/security.md#external-plugins);
kjak v1.1.0 refuses it). `kjak feather plugins` shows the value:

```sh
kjak feather plugins     # notify-macos ... Homebrew: runs only for a feather with sha256 = "..."
shasum -a 256 "$(brew --prefix)/opt/kjak-notify-macos/bin/kjak-feather-notify-macos"   # the same value
```

Put it in the feather as `sha256 = "..."` (Quick start, below), and update
it after each `brew upgrade kjak-notify-macos`: until then kjak refuses the
new version. A project's feather is pinned by `kjak config trust` instead.
Checked on an Intel Mac (`/usr/local`) and, by CI, on GitHub's Apple
silicon and Intel runners (`/opt/homebrew`, `/usr/local`); not on a real
Apple silicon Mac.

### From source

```sh
brew install terminal-notifier                 # 3.1.0
make install                                   # ~/.local/bin/kjak-feather-notify-macos (Go 1.27)
```

## Quick start

```sh
kjak feather plugins                           # lists notify-macos as ok
```

```toml
# ~/.config/kjak/config.toml
[feathers.mac]
type = "external"
plugin = "notify-macos"
# sha256 = "..."                                                      # required with Homebrew (above)
settings = { terminal_notifier = "/opt/homebrew/bin/terminal-notifier" } # /usr/local/bin on an Intel Mac
timeout = "10s"
```

Without `terminal_notifier`, the plugin uses the first
`terminal-notifier` of kjak's `PATH`. It runs terminal-notifier only from
Homebrew's Cellar (`/opt/homebrew` or `/usr/local`), `/Applications` or
`~/Applications` (another prefix: `TERMINAL_NOTIFIER_APP_DIRS`).

```sh
kjak config validate
kjak send --to mac --dry-run "test"            # checks the plugin, runs nothing
kjak send --to mac --title "Build" --level success "All tests passed"
```

The [tutorial](docs/tutorial.en.md) covers permission, Focus,
troubleshooting and uninstalling.

## How it works

kjak runs the plugin with a JSON request on stdin. The plugin runs
`terminal-notifier -title '\<title>' -subtitle '\<level>'` with the body
(and the URL) on terminal-notifier's stdin, then answers kjak on stdout:

| terminal-notifier | kjak reports |
|---|---|
| exit 0 (accepted by the notification center) | `delivered` |
| notifications not allowed (exit 3), the request refused (exit 5 with "Failed to deliver notification"), bad arguments (exit 1, 2) | `failed`, acceptance `rejected` |
| not found, not terminal-notifier, not trusted, not macOS, a timeout below 2s, an empty body: nothing is run | `failed`, acceptance `rejected` |
| no answer from the notification service (exit 4), other exits, stopped at the deadline | `failed`, acceptance `unknown` |

Nothing is retried. The text of the message is data: it never becomes an
option, a shell command or a property list. Only terminal-notifier's own
executable is run, from where it is installed (Homebrew's Cellar,
`/Applications`, `~/Applications`), named directly or through a link or
Homebrew's wrapper script; never another program a setting names
([architecture](docs/design/architecture.md)).

## Development

```sh
make check          # gofmt, go vet, staticcheck, unit tests, race, integration with kjak v1.2.0 and v1.1.0
make fuzz           # FuzzArgs
```

The tests never post a notification: terminal-notifier is a stub (an
application bundle with a shell script as its executable). `make
integration` runs kjak v1.2.0: `KJAK_BIN=/path/to/kjak`, else it builds it
(`go install github.com/magicdrive/kjak@v1.2.0`) into `build/` once; `make
integration-compat` runs the same tests with kjak v1.1.0.

Releases are made with Hephaestus from `hephaestus.yaml`
([releasing](docs/release.md)): `make release-check snapshot`, and `make
homebrew-e2e` for the formula with this Mac's Homebrew.

Documents: [releasing](docs/release.md) · [plugin protocol v1](docs/design/plugin-protocol.md) ·
[architecture](docs/design/architecture.md) ·
[limitations](docs/design/limitations.md) · [audit](docs/audit.md)

## License

MIT ([LICENSE](LICENSE)). terminal-notifier is a separate project under its
own license (MIT) and is not included.
