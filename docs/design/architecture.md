# Architecture

`kjak-feather-notify-macos` connects two things that already work: kjak's
external feather (protocol v1, [plugin-protocol.md](plugin-protocol.md)) and
terminal-notifier 3.1.0, which the owner has seen post notifications on an
Intel Mac. It implements no notification code of its own.

```
kjak send --to mac  ──stdin: request JSON──▶  kjak-feather-notify-macos
                     ◀─stdout: response JSON──      │
                                                    │ argv: -title \<title> -subtitle \<level>
                                                    │ stdin: body (+ URL)
                                                    ▼
                                            terminal-notifier ──▶ macOS notification center
```

## Language: Go (standard library only)

| | POSIX shell | Go |
|---|---|---|
| reading the request | needs a JSON parser (`jq`, an extra dependency) or a hand-made one (unsafe for strings with escapes) | `encoding/json` |
| writing the response | escaping by hand | `encoding/json` |
| arguments to terminal-notifier | safe with `"$@"`, but the values come out of a parser first | an argument vector, no shell |
| Unicode, invalid UTF-8 | depends on the tools and the locale | explicit (`strings.ToValidUTF8`) |
| timeout before kjak's deadline | `timeout` is not on macOS; background jobs and `kill` | `exec.CommandContext`, `WaitDelay` |
| tests | shell stubs, fragile | unit, race, fuzz, integration with kjak |
| distribution | a script | one static binary, `go build` |

Go wins on the criteria in order (compatibility, security, maintenance,
simplicity, tests). No third-party module is used.

Layout: `cmd/kjak-feather-notify-macos` (main), `internal/plugin`
(`plugin.go`: request, response, text; `backend.go`: finding and running
terminal-notifier), `tests/` (integration with kjak v1.1.0).

## terminal-notifier 3.1.0

Read in its source (tag `3.1.0`, the tarball Homebrew builds,
`Terminal Notifier/AppDelegate.m`):

- Options are read through `NSUserDefaults` (`-key value` pairs of argv).
  A value that starts with `-` is read as the next key (a title `-execute`
  is lost, and the next argument would be taken as a value), and a value
  that starts with `(`, `{`, `<` or a quote is parsed as a property list.
  Its `objectForKeyedSubscript:` removes **one leading backslash** from every
  value. So every value is passed with a leading `\`: it then never starts
  with `-` and is never parsed, and arrives as the exact string. Checked
  with a program using the same `NSUserDefaults` code (no notification).
- Some options are looked for in argv as whole strings (`-help`,
  `-version`, `-diagnose`, `--diagnose`, `-message`, `-reply`, `-action`,
  `-ignoreDnD`): no value can equal them, since every value starts with
  `\`.
- Options this plugin does not pass (`message`, `execute`, `open`, `group`,
  `in`, ...) are still read by `NSUserDefaults` from terminal-notifier's own
  domain (`defaults write fr.julienxx.oss.terminal-notifier ...`) and the
  global one. Only the user can write those; none is set by default
  ([limitations](limitations.md)).
- Without `-message`, and when stdin is not a terminal, the message is read
  from stdin as UTF-8 (trailing line breaks removed). The body is passed this
  way: it is never an argument at all.
- The title defaults to "Terminal" when not given; this plugin always gives
  one (`kjak` when the message has none). The level is the subtitle.
- Exit statuses: 0 the request was added to the notification center; 1
  usage; 2 bad argument; 3 not authorized (denied, or permission declined);
  4 its 10 s watchdog (no answer from the notification service: usually no
  GUI session); 5 the notification center refused the request (it then
  prints "Failed to deliver notification") — or the handling of a click on an
  older notification, which macOS may deliver to this process, failed; 6 no
  response (only with `-action`/`-reply`, never used here).
- An empty message (stdin) makes it print its help and exit 1: the plugin
  refuses an empty body before running it.
- The first notification of a new install asks for permission and waits for
  the user (its watchdog stands down meanwhile).
- `-diagnose` reports the authorization state without posting.
- Homebrew installs it as `bin/terminal-notifier`, a bash script that only
  does `exec "<Cellar>/terminal-notifier.app/Contents/MacOS/terminal-notifier" "$@"`:
  the arguments and stdin pass through unchanged, and no process stays in
  between. The app is ad-hoc signed, bundle identifier
  `fr.julienxx.oss.terminal-notifier`.
- `-execute`, `-open`, `-activate`, `-contentImage`, `-sound`, `-group`,
  `-in`, `-at`, `-action`, `-reply` are never passed: a click does nothing
  but what macOS does by default.

## Mapping

| terminal-notifier | response | why |
|---|---|---|
| exit 0 | `delivered` (`confirmed`) | the notification center accepted it. Not "shown", not "seen" |
| exit 3 | `failed`, `rejected`, `rejected` | not authorized: nothing posted |
| exit 1, 2 | `failed`, `rejected`, `internal` | refused before posting: a bug here |
| exit 5 with "Failed to deliver notification" | `failed`, `rejected`, `rejected` | the notification center refused it |
| exit 5 without it | `failed`, `unknown` | a click on an older notification was handled: this one may be posted |
| exit 4 | `failed`, `unknown` | no answer: it may have been posted |
| other exit, a signal | `failed`, `unknown` | it ran: it may have posted |
| stopped at the deadline | `failed`, `unknown` | it may have posted |
| cannot be started | `failed`, `rejected`, `rejected` | nothing ran |
| not macOS, a bad setting, no trusted terminal-notifier, an empty body, a timeout below 2 s | `failed`, `rejected`, `rejected` | nothing ran |
| malformed request | `failed`, `rejected`, `protocol` | nothing ran |

`retryable` is never set, so kjak never retries: a second attempt would most
likely meet the same machine, and a duplicate notification is worse than a
missing one.

## Finding terminal-notifier

Only terminal-notifier's own executable is run: a file named
`terminal-notifier` whose real path (links resolved) is
`<dir>/terminal-notifier.app/Contents/MacOS/terminal-notifier`, in a bundle
whose `Info.plist` (XML or binary) has the identifier
`fr.julienxx.oss.terminal-notifier`, where `<dir>` is where
terminal-notifier is installed: Homebrew's
`{/usr/local,/opt/homebrew}/Cellar/terminal-notifier/<version>`,
`/Applications`, `~/Applications`, or a directory of
`$TERMINAL_NOTIFIER_APP_DIRS` (absolute, colon-separated; from the user's
environment, which a configuration file cannot set — the feather must list
it in `env` for kjak to pass it).
The name comes from the `terminal_notifier` setting (an absolute path, file
name `terminal-notifier`), else from the absolute directories of `PATH`
(empty and relative elements are skipped). Homebrew's
`bin/terminal-notifier` is a script; it is accepted only in its exact form
(`#!/bin/bash`, `exec "<...>/terminal-notifier.app/Contents/MacOS/terminal-notifier" "$@"`),
and the executable it names is run directly: bash is never started.

