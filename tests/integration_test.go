// Package tests runs the plugin under kjak v1.1.0: the real external
// feather (discovery, checks, protocol v1, timeouts), with a stub
// terminal-notifier. No notification is posted.
//
// KJAK_BIN names the kjak executable (make integration builds v1.1.0);
// without it the tests are skipped.
package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const pluginFile = "kjak-feather-notify-macos"

var pluginBin string // built once by TestMain

func TestMain(m *testing.M) {
	if os.Getenv("KJAK_BIN") == "" || runtime.GOOS != "darwin" {
		fmt.Println("integration tests skipped: set KJAK_BIN to a kjak v1.1.0 executable (make integration), on macOS")
		os.Exit(0)
	}
	dir, err := os.MkdirTemp("", "kjak-notify-macos-bin-")
	if err == nil {
		dir, err = filepath.EvalSymlinks(dir)
	}
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	pluginBin = filepath.Join(dir, pluginFile)
	build := exec.Command("go", "build", "-o", pluginBin, "../cmd/kjak-feather-notify-macos")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// sandbox is a kjak home of its own: configuration, PATH with the plugin,
// a stub terminal-notifier.
type sandbox struct {
	t                  *testing.T
	root, bin, stubDir string
	stub               string
	config             string
	env                []string
}

func newSandbox(t *testing.T, script string) *sandbox {
	t.Helper()
	root := t.TempDir()
	if real, err := filepath.EvalSymlinks(root); err == nil {
		root = real
	}
	s := &sandbox{t: t, root: root, bin: filepath.Join(root, "bin"), stubDir: filepath.Join(root, "tn", "terminal-notifier.app", "Contents", "MacOS")}
	for _, d := range []string{s.bin, s.stubDir, filepath.Join(root, "home"), filepath.Join(root, "xdg", "kjak")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(pluginBin)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.bin, pluginFile), b, 0o755); err != nil {
		t.Fatal(err)
	}
	// a terminal-notifier application, as Homebrew's Cellar holds it
	s.stub = filepath.Join(s.stubDir, "terminal-notifier")
	plist := "<key>CFBundleIdentifier</key>\n<string>fr.julienxx.oss.terminal-notifier</string>\n"
	if err := os.WriteFile(filepath.Join(filepath.Dir(s.stubDir), "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	s.setStub(script)
	s.config = filepath.Join(root, "xdg", "kjak", "config.toml")
	s.env = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "xdg"),
		"PATH=" + s.bin + ":/usr/bin:/bin",
		"TMPDIR=" + os.TempDir(),
		"LANG=en_US.UTF-8",
		// the stub's application is not where terminal-notifier is installed
		"TERMINAL_NOTIFIER_APP_DIRS=" + filepath.Join(root, "tn"),
	}
	return s
}

// setStub: the stub appends its arguments (NUL-separated) and its standard
// input to calls, one record per run; like terminal-notifier it prints its
// help and exits 1 on an empty standard input; then it runs script.
func (s *sandbox) setStub(script string) {
	body := fmt.Sprintf("#!/bin/sh\nd=%q\nin=$(cat; printf x); in=${in%%x}\n{ printf 'ARGS'; for a in \"$@\"; do printf '\\0%%s' \"$a\"; done; printf '\\0STDIN\\0%%s\\0END\\0' \"$in\"; } >> \"$d/calls\"\necho $$ >> \"$d/pids\"\n[ -n \"$in\" ] || { echo 'terminal-notifier (3.1.0) is a command-line tool'; exit 1; }\n%s\n", s.stubDir, script)
	if err := os.WriteFile(s.stub, []byte(body), 0o755); err != nil {
		s.t.Fatal(err)
	}
}

type call struct {
	args  []string
	stdin string
}

func (s *sandbox) calls() []call {
	b, err := os.ReadFile(filepath.Join(s.stubDir, "calls"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		s.t.Fatal(err)
	}
	var out []call
	for _, rec := range strings.Split(string(b), "\x00END\x00") {
		if rec == "" {
			continue
		}
		head, stdin, _ := strings.Cut(rec, "\x00STDIN\x00")
		parts := strings.Split(head, "\x00")
		out = append(out, call{args: parts[1:], stdin: stdin})
	}
	return out
}

func (s *sandbox) write(config string) {
	if err := os.WriteFile(s.config, []byte(config), 0o600); err != nil {
		s.t.Fatal(err)
	}
}

type result struct {
	code           int
	stdout, stderr string
}

func (s *sandbox) kjak(args ...string) result {
	s.t.Helper()
	cmd := exec.Command(os.Getenv("KJAK_BIN"), args...)
	cmd.Env = s.env
	cmd.Dir = s.root
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		s.t.Fatal(err)
	}
	return result{code, out.String(), errOut.String()}
}

