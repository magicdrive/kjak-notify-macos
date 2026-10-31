package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These tests never post a notification: terminal-notifier is a stub, a
// shell script that records its arguments and standard input and exits with
// the status a test chooses.

// stub writes a terminal-notifier application into a new directory
// (DIR/terminal-notifier.app, with its Info.plist) and returns its
// executable, DIR/terminal-notifier.app/Contents/MacOS/terminal-notifier,
// and that MacOS directory. The executable records its arguments
// (NUL-separated, in args), standard input (stdin), environment (env) and
// pid next to itself; like terminal-notifier, it prints its help and exits
// 1 when standard input is empty; then it runs script.
func stub(t *testing.T, script string) (path, dir string) {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	stubRoots = append(stubRoots, root)
	contents := filepath.Join(root, "terminal-notifier.app", "Contents")
	dir = filepath.Join(contents, "MacOS")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
	<key>CFBundleIdentifier</key>
	<string>` + BundleID + `</string>
</dict></plist>
`
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	path = filepath.Join(dir, BackendName)
	body := fmt.Sprintf("#!/bin/sh\nd=%q\n: > \"$d/args\"\nfor a in \"$@\"; do printf '%%s\\0' \"$a\" >> \"$d/args\"; done\ncat > \"$d/stdin\"\nenv > \"$d/env\"\necho $$ > \"$d/pid\"\n[ -s \"$d/stdin\" ] || { echo 'terminal-notifier (3.1.0) is a command-line tool'; exit 1; }\n%s\n", dir, script)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

// stubRoots are the directories of the stubs' applications: run gives them
// to the plugin as $TERMINAL_NOTIFIER_APP_DIRS (unless the test sets it).
var stubRoots []string

func recorded(t *testing.T, dir string) (args []string, stdin string) {
	t.Helper()
	a, err := os.ReadFile(filepath.Join(dir, "args"))
	if err != nil {
		t.Fatal(err)
	}
	args = strings.Split(strings.TrimSuffix(string(a), "\x00"), "\x00")
	s, err := os.ReadFile(filepath.Join(dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	return args, string(s)
}

func request(msg Message, settings map[string]string, extra map[string]any) string {
	r := map[string]any{
		"protocol_version": 1, "feather": "mac", "plugin": "notify-macos", "attempt": 1,
		"deadline": time.Now().Add(10 * time.Second).UTC().Format(time.RFC3339Nano), "timeout_ms": 10000,
		"idempotency_key": "k", "message": msg, "settings": settings, "secrets": map[string]string{},
	}
	for k, v := range extra {
		if v == nil {
			delete(r, k)
		} else {
			r[k] = v
		}
	}
	b, _ := json.Marshal(r)
	return string(b)
}

type result struct {
	resp           map[string]any
	stdout, stderr string
	code           int
}

func run(t *testing.T, in string, env Env) result {
	t.Helper()
	if env.GOOS == "" {
		env.GOOS = "darwin"
	}
	getenv := env.Getenv
	if getenv == nil {
		getenv = func(string) string { return "" }
	}
	env.Getenv = func(k string) string {
		if v := getenv(k); v != "" || k != EnvAppDirs {
			return v
		}
		return strings.Join(stubRoots, ":")
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	var out, errOut bytes.Buffer
	code := Run(context.Background(), strings.NewReader(in), &out, &errOut, env)
	r := result{stdout: out.String(), stderr: errOut.String(), code: code}
	checkResponse(t, r.stdout)
	if err := json.Unmarshal(out.Bytes(), &r.resp); err != nil {
		t.Fatalf("response %q: %v", r.stdout, err)
	}
	return r
}

// checkResponse: exactly one JSON object on stdout, in the shape protocol v1
// requires (the rules of kjak's parseResponse).
func checkResponse(t *testing.T, stdout string) {
	t.Helper()
	if strings.Count(stdout, "\n") != 1 || !strings.HasSuffix(stdout, "}\n") {
		t.Fatalf("stdout is not one JSON line: %q", stdout)
	}
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.DisallowUnknownFields()
	var r struct {
		ProtocolVersion *int   `json:"protocol_version"`
		Status          string `json:"status"`
		Acceptance      string `json:"acceptance"`
		Error           *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := dec.Decode(&r); err != nil {
		t.Fatalf("response %q: %v", stdout, err)
	}
	if r.ProtocolVersion == nil || *r.ProtocolVersion != 1 {
		t.Fatalf("protocol_version: %q", stdout)
	}
	switch {
	case r.Status == "delivered" && r.Acceptance == "confirmed" && r.Error == nil:
	case r.Status == "failed" && (r.Acceptance == "rejected" || r.Acceptance == "unknown") && r.Error != nil && r.Error.Message != "":
		switch r.Error.Code {
		case "", "rejected", "protocol", "internal":
		default:
			t.Fatalf("error code %q", r.Error.Code)
		}
	default:
		t.Fatalf("not a v1 response: %q", stdout)
	}
}

func expect(t *testing.T, r result, status, acceptance, code, text string) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d", r.code)
	}
	if r.resp["status"] != status || r.resp["acceptance"] != acceptance {
		t.Fatalf("got %s", r.stdout)
	}
	if _, ok := r.resp["retryable"]; ok {
		t.Fatalf("retryable set: %s", r.stdout)
	}
	if status == "failed" {
		e, _ := r.resp["error"].(map[string]any)
		if c, _ := e["code"].(string); c != code {
			t.Fatalf("code %q, want %q: %s", c, code, r.stdout)
		}
		if m, _ := e["message"].(string); !strings.Contains(m, text) {
			t.Fatalf("message %q, want %q", m, text)
		}
	}
	if r.stderr != "" {
		t.Fatalf("stderr %q (a failure is told in the response only)", r.stderr)
	}
}

func TestDeliver(t *testing.T) {
	p, dir := stub(t, "exit 0")
	r := run(t, request(Message{Title: "Build done", Body: "all tests passed", Level: "success", URL: "https://ci.example/7"}, map[string]string{SettingBackend: p}, nil), Env{})
	expect(t, r, "delivered", "confirmed", "", "")
	args, stdin := recorded(t, dir)
	if want := []string{"-title", `\Build done`, "-subtitle", `\success`}; fmt.Sprint(args) != fmt.Sprint(want) {
		t.Fatalf("args %q", args)
	}
	if stdin != "all tests passed\nhttps://ci.example/7" {
		t.Fatalf("stdin %q", stdin)
	}
	if r.stderr != "" {
		t.Fatalf("stderr %q", r.stderr)
	}
}

// The text is data: whatever it holds, the arguments are exactly
// -title \<title> -subtitle \<level>, every value starts with the backslash
// terminal-notifier removes (never "-": never another option), and the body
// is standard input.
func TestTextIsData(t *testing.T) {
	mark := filepath.Join(t.TempDir(), "mark")
	cases := []Message{
		{Title: "$(touch " + mark + ")", Body: "`touch " + mark + "`; touch " + mark, Level: "info"},
		{Title: "-execute", Body: "-message", Level: "-help"},
		{Title: `\already "quoted" 'single'`, Body: "back\\slash \\n", Level: "info"},
		{Title: "日本語のタイトル 😀", Body: "本文 🚀\n二行目", Level: "error"},
		{Title: "$(touch /tmp/x) `id` ; | & > <", Body: "$HOME ${PATH} !! * ? [a] {b} ~", Level: "warning"},
		{Title: "(a,b)", Body: "{a=b;}", Level: "<0001>"},
		{Title: "line\none\r\ntwo\ttab\x00nul\x1b[31m", Body: "a\r\nb\rc\x07d e", Level: "critical"},
		{Title: "", Body: "only a body", Level: ""},
	}
	for i, m := range cases {
		p, dir := stub(t, "exit 0")
		r := run(t, request(m, map[string]string{SettingBackend: p}, nil), Env{})
		expect(t, r, "delivered", "confirmed", "", "")
		args, stdin := recorded(t, dir)
		title, subtitle, body := compose(m)
		want := []string{"-title", `\` + title}
		if subtitle != "" {
			want = append(want, "-subtitle", `\`+subtitle)
		}
		if fmt.Sprintf("%q", args) != fmt.Sprintf("%q", want) || stdin != body {
			t.Fatalf("%d: args %q stdin %q, want %q %q", i, args, stdin, want, body)
		}
		for _, a := range args[1:] {
			if a != "-subtitle" && !strings.HasPrefix(a, `\`) {
				t.Fatalf("%d: value %q", i, a)
			}
		}
		for _, s := range []string{title, subtitle} {
			if strings.ContainsAny(s, "\n\r\t\x00\x1b") {
				t.Fatalf("%d: %q", i, s)
			}
		}
	}
	if _, err := os.Stat(mark); err == nil {
		t.Fatal("the text was run as a command")
	}
}

// A message with nothing to show is refused before terminal-notifier runs
// (it would print its help and exit 1).
func TestEmptyBody(t *testing.T) {
	p, dir := stub(t, "exit 0")
	for _, body := range []string{"", " \n\t", "\uFEFF", "\x01\x7f", "\u200B\u202E"} {
		expect(t, run(t, request(Message{Title: "t", Body: body}, map[string]string{SettingBackend: p}, nil), Env{}), "failed", "rejected", "rejected", "no body")
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("terminal-notifier ran")
	}
	// the stub, like terminal-notifier, refuses an empty standard input
	cmd := exec.Command(p, "-title", `\t`)
	if err := cmd.Run(); err == nil {
		t.Fatal("the stub accepted an empty message")
	}
}

func TestCompose(t *testing.T) {
	for _, c := range []struct {
		in                    Message
		title, subtitle, body string
	}{
		{Message{Body: "b"}, "kjak", "", "b"},
		{Message{Title: "  t  ", Body: " b ", Level: "info"}, "t", "info", "b"},
		{Message{Title: "a\nb", Body: "x\r\ny\rz", URL: "u"}, "a b", "", "x\ny\nz\nu"},
		{Message{Body: "", URL: "https://x"}, "kjak", "", "https://x"},
		{Message{Title: "\xff", Body: "\x01\x7f\u0085"}, "\uFFFD", "", ""},
		{Message{Title: "a\u2028b", Body: "x\u2029y"}, "a b", "", "x y"},
		{Message{Title: "moc.evil\u202E//:sptth", Body: "\uFEFFa\u200Bb\u2066c\u2069"}, "moc.evil //:sptth", "", "a b c"},
		{Message{Body: "👨\u200D👩\u200D👧 keeps its joiners"}, "kjak", "", "👨\u200D👩\u200D👧 keeps its joiners"},
	} {
		title, subtitle, body := compose(c.in)
		if title != c.title || subtitle != c.subtitle || body != c.body {
			t.Errorf("%+v: %q %q %q", c.in, title, subtitle, body)
		}
	}
}

// A long body (kjak's limit is 16384 characters) reaches terminal-notifier
// whole, on standard input.
func TestLongBody(t *testing.T) {
	p, dir := stub(t, "exit 0")
	long := strings.Repeat("長い本文 😀 ", 2000)
	r := run(t, request(Message{Body: long}, map[string]string{SettingBackend: p}, nil), Env{})
	expect(t, r, "delivered", "confirmed", "", "")
	if _, stdin := recorded(t, dir); stdin != strings.TrimSpace(long) {
		t.Fatalf("stdin of %d bytes", len(stdin))
	}
}

func TestMalformedRequests(t *testing.T) {
	p, dir := stub(t, "exit 0")
	ok := map[string]string{SettingBackend: p}
	for name, in := range map[string]string{
		"empty":            "",
		"blank":            " \n\t",
		"not json":         "hello",
		"truncated":        `{"protocol_version": 1, "message": {"body": "x"`,
		"array":            `[1, 2]`,
		"two values":       request(Message{Body: "x"}, ok, nil) + request(Message{Body: "x"}, ok, nil),
		"no version":       request(Message{Body: "x"}, ok, map[string]any{"protocol_version": nil}),
		"version 2":        request(Message{Body: "x"}, ok, map[string]any{"protocol_version": 2}),
		"version string":   request(Message{Body: "x"}, ok, map[string]any{"protocol_version": "1"}),
		"no message":       request(Message{Body: "x"}, ok, map[string]any{"message": nil}),
		"message string":   request(Message{Body: "x"}, ok, map[string]any{"message": "x"}),
		"body number":      request(Message{Body: "x"}, ok, map[string]any{"message": map[string]any{"body": 1}}),
		"settings number":  request(Message{Body: "x"}, ok, map[string]any{"settings": map[string]any{SettingBackend: 1}}),
		"deadline garbage": request(Message{Body: "x"}, ok, map[string]any{"deadline": "soon"}),
	} {
		t.Run(name, func(t *testing.T) {
			expect(t, run(t, in, Env{}), "failed", "rejected", "protocol", "")
		})
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("terminal-notifier ran for a malformed request")
	}
	// too large: never read past the limit
	big := `{"protocol_version":1,"message":{"body":"` + strings.Repeat("a", MaxRequest) + `"}}`
	expect(t, run(t, big, Env{}), "failed", "rejected", "protocol", "larger than")
}

// Unknown fields of the request are ignored (new ones may come in v1).
func TestUnknownRequestFields(t *testing.T) {
	p, _ := stub(t, "exit 0")
	in := request(Message{Body: "x"}, map[string]string{SettingBackend: p}, map[string]any{"future": map[string]any{"a": 1}})
	expect(t, run(t, in, Env{}), "delivered", "confirmed", "", "")
}

func TestSettings(t *testing.T) {
	p, _ := stub(t, "exit 0")
	expect(t, run(t, request(Message{Body: "x"}, map[string]string{"sound": "default"}, nil), Env{}), "failed", "rejected", "rejected", `unknown setting "sound"`)
	expect(t, run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: "bin/terminal-notifier"}, nil), Env{}), "failed", "rejected", "rejected", "must be an absolute path")
	// no setting: the absolute PATH directories only
	dir := filepath.Dir(p)
	path := func(s string) Env {
		return Env{Getenv: func(k string) string {
			if k == "PATH" {
				return s
			}
			return ""
		}}
	}
	expect(t, run(t, request(Message{Body: "x"}, nil, nil), path("/nonexistent:"+dir)), "delivered", "confirmed", "", "")
	expect(t, run(t, request(Message{Body: "x"}, nil, nil), path("")), "failed", "rejected", "rejected", "was not found")
	rel, err := filepath.Rel(mustGetwd(t), dir)
	if err != nil {
		t.Fatal(err)
	}
	expect(t, run(t, request(Message{Body: "x"}, nil, nil), path(rel+"::.")), "failed", "rejected", "rejected", "was not found")
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}

// A terminal-notifier that someone else could have changed is not run.
func TestUntrustedBackend(t *testing.T) {
	send := func(setting string) result {
		return run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: setting}, nil), Env{})
	}
	p, dir := stub(t, "exit 0")
	chmod := func(name string, mode os.FileMode) {
		t.Helper()
		if err := os.Chmod(name, mode); err != nil {
			t.Fatal(err)
		}
	}
	chmod(p, 0o775)
	expect(t, send(p), "failed", "rejected", "rejected", "writable by its group")
	chmod(p, 0o757)
	expect(t, send(p), "failed", "rejected", "rejected", "writable by others")
	chmod(p, 0o755)
	chmod(dir, 0o777)
	expect(t, send(p), "failed", "rejected", "rejected", "writable by others")
	chmod(dir, 0o775) // the group of a test directory is not wheel or admin
	expect(t, send(p), "failed", "rejected", "rejected", "writable by its group")
	chmod(dir, 0o755)
	// a directory above: the bundle's Contents
	chmod(filepath.Dir(dir), 0o777)
	expect(t, send(p), "failed", "rejected", "rejected", "writable by others")
	chmod(filepath.Dir(dir), 0o755)
	chmod(p, 0o644)
	expect(t, send(p), "failed", "rejected", "rejected", "not executable")
	chmod(p, 0o755)
	expect(t, send(filepath.Join(dir, BackendName+"-gone", BackendName)), "failed", "rejected", "rejected", "no such file")
	if err := os.Rename(p, p+".moved"); err != nil {
		t.Fatal(err)
	}
	expect(t, send(p), "failed", "rejected", "rejected", "no terminal-notifier at "+p)
	if err := os.Rename(p+".moved", p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("an untrusted terminal-notifier ran")
	}
	expect(t, send(p), "delivered", "confirmed", "", "")
}

// Only terminal-notifier is run: its application's executable (named, or
// through a link), or Homebrew's wrapper, which is not run itself; never
// an interpreter or another program, whatever the setting names.
func TestBackendIdentity(t *testing.T) {
	send := func(setting string) result {
		return run(t, request(Message{Title: "t", Body: "exec touch /nonexistent/x"}, map[string]string{SettingBackend: setting}, nil), Env{})
	}
	p, dir := stub(t, "exit 0")
	other := t.TempDir()
	if real, err := filepath.EvalSymlinks(other); err == nil {
		other = real
	}
	// an interpreter or another program
	expect(t, send("/usr/bin/tclsh"), "failed", "rejected", "rejected", "must name a file called terminal-notifier")
	expect(t, send(dir), "failed", "rejected", "rejected", "must name a file called terminal-notifier")
	link := filepath.Join(other, BackendName)
	if err := os.Symlink("/usr/bin/tclsh", link); err != nil {
		t.Fatal(err)
	}
	expect(t, send(link), "failed", "rejected", "rejected", "is not terminal-notifier")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	// a link to the application's executable (as /usr/local/bin/terminal-notifier)
	if err := os.Symlink(p, link); err != nil {
		t.Fatal(err)
	}
	expect(t, send(link), "delivered", "confirmed", "", "")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	// Homebrew's wrapper: the executable it names runs, without bash
	wrapper := filepath.Join(other, BackendName)
	write := func(s string) {
		t.Helper()
		if err := os.WriteFile(wrapper, []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("#!/bin/bash\nexec \"" + p + "\" \"$@\"\n")
	if got, err := findBackend(wrapper, "", stubRoots); err != nil || got != p {
		t.Fatalf("%q %v", got, err)
	}
	expect(t, send(wrapper), "delivered", "confirmed", "", "")
	for _, bad := range []string{
		"#!/bin/bash\nexport BASH_ENV=x\nexec \"" + p + "\" \"$@\"\n",
		"#!/bin/bash\nexec \"/usr/bin/tclsh\" \"$@\"\n",
		"#!/bin/sh\nexec \"" + p + "\" \"$@\"\n",
		"#!/bin/bash\nexec \"" + p + "\" \"$@\" -execute x\n",
		"#!/bin/bash\nexec \"$(id)/terminal-notifier.app/Contents/MacOS/terminal-notifier\" \"$@\"\n",
	} {
		write(bad)
		expect(t, send(wrapper), "failed", "rejected", "rejected", "is not terminal-notifier")
	}
	// a wrapper naming a terminal-notifier.app that is not terminal-notifier's
	fake, fakeDir := stub(t, "exit 0")
	plist := filepath.Join(filepath.Dir(fakeDir), "Info.plist")
	if err := os.WriteFile(plist, []byte("<key>CFBundleIdentifier</key><string>com.example.other</string>"), 0o644); err != nil {
		t.Fatal(err)
	}
	expect(t, send(fake), "failed", "rejected", "rejected", "bundle "+BundleID)
	write("#!/bin/bash\nexec \"" + fake + "\" \"$@\"\n")
	expect(t, send(wrapper), "failed", "rejected", "rejected", "bundle "+BundleID)
	if _, err := os.Stat(filepath.Join(fakeDir, "args")); err == nil {
		t.Fatal("ran")
	}
}

// The directory where the name was found, and the link itself, are checked
// too: someone who can write there could make the name lead anywhere.
func TestPathDirectoryChecked(t *testing.T) {
	p, _ := stub(t, "exit 0")
	open := t.TempDir()
	if real, err := filepath.EvalSymlinks(open); err == nil {
		open = real
	}
	if err := os.Symlink(p, filepath.Join(open, BackendName)); err != nil {
		t.Fatal(err)
	}
	env := Env{Getenv: func(k string) string {
		if k == "PATH" {
			return open
		}
		return ""
	}}
	expect(t, run(t, request(Message{Body: "x"}, nil, nil), env), "delivered", "confirmed", "", "")
	if err := os.Chmod(open, 0o777); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(open, 0o755)
	expect(t, run(t, request(Message{Body: "x"}, nil, nil), env), "failed", "rejected", "rejected", "writable by others")
}

func TestTrustedMode(t *testing.T) {
	const me = 501
	for _, c := range []struct {
		mode     os.FileMode
		uid, gid uint32
		dir      bool
		err      string
	}{
		{0o755, me, 20, false, ""},
		{0o755, 0, 0, false, ""},
		{0o755, 502, 20, false, "belongs to another user (uid 502)"},
		{os.ModeSymlink | 0o777, me, 20, false, ""},
		{os.ModeSymlink | 0o777, 502, 20, false, "belongs to another user"},
		{0o775, me, 80, false, ""},                      // an application dragged into /Applications (admin)
		{0o775, me, 20, false, "writable by its group"}, // staff
		{0o775, me, 80, true, ""},                       // Homebrew's /usr/local/Cellar (admin)
		{0o775, me, 0, true, ""},                        // wheel
		{0o775, me, 20, true, "writable by its group"},  // staff
		{0o777 | os.ModeSticky, 0, 0, true, ""},         // /tmp
		{0o777, 0, 0, true, "writable by others"},
		{0o757, me, 20, false, "writable by others"},
	} {
		err := trustedMode("f", c.mode, c.uid, c.gid, c.dir, me)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%v uid %d gid %d dir %v: %v, want %q", c.mode, c.uid, c.gid, c.dir, err, c.err)
		}
	}
}

// terminal-notifier gets a small environment: none of the plugin's other
// variables (BASH_ENV, DYLD_*, ...).
func TestChildEnvironment(t *testing.T) {
	p, dir := stub(t, "exit 0")
	vars := map[string]string{"HOME": "/Users/me", "LANG": "ja_JP.UTF-8", "PATH": "/evil:/usr/bin", "BASH_ENV": "/tmp/x", "DYLD_INSERT_LIBRARIES": "/tmp/x.dylib"}
	r := run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), Env{Getenv: func(k string) string { return vars[k] }})
	expect(t, r, "delivered", "confirmed", "", "")
	b, err := os.ReadFile(filepath.Join(dir, "env"))
	if err != nil {
		t.Fatal(err)
	}
	env := string(b)
	for _, want := range []string{"HOME=/Users/me", "LANG=ja_JP.UTF-8", "PATH=/usr/bin:/bin:/usr/sbin:/sbin"} {
		if !strings.Contains(env, want+"\n") {
			t.Errorf("no %s in\n%s", want, env)
		}
	}
	for _, bad := range []string{"BASH_ENV", "DYLD_INSERT_LIBRARIES", "/evil"} {
		if strings.Contains(env, bad) {
			t.Errorf("%s reached terminal-notifier", bad)
		}
	}
}

// terminal-notifier's exit statuses (3.1.0) with its own messages: rejected
// when it certainly did not post, unknown when it may have. The reason is
// its first line (the next ones are advice). Nothing is retryable.
func TestExitStatuses(t *testing.T) {
	denied := `printf 'Notifications are turned off for this application.\nEnable them in System Settings > Notifications, or reset the\npermission so it can be asked for again:\n  tccutil reset UserNotification fr.julienxx.oss.terminal-notifier\n' >&2; exit 3`
	timedOut := `printf 'Timed out after 10 seconds waiting for the notification service.\nThis usually means no GUI session is available - for example when running over SSH,\nor from launchd as root. Notifications can only be posted for a logged-in user.\n' >&2; exit 4`
	for _, c := range []struct {
		script, acceptance, code, text string
	}{
		{denied, "rejected", "rejected", "notifications are not allowed for terminal-notifier (System Settings, Notifications): Notifications are turned off for this application."},
		{"echo 'Permission to send notifications was declined.' >&2; exit 3", "rejected", "rejected", ": Permission to send notifications was declined."},
		{"exit 1", "rejected", "internal", "refused its arguments (exit 1)"},
		{"exit 2", "rejected", "internal", "refused its arguments (exit 2)"},
		{"printf 'Failed to deliver notification: x\\nRun with -diagnose to see why.\\n' >&2; exit 5", "rejected", "rejected", "refused the notification: Failed to deliver notification: x"},
		{"echo 'Could not run the command: x' >&2; exit 5", "unknown", "", "exited with status 5"},
		{timedOut, "unknown", "", "did not answer (no GUI session?); the notification may have been posted: Timed out after 10 seconds waiting for the notification service."},
		{"exit 7", "unknown", "", "exited with status 7"},
		{"kill -9 $$", "unknown", "", "was stopped"},
	} {
		p, _ := stub(t, c.script)
		r := run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), Env{})
		expect(t, r, "failed", c.acceptance, c.code, c.text)
		if strings.Contains(r.stdout, "tccutil") {
			t.Fatalf("advice in the reason: %s", r.stdout)
		}
	}
}

// What terminal-notifier prints on stdout never reaches kjak's stdout, and
// what is kept of its output is bounded.
func TestBackendOutputIsolated(t *testing.T) {
	p, _ := stub(t, `echo '{"protocol_version":1,"status":"failed"}'; head -c 200000 /dev/zero | tr '\0' x >&2; echo; echo "last line" >&2; exit 0`)
	r := run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), Env{})
	expect(t, r, "delivered", "confirmed", "", "")
	p, _ = stub(t, `head -c 200000 /dev/zero | tr '\0' y >&2; echo; echo 'Failed to deliver notification' >&2; exit 5`)
	r = run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), Env{})
	expect(t, r, "failed", "unknown", "", "exited with status 5") // the line is past what is kept
	if m := r.resp["error"].(map[string]any)["message"].(string); len([]rune(m)) > 400 {
		t.Fatalf("a message of %d characters", len([]rune(m)))
	}
	c := &capped{max: 10}
	for range 3 {
		if n, err := c.Write([]byte("abcdefgh")); n != 8 || err != nil {
			t.Fatal(n, err)
		}
	}
	if c.buf.String() != "abcdefghab" {
		t.Fatalf("%q", c.buf.String())
	}
}

func TestNotMacOS(t *testing.T) {
	p, dir := stub(t, "exit 0")
	expect(t, run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), Env{GOOS: "linux"}), "failed", "rejected", "rejected", "macOS only")
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("ran")
	}
}

// A terminal-notifier that does not finish is stopped before kjak's
// deadline: unknown (it may have posted), and nothing stays behind.
func TestDeadline(t *testing.T) {
	p, dir := stub(t, "exec sleep 30")
	start := time.Now()
	in := request(Message{Body: "x"}, map[string]string{SettingBackend: p}, map[string]any{"deadline": time.Now().Add(2500 * time.Millisecond).UTC().Format(time.RFC3339Nano)})
	r := run(t, in, Env{})
	expect(t, r, "failed", "unknown", "", "before the deadline")
	if d := time.Since(start); d > 2500*time.Millisecond-margin+waitDelay+200*time.Millisecond {
		t.Fatalf("took %v", d)
	}
	pid, err := os.ReadFile(filepath.Join(dir, "pid"))
	if err != nil {
		t.Fatal(err)
	}
	if alive(strings.TrimSpace(string(pid))) {
		t.Fatalf("terminal-notifier (pid %s) still runs", pid)
	}
}

// Too little time: refused before terminal-notifier runs (it would be
// stopped half-way, and the answer would be unknown).
func TestTimeoutTooShort(t *testing.T) {
	p, dir := stub(t, "exit 0")
	for _, d := range []time.Duration{-time.Second, 0, 500 * time.Millisecond, margin + minRun - 50*time.Millisecond} {
		in := request(Message{Body: "x"}, map[string]string{SettingBackend: p}, map[string]any{"deadline": time.Now().Add(d).UTC().Format(time.RFC3339Nano)})
		expect(t, run(t, in, Env{}), "failed", "rejected", "rejected", "timeout of at least 2s")
	}
	if _, err := os.Stat(filepath.Join(dir, "args")); err == nil {
		t.Fatal("ran")
	}
}

func TestRunDeadline(t *testing.T) {
	now := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		req  Request
		want time.Duration
		ok   bool
	}{
		{Request{Deadline: now.Add(5 * time.Second)}, 5*time.Second - margin, true},
		{Request{TimeoutMS: 3000}, 3*time.Second - margin, true},
		{Request{}, defaultTimeout - margin, true},
		{Request{TimeoutMS: 2000}, 2*time.Second - margin, true},
		{Request{TimeoutMS: 1600}, 1600*time.Millisecond - margin, true}, // 2s, less what starting took
		{Request{TimeoutMS: 1499}, 1499*time.Millisecond - margin, false},
		{Request{Deadline: now.Add(-time.Second)}, -time.Second - margin, false},
	} {
		got, ok := runDeadline(&c.req, now)
		if got.Sub(now) != c.want || ok != c.ok {
			t.Errorf("%+v: %v %v, want %v %v", c.req, got.Sub(now), ok, c.want, c.ok)
		}
	}
}

// Cancellation (kjak stopping the plugin is a kill of its process group;
// this is the context of an embedding): terminal-notifier is stopped.
func TestCancel(t *testing.T) {
	p, dir := stub(t, "exec sleep 30")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(300*time.Millisecond, cancel)
	var out, errOut bytes.Buffer
	getenv := func(k string) string {
		if k == EnvAppDirs {
			return strings.Join(stubRoots, ":")
		}
		return ""
	}
	Run(ctx, strings.NewReader(request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil)), &out, &errOut, Env{GOOS: "darwin", Getenv: getenv, Now: time.Now})
	checkResponse(t, out.String())
	if !strings.Contains(out.String(), `"acceptance":"unknown"`) || errOut.Len() != 0 {
		t.Fatal(out.String(), errOut.String())
	}
	pid, _ := os.ReadFile(filepath.Join(dir, "pid"))
	if alive(strings.TrimSpace(string(pid))) {
		t.Fatal("terminal-notifier still runs")
	}
}

func TestFirstLine(t *testing.T) {
	if got := firstLine("\n  a  \nb\n"); got != "a" {
		t.Fatal(got)
	}
	if got := firstLine(""); got != "" {
		t.Fatal(got)
	}
	if got := firstLine(strings.Repeat("あ", 400)); len([]rune(got)) != 300 {
		t.Fatal(len([]rune(got)))
	}
}

// The application runs only from where terminal-notifier is installed:
// a look-alike bundle elsewhere (a project's own files) is refused, unless
// the user's $TERMINAL_NOTIFIER_APP_DIRS names its directory.
func TestAppDirs(t *testing.T) {
	p, _ := stub(t, "exit 0")
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(p))))
	only := func(dirs string) Env {
		return Env{Getenv: func(k string) string {
			switch k {
			case EnvAppDirs:
				return dirs
			case "HOME":
				return "/nonexistent-home"
			}
			return ""
		}}
	}
	send := func(env Env) result {
		return run(t, request(Message{Body: "x"}, map[string]string{SettingBackend: p}, nil), env)
	}
	expect(t, send(only("relative/dir:/elsewhere")), "failed", "rejected", "rejected", "is not in a place terminal-notifier is installed to")
	expect(t, send(only("/elsewhere:"+root)), "delivered", "confirmed", "", "")
	expect(t, send(only(filepath.Dir(root))), "failed", "rejected", "rejected", "is not in a place") // not a parent: the very directory
	for _, c := range []struct {
		dir  string
		want bool
	}{
		{"/usr/local/Cellar/terminal-notifier/3.1.0", true},
		{"/opt/homebrew/Cellar/terminal-notifier/3.1.0", true},
		{"/Applications", true},
		{"/Users/me/Applications", true},
		{"/usr/local/Cellar/other/3.1.0", false},
		{"/usr/local/Cellar/terminal-notifier/3.1.0/x", false},
		{"/Users/me/src/repo/vendor", false},
	} {
		real := filepath.Join(c.dir, "terminal-notifier.app/Contents/MacOS/terminal-notifier")
		if got := inAppDir(real, AppDirs(func(k string) string {
			if k == "HOME" {
				return "/Users/me"
			}
			return ""
		})); got != c.want {
			t.Errorf("%s: %v", c.dir, got)
		}
	}
}

// What is read of an install is small and regular: a big wrapper, a FIFO
// Info.plist are refused without being read; a binary Info.plist is read.
func TestReadSmall(t *testing.T) {
	p, dir := stub(t, "exit 0")
	other := t.TempDir()
	if real, err := filepath.EvalSymlinks(other); err == nil {
		other = real
	}
	wrapper := filepath.Join(other, BackendName)
	big := "#!/bin/bash\nexec \"" + p + "\" \"$@\"\n" + strings.Repeat("#", maxWrapper)
	if err := os.WriteFile(wrapper, []byte(big), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := findBackend(wrapper, "", stubRoots); err == nil || !strings.Contains(err.Error(), "at most") {
		t.Fatal(err)
	}
	if err := os.WriteFile(wrapper, []byte("#!/bin/bash\nexec \"/$HOME/terminal-notifier.app/Contents/MacOS/terminal-notifier\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := findBackend(wrapper, "", stubRoots); err == nil || !strings.Contains(err.Error(), "is not terminal-notifier") {
		t.Fatal(err)
	}
	plist := filepath.Join(filepath.Dir(dir), "Info.plist")
	if err := os.WriteFile(plist, []byte("bplist00\x01CFBundleIdentifier\x02"+BundleID), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := findBackend(p, "", stubRoots); err != nil {
		t.Fatal("binary plist:", err)
	}
	if err := os.Remove(plist); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command("mkfifo", plist).Run(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := findBackend(p, "", stubRoots); done <- err }()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "bundle") {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("a FIFO Info.plist blocked")
	}
}

// A wrapper whose target is writable by its (staff) group is refused.
func TestWrapperTargetChecked(t *testing.T) {
	p, _ := stub(t, "exit 0")
	other := t.TempDir()
	if real, err := filepath.EvalSymlinks(other); err == nil {
		other = real
	}
	wrapper := filepath.Join(other, BackendName)
	if err := os.WriteFile(wrapper, []byte("#!/bin/bash\nexec \""+p+"\" \"$@\"\n"), 0o775); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(wrapper, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := findBackend(wrapper, "", stubRoots); err == nil || !strings.Contains(err.Error(), "writable by its group") {
		t.Fatal(err)
	}
	if err := os.Chmod(wrapper, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o775); err != nil {
		t.Fatal(err)
	}
	if _, err := findBackend(wrapper, "", stubRoots); err == nil || !strings.Contains(err.Error(), "writable by its group") {
		t.Fatal(err)
	}
}
