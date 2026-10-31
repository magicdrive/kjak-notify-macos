# kjak plugin protocol v1, as kjak v1.1.0 implements it

This page records what this plugin relies on. The source of truth is kjak
v1.1.0 (tag `v1.1.0`, commit `5c51124`), read only:

| what | where in kjak v1.1.0 |
|---|---|
| the protocol, the process, discovery | `docs/plugins.md` |
| request and response types, limits, run, response parsing | `internal/feather/external/external.go`: `Request`, `RequestMessage`, `Response`, `run`, `parseResponse`, `rejectedCode`, `MaxStdout`, `MaxStderr`, `DefaultTimeout`, `stopGrace`, `BaseEnv` |
| configuration keys and their checks | `internal/configure/validate_external.go`: `externalProblems`, `ValidPluginName`, `PluginFile`, `pluginEnvDenied`, `MaxPluginSettings`, `MaxPluginSettingLen` |
| message limits | `internal/core/message.go`: `MaxTitleLength` 256 runes, `MaxBodyLength` 16384 runes, `MaxURLLength` 2048 bytes |
| behaviour under test | `internal/feather/external/external_test.go`: `TestPluginV1`, `TestCritical_PluginMisbehavior`, `TestPluginTimeoutAndCancellation`, `TestPluginRetryPolicy`, `TestConcurrentPlugins`, `TestParseResponse`; the test plugin `testdata/plugin/main.go` |

## The feather (configuration)

```toml
[feathers.mac]
type = "external"
plugin = "notify-macos"          # runs kjak-feather-notify-macos
settings = { terminal_notifier = "/usr/local/bin/terminal-notifier" }
timeout = "10s"                  # default 10s; at most kjak's MaxTimeout
# path = "/abs/kjak-feather-notify-macos"   # pin the file (no PATH search)
# sha256 = "..."                            # pin its content
# env = ["NAME"]                            # extra variables (never KJAK_*, LD_*, DYLD_*)
# [feathers.mac.retry]
# policy = "never"                          # default; "plugin" retries rejected + retryable
```

- `plugin`: `^[a-z0-9][a-z0-9_-]{0,47}$`, not a built-in type or a reserved
  name. `macos` is reserved (an OS name), hence `notify-macos`.
- `settings`: plain strings, keys `^[a-z][a-z0-9_]{0,63}$`, at most 64, each
  at most 4 KiB. This plugin knows one: `terminal_notifier`.
- `secrets`: none needed; the request's `secrets` is `{}`.

## Discovery and checks (by kjak, before every run)

`kjak-feather-notify-macos` in the absolute `PATH` directories (empty and
relative elements are not searched), or `path`. The file (links resolved)
must be a regular executable owned by root or the user, not writable by
group or others, in directories with the same properties (ACLs checked on
macOS). Its identity is checked again just before each run.

## The process

| | |
|---|---|
| arguments | none |
| environment | only `PATH` (absolute elements), `HOME`, `TMPDIR`, `TEMP`, `TMP`, `LANG`, `LC_ALL`, `LC_CTYPE`, `TZ`, `SYSTEMROOT`, `USERPROFILE` (those set), the feather's `env`, `KJAK_PLUGIN_PROTOCOL=1` |
| working directory | a new private directory, removed after the attempt |
| stdin | the request (one JSON object), then closed |
| stdout | one JSON object (the response); more than 1 MiB is a protocol error |
| stderr | diagnostics; kjak keeps 64 KiB and puts the last 300 characters (cleaned, secrets redacted) in a failure's message |
| timeout | the feather's `timeout`, and the send's deadline: kjak kills the plugin's **process group** then (and after it exits) |
| concurrency | different feathers run at once; one feather's attempts never overlap |
| dry run | `kjak send --dry-run` finds and checks the plugin; it does **not** run it |

## Request

```json
{"protocol_version": 1, "feather": "mac", "plugin": "notify-macos", "attempt": 1,
 "deadline": "2026-10-31T09:00:10Z", "timeout_ms": 10000,
 "idempotency_key": "4f1c...", "message": {"title": "Build", "body": "ok", "level": "success", "url": ""},
 "settings": {"terminal_notifier": "/usr/local/bin/terminal-notifier"}, "secrets": {}}
```

`message.level` is one of `info`, `success`, `warning`, `error`, `critical`;
unset fields are empty strings (kjak requires a body). New request fields may
be added in v1: they are ignored.

## Response, exit status, classification

| the plugin writes / does | kjak reports |
|---|---|
| `{"protocol_version":1,"status":"delivered"}` (acceptance `confirmed` by default), exit 0 | delivered, confirmed |
| `"status":"failed","acceptance":"rejected"` | failed, rejected; `error.code` one of `rate_limited`, `server_error`, `network`, `protocol`, `internal`, else `rejected`; retried only with `retry.policy = "plugin"` and `"retryable": true` |
| `"status":"failed"` with acceptance `unknown` or none | failed, unknown (`ambiguous`), never retried |
| exit status other than 0, a signal | failed, unknown, whatever stdout holds |
| no response, not one JSON object, missing or other `protocol_version` | failed, unknown (`protocol`) |
| killed at the timeout or the deadline | failed, unknown |
| cannot be started | failed, rejected, not retried |

## What this plugin does with it

- Reads at most 4 MiB of stdin (kjak's own limits keep a request far
  smaller); a malformed request, another `protocol_version` or no `message`
  is answered `failed`/`rejected` with `error.code = "protocol"` (nothing
  was posted).
- Never sets `retryable`: with any retry policy, kjak does not retry it.
- Stops terminal-notifier 1 s before the request's `deadline` and answers
  `unknown`, so that its own answer arrives before kjak kills it; with less
  than 1 s left for it (a feather `timeout` below 2 s), it does not start it
  and answers `rejected`.
- Writes exactly one JSON line on stdout and exits 0. A failure's reason,
  with the first line of terminal-notifier's stderr, is in `error.message`
  only; nothing is written on stderr (kjak appends stderr to the message).
