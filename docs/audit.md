# Audit

Independent audits of the first version (2026-10-31), each by a separate
reviewer reading the code, kjak v1.1.0 and terminal-notifier 3.1.0's source,
and running experiments with a stub terminal-notifier (no notification was
posted). Findings, severity and what was done.

| audit | scope | Critical | High | Medium | Low |
|---|---|---|---|---|---|
| A Protocol, C Reliability | protocol v1, classification, exit status, retry, timeout; failures, cancellation, leftover processes, duplicates, no GUI session | 0 | 0 | 1 | 4 |
| B Security | injection, PATH hijacking, arguments, secrets, logs, temporary files, processes | 0 | 0 | 2 | 4 |
| D Test, E Documentation | tests, stubs, false positives, isolation of real notifications; configuration, commands, experimental status, consistency | 0 | 0 | 2 | 10 |

## Medium

| # | finding | resolution |
|---|---|---|
| A-M1 | with a `timeout` of about 1 s or less terminal-notifier got 100 ms and the answer was almost always `unknown`, even past the deadline | terminal-notifier is not started with less than 1 s left: `rejected`, "give the feather a timeout of at least 2s"; documented (README, tutorials, limitations). Tests: `TestTimeoutTooShort`, `TestRunDeadline`, integration `TestOutcomes` |
| B-M1 | the `terminal_notifier` setting accepted any root-owned program: `/usr/bin/tclsh` ran the body as a script (a project's `.kjak.toml` chooses settings) | only terminal-notifier's application executable runs: file name `terminal-notifier`, real path `.../terminal-notifier.app/Contents/MacOS/terminal-notifier`, bundle identifier `fr.julienxx.oss.terminal-notifier`; Homebrew's wrapper only in its exact form, and its target is run without bash. Tests: `TestBackendIdentity` |
| B-M2 | the `PATH` directory holding the name, and a link's owner, were not checked: a writable directory could hold `terminal-notifier -> /usr/bin/tclsh` | the directory of the name and every directory above it, and the link's owner, are checked; B-M1 refuses such a link anyway. Tests: `TestPathDirectoryChecked`, `TestTrustedMode` |
| D-1 | denied permission was reported with terminal-notifier's last stderr line (`tccutil reset ...`), not its reason; the stub printed one line | the reason is the first line of terminal-notifier's stderr; the stubs print terminal-notifier's own lines (exit 3, 4). Tests: `TestExitStatuses` |
| D-2 | README linked a missing `docs/audit.md` | this file; `docs/links_test.go` checks every relative link |

## Low

| # | finding | resolution |
|---|---|---|
| A-L1, D-3 | a body empty after cleaning (U+FEFF only) made terminal-notifier print its help and exit 1, reported `internal`; a test expected `delivered` for an empty body | refused before running ("no body"); the stub, like terminal-notifier, exits 1 on an empty stdin. `TestEmptyBody` |
| A-L2 | exit 5 is also the end of handling a click on an older notification | `rejected` only with "Failed to deliver notification" on stderr, else `unknown`; documented |
| A-L3, D-10 | kjak's report repeated the plugin's sentence (response and stderr) | the plugin writes nothing on stderr; integration `TestOutcomes` checks the reason appears once |
| A-L4, B-L3 | terminal-notifier reads options not passed from its `defaults` domain | documented (limitations, architecture): only the user can write them |
| B-L1 | ancestors and ACLs were not checked; Homebrew's Cellar is admin-writable | ancestors are checked (group write allowed only for wheel and admin); ACLs are not read: documented |
| B-L2 | the child inherited the plugin's environment (`BASH_ENV` read by the wrapper's bash) | bash is never run (B-M1), and terminal-notifier gets a fixed small environment. `TestChildEnvironment` |
| B-L4 | bidirectional controls and zero-width characters passed | they become spaces (joiners stay for emoji). `TestCompose` |
| D-4 | mutants survived: no link-resolution, no uid check, no output cap, no U+2028 | `TestBackendIdentity` (a link to another program), `TestTrustedMode` (uid), `TestBackendOutputIsolated` (cap), `TestCompose` (U+2028, U+2029) |
| D-5 | stub children (`sleep`) outlived the tests | `exec sleep` in the stubs |
| D-6 | `/tmp/x` was logged, not checked | a marker in the test's own directory, checked |
| D-7 | integration gaps: the `PATH` search, exit 4 and 5, signals, unknown settings, concurrency; a weak `feather plugins` check | `TestFoundOnPath`, `TestOutcomes`, `TestConcurrentFeathers`; `feather plugins -o json` checked field by field |
| D-8 | "a timeout above 10 s" to see exit 4 | "12 s or more" |
| D-9 | README omitted staticcheck; `KJAK_BIN` of a new path was built elsewhere | fixed (`GOBIN` is `KJAK_BIN`'s directory) |
| D-11 | incomplete list of options matched as whole argv strings | completed (architecture) |
| D-12 | "not in an absolute PATH directory" listed as "refused" by `kjak feather plugins` | separate rows: a problem vs not listed / skipped elements |

## Confirmed without change

- Protocol v1: request read with a 4 MiB limit, one JSON value, unknown
  fields ignored; one response line that kjak's `parseResponse` accepts;
  `retryable` never set; exit status 0.
- Injection: every value given to terminal-notifier starts with `\`, which
  its `NSUserDefaults` reading removes, so no value becomes an option or a
  property list (checked with a program using the same code); the body is
  standard input; no shell is involved.
- Processes: Homebrew's wrapper `exec`s (one process); kjak kills the
  plugin's process group; the plugin stops terminal-notifier before kjak's
  deadline.
- Tests never call the real terminal-notifier.

## Regression

After the fixes: `make check` (gofmt, go vet, staticcheck, unit tests, race,
integration with kjak v1.1.0), `make fuzz`, and the resolution of the
owner's real install (`/usr/local/bin/terminal-notifier`, resolved to the
Cellar's application executable, nothing run).

## Re-audit

An independent re-audit of the fixes (attempts to bypass B-M1 and B-M2,
false refusals, mutation of the new tests). Critical 0, High 0, Medium 1,
Low 6.

| # | finding | resolution |
|---|---|---|
| F1 Medium | a look-alike `terminal-notifier.app` owned by the user (a repository's own files, a project's `terminal_notifier` setting) passed: the identifier is only a string | the application must be where terminal-notifier is installed: Homebrew's Cellar, `/Applications`, `~/Applications`, or `$TERMINAL_NOTIFIER_APP_DIRS` (the user's environment, not the configuration). `TestAppDirs`, integration `TestMinimumTimeoutAndAppDirs` |
| F2 Low | `timeout = "2s"` was always refused (the plugin's start took the margin) | at least 500 ms for terminal-notifier (2s documented); integration test with `2s` |
| F3 Low | the wrapper and `Info.plist` were read before their size was checked; a FIFO `Info.plist` blocked | read only as regular files of at most 4 KiB / 1 MiB. `TestReadSmall` |
| F4 Low | surviving mutants (wrapper checks, `$` in the wrapper, its size, ancestors) | `TestReadSmall`, `TestWrapperTargetChecked`, `TestUntrustedBackend` (ancestors) |
| F5 Low | `TestConcurrentFeathers` failed when packages ran in parallel | longer stub, bound 3.5 s for 2 s runs; passed 3 runs in parallel |
| F6 Low | an admin-writable file (an application dragged into `/Applications`) and a binary `Info.plist` were refused | group write by wheel or admin allowed for files too; binary property lists read |
| F7 Low | this section was missing; a missing directory gave a bare `stat` error | this section; "cannot check DIR: ..." and a troubleshooting row |

Confirmed: `..` in paths, link chains, the check-then-run window (only
directories of root, the user or admin are trusted), the wrapper pattern
(CRLF, spaces, `$`, size), case and Unicode (refused, safe side), the real
install (resolved, not run).

## Real-device check (owner, 2026-10-31)

Intel (x86_64), macOS 15.7.9, terminal-notifier 3.1.0 from Homebrew (built
from source), kjak v1.1.0, an isolated kjak configuration, run from a
terminal of the logged-in user, with the owner's approval.

| step | reported | on the screen |
|---|---|---|
| the plugin alone | `delivered`, exit 0 | shown |
| through kjak | `ok mac (external) delivered`, exit 0 | shown |
| Japanese, emoji, two-line body, URL | `success`, `delivered` | shown, not garbled |
| processes left | none | |
| shown as from | | terminal-notifier |

A first run while the Mac was asleep reported the same `delivered` results,
and nothing was seen: `delivered` is the notification center's acceptance.
