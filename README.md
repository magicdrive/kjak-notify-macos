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
> - Speaks kjak's plugin protocol v1 (kjak v1.1.0).
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
| supported | macOS, kjak v1.1.0 (plugin protocol v1), terminal-notifier 3.1.0, a logged-in GUI session |
| checked on a real Mac | Intel (x86_64), macOS 15.7.9, terminal-notifier 3.1.0 (Homebrew, built from source), kjak v1.1.0, from a terminal of the logged-in user (2026-10-31): the plugin alone and through kjak, Japanese text and emoji, a two-line body; shown as from terminal-notifier |
| not checked | Apple silicon, other macOS versions, terminals other than iTerm2 |

## Quick start

```sh
brew install terminal-notifier                 # 3.1.0
make install                                   # ~/.local/bin/kjak-feather-notify-macos (Go 1.27)
kjak feather plugins                           # lists notify-macos as ok (~/.local/bin must be in PATH)
```

```toml
# ~/.config/kjak/config.toml
[feathers.mac]
type = "external"
plugin = "notify-macos"
settings = { terminal_notifier = "/usr/local/bin/terminal-notifier" }   # /opt/homebrew/bin on Apple silicon
timeout = "10s"
```

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
make check          # gofmt, go vet, staticcheck, unit tests, race, integration with kjak v1.1.0
make fuzz           # FuzzArgs
```

The tests never post a notification: terminal-notifier is a stub (an
application bundle with a shell script as its executable). `make
integration` runs kjak v1.1.0: `KJAK_BIN=/path/to/kjak`, else it builds it
(`go install github.com/magicdrive/kjak@v1.1.0`) into `build/` once.

Documents: [plugin protocol v1](docs/design/plugin-protocol.md) ·
[architecture](docs/design/architecture.md) ·
[limitations](docs/design/limitations.md) · [audit](docs/audit.md)

## License

MIT ([LICENSE](LICENSE)). terminal-notifier is a separate project under its
own license (MIT) and is not included.