Why: a project's `.kjak.toml` can register this plugin with settings of the
repository's choosing (kjak's trust shows them and pins the plugin, not the
files a setting names). Without these rules, `terminal_notifier =
"/usr/bin/tclsh"` would run the message's body as a Tcl script, and a
look-alike `terminal-notifier.app` among the repository's files would run
the repository's code. The bundle's identifier is a string anyone can
write, and terminal-notifier is only ad-hoc signed: the install location is
what tells the real one.

Ownership: every file involved (the name, a link, the wrapper, the
executable) and every directory that holds one, and every directory above
it, must be owned by root or the user, not writable by others (a sticky
directory may be: `/tmp`), and writable by its group only when the group is
wheel or admin (Homebrew on Intel makes `/usr/local/Cellar` writable by
admin; an application dragged into `/Applications` is admin-writable; admin
members can become root anyway). The wrapper and `Info.plist` are read only
when they are regular files of at most 4 KiB and 1 MiB. ACLs are not read:
a macOS ACL that lets someone else write there is not noticed
([limitations](limitations.md)).

terminal-notifier runs with a small environment: `HOME`, `TMPDIR`, `LANG`,
`LC_ALL`, `LC_CTYPE`, `TZ` (when set) and `PATH=/usr/bin:/bin:/usr/sbin:/sbin`;
nothing else of the plugin's (no `BASH_ENV`, no `DYLD_*`).

## Text

- Title and level: one line; control characters and line breaks become
  spaces; invalid UTF-8 becomes U+FFFD.
- Body: line breaks kept (CRLF and CR become LF), tabs kept, other control
  characters become spaces; then the URL on its own line. A body that is
  empty after that is refused (terminal-notifier would print its help).
- Both: the characters that make text read otherwise than it is become
  spaces too: bidirectional controls (U+202A–U+202E, U+2066–U+2069, U+200E,
  U+200F, U+061C), the zero-width space U+200B and the byte order mark
  U+FEFF. Joiners (U+200C, U+200D) stay: emoji sequences use them.
- kjak's limits (title 256, body 16384 characters) apply before the plugin;
  macOS shortens what it shows.

## Time

terminal-notifier is stopped 1 s before the request's `deadline`. If less
than 1 s would be left for it, it is not started: the answer is `rejected`
("give the feather a timeout of at least 2s"). `WaitDelay` (250 ms) bounds
the wait for its pipes. The answer is then written before kjak's own kill.
If kjak stops the plugin first (Ctrl-C, the send's deadline), it kills the
plugin's process group, terminal-notifier included, and reports `unknown`
itself.

## Output

stdout carries the response only (one JSON line). terminal-notifier's
stdout is read into a bounded buffer (64 KiB) and dropped. Its stderr
(bounded likewise) gives its first line — its reason; the next lines are
advice — to a failure's `error.message`. The plugin writes nothing on
stderr (kjak adds stderr to the message: it would repeat). It receives no
secret and logs nothing of the request.
