# Tutorial: macOS notifications from kjak (experimental)

[日本語](tutorial.ja.md) · [README](../README.md) · [Limitations](design/limitations.md)

## 1. What this is

`kjak-feather-notify-macos` lets `kjak send` show a macOS notification. It is
a kjak external feather plugin (protocol v1) that runs terminal-notifier:
kjak gives the plugin the message, the plugin gives it to terminal-notifier,
and terminal-notifier posts it to the macOS notification center.

## 2. Experimental

- It is not part of kjak and not a built-in feather; it has its own
  releases.
- Its settings and messages may change while it is experimental.
- `delivered` means that macOS accepted the notification, not that it was
  shown or seen (section 12).

## 3. Supported environment

| | |
|---|---|
| supported | macOS, in a logged-in GUI session; kjak v1.1.0; terminal-notifier 3.1.0 |
| checked on a real Mac | Intel (x86_64), macOS 15.7.9, terminal-notifier 3.1.0, kjak v1.1.0, from a terminal of the logged-in user: the steps of section 9 and 10 (shown as from terminal-notifier) |
| not checked | Apple silicon, other macOS versions, other terminals, SSH, launchd |

## 4. Dependencies

- kjak v1.1.0 (`kjak --version`)
- terminal-notifier 3.1.0 (`terminal-notifier -version`)
- Go 1.27, to build the plugin (`go version`)

## 5. Install terminal-notifier

```sh
brew install terminal-notifier
terminal-notifier -version        # terminal-notifier 3.1.0
command -v terminal-notifier      # /usr/local/bin (Intel) or /opt/homebrew/bin (Apple silicon)
```

On an Intel Mac, Homebrew (which supports Intel Macs at its Tier 3) may have
no bottle and build terminal-notifier from source, which needs a working
Xcode. If the build fails while loading an Xcode framework (for example
`DVTDownloads.framework`), finish Xcode's first launch once:

```sh
sudo xcodebuild -runFirstLaunch
xcodebuild -checkFirstLaunchStatus; echo $?    # 0
```

This is Homebrew's support level, not an incompatibility: terminal-notifier
3.1.0 has been built and has posted notifications on an Intel Mac.

## 6. Build and install the plugin

```sh
git clone https://github.com/magicdrive/kjak-notify-macos   # or your copy
cd kjak-notify-macos
make install                      # builds, installs ~/.local/bin/kjak-feather-notify-macos
```

`make install PREFIX=/some/dir` installs into `/some/dir/bin`. The directory
must be in your `PATH` as an absolute path, and neither it nor the file may
be writable by group or others: kjak checks it before every run.

```sh
kjak feather plugins              # notify-macos ... ok
```

## 7. Register it with kjak

kjak has no command that adds an external feather: write it into
`~/.config/kjak/config.toml` (`kjak config path` shows the file).

## 8. Configuration

```toml
# ~/.config/kjak/config.toml
[feathers.mac]
type = "external"
plugin = "notify-macos"                 # runs kjak-feather-notify-macos
settings = { terminal_notifier = "/usr/local/bin/terminal-notifier" }
timeout = "10s"
```

