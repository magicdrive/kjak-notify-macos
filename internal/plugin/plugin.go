// Package plugin is kjak-feather-notify-macos: a kjak external feather
// plugin (protocol v1, docs/design/plugin-protocol.md) that posts a macOS
// notification by running terminal-notifier.
//
// It reads one request on standard input, runs terminal-notifier once and
// writes one response on standard output, then exits 0. Standard output
// carries the response only; diagnostics go to standard error. The text of
// the message is data: the title and the level reach terminal-notifier as
// argument values that can never be read as an option, and the body on its
// standard input (docs/design/architecture.md).
//
// "delivered" means that the macOS notification center accepted the
// notification, not that it was shown or seen.
package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// ProtocolVersion is the version of kjak's plugin protocol spoken here.
const ProtocolVersion = 1

const (
	// MaxRequest bounds what is read from standard input. kjak's own limits
	// (a 16384-character body, 64 settings of 4 KiB, 32 secrets) keep a
	// request far below it.
	MaxRequest = 4 << 20
	// defaultTimeout is kjak's default timeout of an external feather.
	defaultTimeout = 10 * time.Second
	// margin: terminal-notifier is stopped this long before kjak's deadline,
	// so that the response is written before kjak stops the plugin (it
	// covers waitDelay, for a child that keeps the pipes open).
	margin = time.Second
	// minRun is the least time terminal-notifier is given (it usually takes
	// a few hundred milliseconds): with less, it is not run. The documented
	// least timeout is 2s (minTimeout): minRun leaves room for the time kjak
	// and the plugin take to start.
	minRun     = 500 * time.Millisecond
	minTimeout = 2 * time.Second
	// fallbackTitle is the title of a message that has none.
	fallbackTitle = "kjak"
	// SettingBackend names the terminal-notifier executable (an absolute
	// path); without it, the absolute PATH directories are searched.
	SettingBackend = "terminal_notifier"
)

// Request is the request of protocol v1. Fields this plugin does not use
// (feather, plugin, idempotency_key, ...) are not decoded; unknown fields are
// ignored, as the protocol requires.
type Request struct {
	ProtocolVersion *int              `json:"protocol_version"`
	Attempt         int               `json:"attempt"`
	Deadline        time.Time         `json:"deadline"`
	TimeoutMS       int64             `json:"timeout_ms"`
	Message         *Message          `json:"message"`
	Settings        map[string]string `json:"settings"`
}

// Message is the message of a request.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	Level string `json:"level"`
	URL   string `json:"url"`
}

// Response is the response of protocol v1.
type Response struct {
	ProtocolVersion int    `json:"protocol_version"`
	Status          string `json:"status"`
	Acceptance      string `json:"acceptance"`
	Error           *Error `json:"error,omitempty"`
}

// Error is the error of a failed response.
type Error struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message"`
}

// Env is what the plugin takes from its environment; tests replace it.
type Env struct {
	GOOS   string
	Getenv func(string) string
	Now    func() time.Time
}

// Main runs the plugin with the process's environment and returns its exit
// status (always 0: the response tells kjak the outcome).
func Main(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer) int {
	return Run(ctx, stdin, stdout, stderr, Env{GOOS: runtime.GOOS, Getenv: os.Getenv, Now: time.Now})
}

// Run reads one request, delivers it and writes one response.
//
// A failure is told in the response only (kjak adds the plugin's stderr to
// it: the same text twice would only repeat).
func Run(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, env Env) int {
	resp := handle(ctx, stdin, env)
	if err := json.NewEncoder(stdout).Encode(resp); err != nil {
		fmt.Fprintf(stderr, "kjak-feather-notify-macos: cannot write the response: %v\n", err)
	}
	return 0
}

// rejected: certainly not posted (nothing ran, or terminal-notifier refused
// before posting). Never retryable: a retry would meet the same machine.
func rejected(code, format string, a ...any) Response {
	return Response{ProtocolVersion: ProtocolVersion, Status: "failed", Acceptance: "rejected", Error: &Error{Code: code, Message: fmt.Sprintf(format, a...)}}
}

// unknown: terminal-notifier ran and may have posted the notification; kjak
// never retries it.
func unknown(format string, a ...any) Response {
	return Response{ProtocolVersion: ProtocolVersion, Status: "failed", Acceptance: "unknown", Error: &Error{Message: fmt.Sprintf(format, a...)}}
}