type report struct {
	Outcome string `json:"outcome"`
	Results []struct {
		Feather    string   `json:"feather"`
		Groups     []string `json:"groups"`
		Status     string   `json:"status"`
		Acceptance string   `json:"acceptance"`
		Attempts   int      `json:"attempts"`
		Error      *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"results"`
}

func (s *sandbox) send(args ...string) (result, report) {
	s.t.Helper()
	r := s.kjak(append([]string{"send", "-o", "json"}, args...)...)
	var rep report
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		s.t.Fatalf("kjak send: %v\n%s\n%s", err, r.stdout, r.stderr)
	}
	return r, rep
}

func (s *sandbox) feather(extra string) string {
	return fmt.Sprintf("[feathers.mac]\ntype = \"external\"\nplugin = \"notify-macos\"\nsettings = { terminal_notifier = %q }\nenv = [\"TERMINAL_NOTIFIER_APP_DIRS\"]\ntimeout = \"10s\"\n%s", s.stub, extra)
}

func TestValidateDiscoverDryRun(t *testing.T) {
	s := newSandbox(t, "exit 0")
	s.write(s.feather(""))
	if r := s.kjak("config", "validate", "--strict"); r.code != 0 {
		t.Fatalf("validate: %+v", r)
	}
	r := s.kjak("feather", "plugins", "-o", "json")
	var plugins struct {
		Plugins []struct {
			Name     string   `json:"name"`
			Problem  string   `json:"problem"`
			Shadowed bool     `json:"shadowed"`
			Feathers []string `json:"feathers"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &plugins); err != nil || r.code != 0 || len(plugins.Plugins) != 1 ||
		plugins.Plugins[0].Name != "notify-macos" || plugins.Plugins[0].Problem != "" || plugins.Plugins[0].Shadowed || fmt.Sprint(plugins.Plugins[0].Feathers) != "[mac]" {
		t.Fatalf("plugins: %v %+v\n%s", err, plugins, r.stdout)
	}
	r, rep := s.send("--to", "mac", "--dry-run", "test")
	if r.code != 0 || rep.Results[0].Status != "planned" {
		t.Fatalf("dry run: %+v", r)
	}
	if len(s.calls()) != 0 {
		t.Fatal("a dry run ran terminal-notifier")
	}
}

func TestSend(t *testing.T) {
	s := newSandbox(t, "exit 0")
	s.write(s.feather(""))
	r, rep := s.send("--to", "mac", "--title", "ビルド完了 😀", "--level", "success", "--url", "https://ci.example/7", "日本語の本文\n二行目 \"引用\" $(id) -execute")
	if r.code != 0 || rep.Outcome != "success" || rep.Results[0].Status != "delivered" || rep.Results[0].Acceptance != "confirmed" || rep.Results[0].Attempts != 1 {
		t.Fatalf("%+v", r)
	}
	c := s.calls()
	if len(c) != 1 {
		t.Fatalf("%d runs", len(c))
	}
	if want := []string{"-title", `\ビルド完了 😀`, "-subtitle", `\success`}; fmt.Sprintf("%q", c[0].args) != fmt.Sprintf("%q", want) {
		t.Fatalf("args %q", c[0].args)
	}
	if c[0].stdin != "日本語の本文\n二行目 \"引用\" $(id) -execute\nhttps://ci.example/7" {
		t.Fatalf("stdin %q", c[0].stdin)
	}
}

// What terminal-notifier reports reaches kjak's report: refused (rejected,
// exit 1 of kjak), never retried.
func TestErrorsPropagate(t *testing.T) {
	s := newSandbox(t, "echo 'Notifications are turned off for this application.' >&2; exit 3")
	s.write(s.feather("[feathers.mac.retry]\npolicy = \"plugin\"\n"))
	r, rep := s.send("--to", "mac", "hello")
	res := rep.Results[0]
	if r.code != 1 || res.Status != "failed" || res.Acceptance != "rejected" || res.Error == nil || res.Error.Code != "rejected" ||
		!strings.Contains(res.Error.Message, "not allowed for terminal-notifier") || res.Attempts != 1 {
		t.Fatalf("%+v\n%s", res, r.stdout)
	}
	// a missing terminal-notifier: refused before anything ran
	if err := os.Remove(s.stub); err != nil {
		t.Fatal(err)
	}
	_, rep = s.send("--to", "mac", "hello")
	if res := rep.Results[0]; res.Acceptance != "rejected" || !strings.Contains(res.Error.Message, "no terminal-notifier at") {
		t.Fatalf("%+v %+v", res, res.Error)
	}
}

