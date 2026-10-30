# Limitations

kjak-feather-notify-macos is **experimental**.

## What `delivered` means

`delivered` (acceptance `confirmed`) means that terminal-notifier exited 0:
the macOS notification center **accepted** the notification. It does not mean
that it was **shown**, nor that anyone **saw** it. It can stay off the
screen:

- Focus or Do Not Disturb (macOS may still put it in Notification Center);
- the notification settings of terminal-notifier (System Settings,
  Notifications): alerts off, banners off, or notifications off — the last
  one is reported (`rejected`), the others are not;
- a locked screen, a screen saver, another user's session on the screen;
- a Mac asleep: notifications sent then were accepted (`delivered`) and
  not seen (observed on the owner's Mac).

`-ignoreDnD` is not used: terminal-notifier 3.1.0 documents it as a no-op
without an Apple entitlement.

## Environment

| | |
|---|---|
| operating system | macOS only; elsewhere every send is `rejected` |
| supported | macOS with terminal-notifier 3.1.0 in a logged-in GUI session, kjak v1.1.0 (plugin protocol v1) |
| checked on a real Mac | Intel (x86_64), macOS 15.7.9, terminal-notifier 3.1.0 (Homebrew, built from source), kjak v1.1.0, from a terminal of the logged-in user (owner, 2026-10-31): the plugin alone, through kjak, Japanese text and emoji, a two-line body with the URL; no process left; shown as from terminal-notifier |
| not checked | Apple silicon, other macOS versions, other terminals, Terminal.app, launchd jobs, SSH sessions |

## No GUI session

Over SSH, from a launchd daemon or as another user, the notification service
may not answer: terminal-notifier waits 10 s and exits 4, reported as
`unknown`. The plugin stops terminal-notifier 1 s before the feather's
`timeout`, so that message is seen only with a `timeout` of 12 s or more;
with the default 10 s the answer is the plugin's own deadline (`unknown` as
well).

## Timeout

The feather's `timeout` must be at least 2 s: terminal-notifier is stopped
1 s before it, and is not started with less than 1 s left (the answer is
then `rejected`, with that reason). 10 s (kjak's default) is recommended.

## Permission

The first notification of a new terminal-notifier install asks for
permission and waits for an answer. kjak's timeout (10 s by default) stops
it first: grant the permission once by running terminal-notifier yourself
(tutorial, "Notification permission").

## Identity

Notifications are shown as from terminal-notifier
(`fr.julienxx.oss.terminal-notifier`), with its icon. `-sender` and
`-appIcon` are gone from terminal-notifier 3.x; this plugin does not build an
application bundle of its own.

## Other

- No sound, no action, no click handling, no grouping, no scheduling.
- Never retried: a duplicate notification is worse than a missing one.
- Only terminal-notifier's own executable is run, from where it is
  installed (Homebrew's Cellar, `/Applications`, `~/Applications`, or
  `$TERMINAL_NOTIFIER_APP_DIRS`), and its files and the directories above
  them must belong to root or you and not be writable by others
  ([architecture](architecture.md)). macOS ACLs are not read (kjak
  reads them for the plugin itself): an ACL that lets someone else write
  there is not noticed.
- terminal-notifier reads options it is not given from its `defaults`
  (`defaults write fr.julienxx.oss.terminal-notifier message ...`, `execute`,
  `open`, `in`, ...) and the global domain. Such a value changes what it
  does (an ignored body, a click that runs a command, a notification
  scheduled for later but reported `delivered`). Only you can write them;
  none is set by default. `defaults read fr.julienxx.oss.terminal-notifier`
  shows them.
- If you click an older terminal-notifier notification while this plugin's
  terminal-notifier runs, macOS may hand the click to it, and it may exit
  before posting (exit 0, reported `delivered`) or report that click's
  failure (exit 5 without "Failed to deliver notification": `unknown`). The
  window is short.
- Homebrew treats Intel Macs as Tier 3: a bottle may be missing and
  terminal-notifier built from source (Xcode needed). That is Homebrew's
  support level, not an incompatibility: terminal-notifier 3.1.0 was built
  and posted notifications on an Intel Mac.
- Experimental: settings and messages of the plugin may change.