| key | |
|---|---|
| `plugin` | `notify-macos` (`macos` alone is a name kjak reserves) |
| `settings.terminal_notifier` | the absolute path of terminal-notifier (`command -v terminal-notifier`): Homebrew's link or script, or the executable inside `terminal-notifier.app`. Without it, the plugin uses the first `terminal-notifier` of the absolute directories of `PATH`. Only terminal-notifier's own executable is ever run, from Homebrew's Cellar, `/Applications` or `~/Applications` (another directory: list it in `$TERMINAL_NOTIFIER_APP_DIRS` and add `env = ["TERMINAL_NOTIFIER_APP_DIRS"]` to the feather); it is refused if one of its files or directories can be changed by another user |
| `timeout` | how long kjak lets the plugin run: at least 2s, 10s (the default) recommended. The plugin stops terminal-notifier 1 second before and reports `unknown` |
| `path`, `sha256` | optional: pin the plugin file (kjak's own keys) |

No other setting exists; an unknown one is refused. The plugin needs no
secret. Several destinations at once:

```toml
[groups.everywhere]
feathers = ["mac", "slack"]
```

Check it:

```sh
kjak config validate
kjak send --to mac --dry-run "test"     # finds and checks the plugin, runs nothing
```

## 9. Test notification

**This shows a notification on your screen.** First the plugin alone, with
a request as kjak writes it:

```sh
printf '%s' '{"protocol_version":1,"message":{"title":"kjak-notify-macos","body":"plugin test","level":"info","url":""},"settings":{"terminal_notifier":"/usr/local/bin/terminal-notifier"}}' \
  | kjak-feather-notify-macos; echo "exit $?"
```

Expected: a notification "kjak-notify-macos" / "info" / "plugin test", and
on stdout `{"protocol_version":1,"status":"delivered","acceptance":"confirmed"}`
then `exit 0`. Then through kjak:

```sh
kjak send --to mac --title "kjak" --level success "Hello from kjak"
kjak send --to mac -o json "Hello again"        # results[0].status, .acceptance
```

Expected: `ok    mac (external) delivered ...`, exit 0, and a notification.
Check on the screen: kjak's exit status tells only what macOS accepted.

## 10. Japanese text

```sh
kjak send --to mac --title "ビルド完了 😀" --level success "日本語の本文
二行目もそのまま表示されます"
```

Title, level and body reach the notification as written: the title and the
level on one line each, the body with its line breaks. Control characters
become spaces.

## 11. Notification permission

The first notification of a new terminal-notifier install asks for
permission and waits for your answer; kjak's timeout would stop it first.
Grant it once, yourself:

```sh
terminal-notifier -title "kjak-notify-macos" -message "permission test"
terminal-notifier -diagnose       # the authorization state; posts nothing
```

If kjak reports `notifications are not allowed for terminal-notifier
(System Settings, Notifications): Notifications are turned off for this
application.`, turn them on in **System Settings → Notifications →
terminal-notifier** (Allow notifications; Banners or Alerts).

terminal-notifier also suggests resetting the permission so that macOS asks
again:

```sh
tccutil reset UserNotification fr.julienxx.oss.terminal-notifier
```

This **erases** the permission you gave terminal-notifier (for every program
that uses it). Run it only when you want macOS to ask again; installing the
plugin never runs it.

## 12. Focus and Do Not Disturb

Focus (Do Not Disturb) can keep a notification off the screen; it is then
usually in Notification Center. kjak still reports `delivered`: macOS
accepted it. terminal-notifier's `-ignoreDnD` is not used (it needs an Apple
entitlement and is usually a no-op). Allow terminal-notifier in the Focus
settings if it must break through.

## 13. Troubleshooting

| kjak says | why | what to do |
|---|---|---|
| `delivered`, but nothing on the screen | Focus, the notification style (None), a locked screen | Notification Center; System Settings, Notifications, terminal-notifier |
| `notifications are not allowed for terminal-notifier ...` | permission denied | section 11 |
| `terminal-notifier was not found in the absolute directories of PATH` | not installed, or not in the plugin's `PATH` | `brew install terminal-notifier`, or set `terminal_notifier` |
| `setting terminal_notifier must be an absolute path` | a relative path | `command -v terminal-notifier` |
| `no terminal-notifier at ...` | the setting names a file that is not there | `command -v terminal-notifier` |
| `... is not terminal-notifier ...`, `must name a file called terminal-notifier` | the setting names another program | the path of terminal-notifier |
| `... is writable by others` / `by its group` / `belongs to another user: it is not run` | a file or a directory of the install can be changed by another user | `chmod go-w`, or reinstall |
| `too little time left to run terminal-notifier: give the feather a timeout of at least 2s` | `timeout` below 2s | `timeout = "10s"` |
| `... is not in a place terminal-notifier is installed to ...` | terminal-notifier.app is somewhere else | install it with Homebrew, or into `/Applications`; or `$TERMINAL_NOTIFIER_APP_DIRS` (section 8) |
| `cannot check ...: no such file or directory` | a directory of the setting's path does not exist | `command -v terminal-notifier` |
| `the notification service did not answer (no GUI session?)` (acceptance unknown) | over SSH, from launchd, another user | run kjak in your desktop session |
| `terminal-notifier did not finish before the deadline` (unknown) | waiting for permission, a slow system | grant the permission (section 11); a longer `timeout` |
| `unknown setting "..."` | a setting this plugin does not know | only `terminal_notifier` exists |
| `notify-macos runs on macOS only` | another OS | use another feather there |
| `kjak feather plugins` shows a problem for notify-macos | the plugin's file or directory can be changed by others | section 6 |
| `kjak feather plugins` does not list notify-macos (or lists its directory under `skipped_path_elements`) | its directory is not in `PATH`, or only as a relative path | add the absolute directory to `PATH`, or set `path` in the feather |

An `unknown` result is never retried: the notification may already be on the
screen.

## 14. Uninstall

```sh
make uninstall                    # removes ~/.local/bin/kjak-feather-notify-macos
```

Remove `[feathers.mac]` (and its group entries) from the configuration. Keep
terminal-notifier, or `brew uninstall terminal-notifier`. Its notification
permission stays in System Settings until you remove it there.

## 15. Known limitations

See [limitations](design/limitations.md): macOS only; a GUI session; shown as
terminal-notifier (its name and icon); no sound, action or click handling;
never retried; `delivered` is not "seen"; experimental.