// A terminal-notifier that hangs: the plugin stops it before kjak's
// timeout and answers unknown (never retried); nothing stays behind.
func TestTimeout(t *testing.T) {
	s := newSandbox(t, "sleep 30")
	s.write(strings.Replace(s.feather(""), `timeout = "10s"`, `timeout = "3s"`, 1))
	start := time.Now()
	r, rep := s.send("--to", "mac", "hello")
	res := rep.Results[0]
	if r.code != 1 || res.Acceptance != "unknown" || res.Attempts != 1 || !strings.Contains(res.Error.Message, "before the deadline") {
		t.Fatalf("%+v\n%s", res, r.stdout)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Fatalf("took %v", d)
	}
	pids, _ := os.ReadFile(filepath.Join(s.stubDir, "pids"))
	for _, pid := range strings.Fields(string(pids)) {
		if exec.Command("kill", "-0", pid).Run() == nil {
			t.Fatalf("terminal-notifier %s still runs", pid)
		}
	}
}

func TestRepeatedGroupTemplate(t *testing.T) {
	s := newSandbox(t, "exit 0")
	s.write(s.feather("") + `
[groups.desk]
feathers = ["mac"]

[templates.done]
title = "{{.task}} finished"
body = "{{.task}} exited with {{.status}}"
level = "info"
`)
	for i := range 3 {
		if r, rep := s.send("--to", "desk", fmt.Sprintf("run %d", i)); r.code != 0 || rep.Results[0].Status != "delivered" || rep.Results[0].Groups[0] != "desk" {
			t.Fatalf("%d: %+v", i, r)
		}
	}
	if r, _ := s.send("--to", "mac", "--template", "done", "--var", "task=make test", "--var", "status=0"); r.code != 0 {
		t.Fatalf("%+v", r)
	}
	c := s.calls()
	if len(c) != 4 {
		t.Fatalf("%d runs", len(c))
	}
	for i := range 3 {
		if c[i].stdin != fmt.Sprintf("run %d", i) {
			t.Fatalf("%d: %q", i, c[i].stdin)
		}
	}
	if c[3].args[1] != `\make test finished` || c[3].stdin != "make test exited with 0" {
		t.Fatalf("template: %q %q", c[3].args, c[3].stdin)
	}
}

// The example configuration is valid for kjak v1.1.0.
func TestExampleConfig(t *testing.T) {
	s := newSandbox(t, "exit 0")
	example, err := filepath.Abs("../examples/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if r := s.kjak("config", "validate", "--strict", "-c", example); r.code != 0 {
		t.Fatalf("%+v", r)
	}
}

// Every whole configuration of the README and the tutorials (a TOML block
// with a feather) is valid for kjak v1.1.0.
func TestDocumentedConfigs(t *testing.T) {
	fence := regexp.MustCompile("(?s)```toml\n(.*?)```")
	n := 0
	for _, doc := range []string{"../README.md", "../docs/tutorial.en.md", "../docs/tutorial.ja.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range fence.FindAllStringSubmatch(string(b), -1) {
			if !strings.Contains(m[1], "[feathers.") {
				continue // a part (a group) of a configuration
			}
			n++
			s := newSandbox(t, "exit 0")
			s.write(m[1])
			if r := s.kjak("config", "validate", "--strict"); r.code != 0 {
				t.Errorf("%s: %s%s\n%s", doc, r.stdout, r.stderr, m[1])
			}
		}
	}
	if n < 3 {
		t.Fatalf("only %d configurations found", n)
	}
}