func handle(ctx context.Context, stdin io.Reader, env Env) Response {
	req, err := readRequest(stdin)
	if err != nil {
		return rejected("protocol", "%v", err)
	}
	if env.GOOS != "darwin" {
		return rejected("rejected", "notify-macos runs on macOS only (this is %s)", env.GOOS)
	}
	for k := range req.Settings {
		if k != SettingBackend {
			return rejected("rejected", "unknown setting %q (the only setting is %s)", k, SettingBackend)
		}
	}
	backend, err := findBackend(req.Settings[SettingBackend], env.Getenv("PATH"), AppDirs(env.Getenv))
	if err != nil {
		return rejected("rejected", "%v", err)
	}
	title, subtitle, body := compose(*req.Message)
	if body == "" {
		// terminal-notifier would refuse it (an empty message prints its help)
		return rejected("rejected", "the message has no body to show")
	}
	deadline, ok := runDeadline(req, env.Now())
	if !ok {
		return rejected("rejected", "too little time left to run terminal-notifier: give the feather a timeout of at least %s", minTimeout)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	return deliver(ctx, backend, childEnv(env.Getenv), title, subtitle, body)
}

// readRequest reads and checks one request.
func readRequest(r io.Reader) (*Request, error) {
	in, err := io.ReadAll(io.LimitReader(r, MaxRequest+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read the request: %v", err)
	}
	if len(in) > MaxRequest {
		return nil, fmt.Errorf("the request is larger than %d bytes", MaxRequest)
	}
	if len(bytes.TrimSpace(in)) == 0 {
		return nil, errors.New("no request on standard input")
	}
	dec := json.NewDecoder(bytes.NewReader(in))
	var req Request
	if err := dec.Decode(&req); err != nil {
		return nil, fmt.Errorf("the request is not a protocol v1 JSON object: %v", err)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("more than one JSON value on standard input")
	}
	switch {
	case req.ProtocolVersion == nil:
		return nil, errors.New("protocol_version is missing")
	case *req.ProtocolVersion != ProtocolVersion:
		return nil, fmt.Errorf("unsupported protocol_version %d (this plugin speaks %d)", *req.ProtocolVersion, ProtocolVersion)
	case req.Message == nil:
		return nil, errors.New("message is missing")
	}
	return &req, nil
}

// runDeadline is when terminal-notifier is stopped: a margin before kjak's
// deadline (the request's, else its timeout from now, else kjak's default
// timeout). false: less than minRun would be left, and nothing is run.
func runDeadline(req *Request, now time.Time) (time.Time, bool) {
	d := req.Deadline
	if d.IsZero() {
		t := time.Duration(req.TimeoutMS) * time.Millisecond
		if t <= 0 {
			t = defaultTimeout
		}
		d = now.Add(t)
	}
	d = d.Add(-margin)
	return d, !d.Before(now.Add(minRun))
}

// compose is what terminal-notifier shows: the title (kjak when empty) and
// the level on one line each, the body and then the URL.
func compose(m Message) (title, subtitle, body string) {
	title = oneLine(m.Title)
	if title == "" {
		title = fallbackTitle
	}
	body = text(m.Body)
	if u := oneLine(m.URL); u != "" {
		if body != "" {
			body += "\n"
		}
		body += u
	}
	return title, oneLine(m.Level), body
}

// text keeps line breaks (CRLF and CR become LF) and tabs, and turns every
// other control character, and the characters that change how text reads
// (bidirectional controls, zero-width space, byte order mark), into a space.
// Joiners stay: emoji sequences use them.
func text(s string) string {
	s = strings.ToValidUTF8(s, "�")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' || disguise(r) {
			return ' '
		}
		return r
	}, s))
}

// disguise: a format character that can make text read otherwise than it
// is (a reversed URL, an invisible break).
func disguise(r rune) bool {
	switch {
	case r >= '\u202A' && r <= '\u202E', r >= '\u2066' && r <= '\u2069':
		return true
	}
	switch r {
	case '\u200B', '\u200E', '\u200F', '\u061C', '\uFEFF':
		return true
	}
	return false
}

// oneLine is text on one line.
func oneLine(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, text(s)))
}