// Without the setting, the plugin finds terminal-notifier in the PATH kjak
// gives it: here Homebrew's wrapper in the plugin's directory, which leads to
// the application's executable.
func TestFoundOnPath(t *testing.T) {
	s := newSandbox(t, "exit 0")
	wrapper := "#!/bin/bash\nexec \"" + s.stub + "\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(s.bin, "terminal-notifier"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	s.write("[feathers.mac]\ntype = \"external\"\nplugin = \"notify-macos\"\nenv = [\"TERMINAL_NOTIFIER_APP_DIRS\"]\n")
	if r, rep := s.send("--to", "mac", "hello"); r.code != 0 || rep.Results[0].Status != "delivered" {
		t.Fatalf("%+v", r)
	}
	if c := s.calls(); len(c) != 1 || c[0].stdin != "hello" {
		t.Fatalf("%+v", c)
	}
}

// terminal-notifier's other outcomes, and the plugin's own refusals, as
// kjak reports them.
func TestOutcomes(t *testing.T) {
	for _, c := range []struct {
		script, config, acceptance, code, text string
	}{
		{"printf 'Timed out after 10 seconds waiting for the notification service.\\nor from launchd as root.\\n' >&2; exit 4", "", "unknown", "ambiguous", "did not answer"},
		{"printf 'Failed to deliver notification: x\\n' >&2; exit 5", "", "rejected", "rejected", "refused the notification: Failed to deliver notification: x"},
		{"exit 5", "", "unknown", "ambiguous", "exited with status 5"},
		{"kill -9 $$", "", "unknown", "ambiguous", "was stopped"},
		{"exit 0", `settings = { sound = "default" }`, "rejected", "rejected", `unknown setting "sound"`},
		{"exit 0", `timeout = "1s"`, "rejected", "rejected", "timeout of at least 2s"},
	} {
		s := newSandbox(t, c.script)
		cfg := s.feather("")
		if strings.HasPrefix(c.config, "settings") {
			cfg = strings.Replace(cfg, fmt.Sprintf("settings = { terminal_notifier = %q }", s.stub), c.config, 1)
		} else if c.config != "" {
			cfg = strings.Replace(cfg, `timeout = "10s"`, c.config, 1)
		}
		s.write(cfg)
		r, rep := s.send("--to", "mac", "hello")
		res := rep.Results[0]
		if r.code != 1 || res.Acceptance != c.acceptance || res.Attempts != 1 || res.Error == nil || res.Error.Code != c.code || !strings.Contains(res.Error.Message, c.text) {
			t.Fatalf("%q: %+v %+v\n%s", c.script, res, res.Error, r.stdout)
		}
		if strings.Count(res.Error.Message, c.text) != 1 {
			t.Fatalf("repeated: %s", res.Error.Message)
		}
	}
}

// Two feathers of the plugin in one send run at the same time.
func TestConcurrentFeathers(t *testing.T) {
	s := newSandbox(t, "sleep 2")
	two := s.feather("") + "\n" + strings.Replace(s.feather(""), "[feathers.mac]", "[feathers.mac2]", 1) + "\n[groups.both]\nfeathers = [\"mac\", \"mac2\"]\n"
	s.write(two)
	start := time.Now()
	r, rep := s.send("--to", "both", "hello")
	if r.code != 0 || rep.Outcome != "success" || len(rep.Results) != 2 {
		t.Fatalf("%+v", r)
	}
	if d := time.Since(start); d > 3500*time.Millisecond { // one after the other: 4s
		t.Fatalf("took %v: not run at the same time", d)
	}
	if len(s.calls()) != 2 {
		t.Fatal(len(s.calls()))
	}
}

// The least documented timeout, 2s, is accepted (the plugin's own start is
// taken into account); without $TERMINAL_NOTIFIER_APP_DIRS the stub's
// application, outside the places terminal-notifier is installed to, is not
// run.
func TestMinimumTimeoutAndAppDirs(t *testing.T) {
	s := newSandbox(t, "exit 0")
	s.write(strings.Replace(s.feather(""), `timeout = "10s"`, `timeout = "2s"`, 1))
	for i := range 3 {
		if r, rep := s.send("--to", "mac", "hello"); r.code != 0 || rep.Results[0].Status != "delivered" {
			t.Fatalf("%d: %+v", i, r)
		}
	}
	s.write(strings.Replace(s.feather(""), "env = [\"TERMINAL_NOTIFIER_APP_DIRS\"]\n", "", 1))
	_, rep := s.send("--to", "mac", "hello")
	if res := rep.Results[0]; res.Acceptance != "rejected" || res.Error == nil || !strings.Contains(res.Error.Message, "is not in a place terminal-notifier is installed to") {
		t.Fatalf("%+v", res)
	}
	if len(s.calls()) != 3 {
		t.Fatal(len(s.calls()))
	}
}
