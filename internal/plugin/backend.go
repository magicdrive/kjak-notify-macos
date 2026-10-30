package plugin

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

// BackendName is the file name of terminal-notifier.
const BackendName = "terminal-notifier"

// BundleID is terminal-notifier's bundle identifier.
const BundleID = "fr.julienxx.oss.terminal-notifier"

// Exit statuses of terminal-notifier 3.1.0 (Terminal Notifier/AppDelegate.m).
const (
	exitSuccess        = 0
	exitUsage          = 1
	exitBadArgument    = 2
	exitNotAuthorized  = 3
	exitTimeout        = 4 // its watchdog: no answer from the notification service
	exitDeliveryFailed = 5 // a refused request, or the handling of a click on an older notification
)

const (
	// maxOutput bounds what is kept of terminal-notifier's stdout and stderr.
	maxOutput = 64 << 10
	// waitDelay bounds the wait for its pipes once it was stopped or exited
	// (terminal-notifier starts no child; a stub might).
	waitDelay = 250 * time.Millisecond
	// maxWrapper bounds the size of a Homebrew wrapper script.
	maxWrapper = 4 << 10
	// maxPlist bounds the size of the application's Info.plist.
	maxPlist = 1 << 20
	// EnvAppDirs names the variable of other directories that may hold
	// terminal-notifier.app (colon-separated, absolute). It comes from the
	// user's environment, which a project's configuration cannot set.
	EnvAppDirs = "TERMINAL_NOTIFIER_APP_DIRS"
	// gidAdmin is macOS's admin group: Homebrew's /usr/local directories are
	// writable by it, and its members can become root anyway.
	gidAdmin = 80
)

var (
	// appExecutable is the executable of terminal-notifier's application.
	appExecutable = regexp.MustCompile(`^/.*/terminal-notifier\.app/Contents/MacOS/terminal-notifier$`)
	// homebrewWrapper is the script Homebrew installs as bin/terminal-notifier.
	homebrewWrapper = regexp.MustCompile(`^#!/bin/bash\nexec "(/[^"\n$\x60\\]*/terminal-notifier\.app/Contents/MacOS/terminal-notifier)" "\$@"\n?$`)
	// bundleID is the identifier in the application's Info.plist (XML).
	bundleID = regexp.MustCompile(`<key>CFBundleIdentifier</key>\s*<string>` + regexp.QuoteMeta(BundleID) + `</string>`)
	// cellar is Homebrew's install directory of a terminal-notifier version.
	cellar = regexp.MustCompile(`^(/usr/local|/opt/homebrew)/Cellar/terminal-notifier/[^/]+$`)
)

// AppDirs are the directories where terminal-notifier.app may be: Homebrew's
// Cellar (matched by cellar), /Applications, ~/Applications and the
// absolute directories of $TERMINAL_NOTIFIER_APP_DIRS. An application
// elsewhere is not run: a setting could otherwise point at a look-alike
// bundle in a project's own files.
func AppDirs(getenv func(string) string) []string {
	dirs := []string{"/Applications"}
	if home := getenv("HOME"); filepath.IsAbs(home) {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	for _, d := range filepath.SplitList(getenv(EnvAppDirs)) {
		if filepath.IsAbs(d) {
			dirs = append(dirs, filepath.Clean(d))
		}
	}
	return dirs
}

// inAppDir: real is the executable of a terminal-notifier.app directly in
// one of the allowed directories.
func inAppDir(real string, dirs []string) bool {
	app := filepath.Dir(filepath.Dir(filepath.Dir(real))) // .../terminal-notifier.app
	parent := filepath.Dir(app)
	if cellar.MatchString(parent) {
		return true
	}
	for _, d := range dirs {
		if parent == d {
			return true
		}
	}
	return false
}

// findBackend is the executable to run: terminal-notifier's application
// executable, named by the setting (an absolute path), else found as the
// first terminal-notifier of the absolute PATH directories (an empty or
// relative element is never searched). The name may be that executable, a
// link to it, or Homebrew's wrapper script (then the executable it names is
// run, without bash). No other program is run: a setting that names an
// interpreter, or a link to one, is refused.
//
// The application must be in one of appDirs (AppDirs) or in Homebrew's
// Cellar. Every file involved, every directory holding one and every
// directory above them must be owned by root or by the user, not writable
// by others (a sticky directory may be), and writable by the group only when
// that group is wheel or admin.
func findBackend(setting, path string, appDirs []string) (string, error) {
	name, err := backendName(setting, path)
	if err != nil {
		return "", err
	}
	// the name, where it was found
	if err := trustedTree(filepath.Dir(name)); err != nil {
		return "", err
	}
	li, err := os.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		// the reason first: kjak keeps 300 characters of a message
		return "", fmt.Errorf("no %s at %s (brew install terminal-notifier, or fix %s)", BackendName, name, SettingBackend)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	if err := trusted(name, li, false); err != nil {
		return "", err
	}
	real, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", fmt.Errorf("%s: %v", name, err)
	}
	if !appExecutable.MatchString(real) {
		// Homebrew's wrapper: run what it would run
		if err := regularFile(real); err != nil {
			return "", err
		}
		script, err := readSmall(real, maxWrapper)
		if err != nil {
			return "", fmt.Errorf("%s is not terminal-notifier (neither its application's executable nor Homebrew's wrapper: %v): it is not run", real, err)
		}
		m := homebrewWrapper.FindSubmatch(script)
		if m == nil {
			return "", fmt.Errorf("%s is not terminal-notifier (neither its application's executable nor Homebrew's wrapper): it is not run", real)
		}
		if real, err = filepath.EvalSymlinks(string(m[1])); err != nil {
			return "", fmt.Errorf("%s: %v", m[1], err)
		}
		if !appExecutable.MatchString(real) {
			return "", fmt.Errorf("%s is not terminal-notifier's application: it is not run", real)
		}
	}
	if !inAppDir(real, appDirs) {
		return "", fmt.Errorf("%s is not in a place terminal-notifier is installed to (Homebrew's Cellar, /Applications, ~/Applications, or $%s): it is not run", real, EnvAppDirs)
	}
	if err := regularFile(real); err != nil {
		return "", err
	}
	plist, err := readSmall(filepath.Join(filepath.Dir(filepath.Dir(real)), "Info.plist"), maxPlist)
	if err != nil || !isBundle(plist) {
		return "", fmt.Errorf("%s is not terminal-notifier's application (bundle %s): it is not run", real, BundleID)
	}
	return real, nil
}

// isBundle: an Info.plist naming terminal-notifier's bundle identifier, XML
// or binary (a binary property list keeps ASCII strings as they are).
func isBundle(plist []byte) bool {
	if bytes.HasPrefix(plist, []byte("bplist00")) {
		return bytes.Contains(plist, []byte("CFBundleIdentifier")) && bytes.Contains(plist, []byte(BundleID))
	}
	return bundleID.Match(plist)
}

// readSmall reads a regular file of at most max bytes (never a FIFO or a
// device, never more than max).
func readSmall(name string, max int64) ([]byte, error) {
	fi, err := os.Stat(name)
	if err != nil {
		return nil, err
	}
	if !fi.Mode().IsRegular() || fi.Size() > max {
		return nil, fmt.Errorf("%s is not a regular file of at most %d bytes", name, max)
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, max))
}

// backendName is the setting, or the first terminal-notifier of the
// absolute PATH directories.
func backendName(setting, path string) (string, error) {
	if setting != "" {
		if !filepath.IsAbs(setting) {
			return "", fmt.Errorf("setting %s must be an absolute path, not %q", SettingBackend, setting)
		}
		if filepath.Base(setting) != BackendName {
			return "", fmt.Errorf("setting %s must name a file called %s, not %q", SettingBackend, BackendName, setting)
		}
		return filepath.Clean(setting), nil
	}
	for _, dir := range filepath.SplitList(path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		p := filepath.Join(dir, BackendName)
		if _, err := os.Lstat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("%s was not found in the absolute directories of PATH (brew install terminal-notifier, or set %s)", BackendName, SettingBackend)
}

// regularFile: a regular executable file nobody else may change, in
// directories nobody else may change.
func regularFile(name string) error {
	fi, err := os.Stat(name)
	if err != nil {
		return fmt.Errorf("%s: %v", name, err)
	}
	switch {
	case !fi.Mode().IsRegular():
		return fmt.Errorf("%s is not a regular file", name)
	case fi.Mode().Perm()&0o111 == 0:
		return fmt.Errorf("%s is not executable", name)
	}
	if err := trusted(name, fi, false); err != nil {
		return err
	}
	return trustedTree(filepath.Dir(name))
}

// trustedTree checks dir and every directory above it.
func trustedTree(dir string) error {
	for d := dir; ; d = filepath.Dir(d) {
		fi, err := os.Stat(d)
		if err != nil {
			return fmt.Errorf("cannot check %s: %v", d, err)
		}
		if err := trusted(d, fi, true); err != nil {
			return err
		}
		if d == filepath.Dir(d) {
			return nil
		}
	}
}

// trusted: owned by root or by the user; not writable by others (a sticky
// directory may be); writable by the group only when the group is wheel or
// admin (whose members can become root: Homebrew's directories, an
// application dragged into /Applications). A symbolic link's own mode means
// nothing.
func trusted(name string, fi fs.FileInfo, dir bool) error {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%s: no owner information", name)
	}
	return trustedMode(name, fi.Mode(), st.Uid, st.Gid, dir, os.Getuid())
}

func trustedMode(name string, mode fs.FileMode, uid, gid uint32, dir bool, me int) error {
	if uid != 0 && int(uid) != me {
		return fmt.Errorf("%s belongs to another user (uid %d): it is not run", name, uid)
	}
	if mode&fs.ModeSymlink != 0 {
		return nil
	}
	perm := mode.Perm()
	if perm&0o002 != 0 && !(dir && mode&fs.ModeSticky != 0) {
		return fmt.Errorf("%s is writable by others: it is not run", name)
	}
	if perm&0o020 != 0 && gid != 0 && gid != gidAdmin {
		return fmt.Errorf("%s is writable by its group: it is not run", name)
	}
	return nil
}

// capped keeps the first max bytes written to it.
type capped struct {
	buf bytes.Buffer
	max int
}

func (c *capped) Write(p []byte) (int, error) {
	if room := c.max - c.buf.Len(); room > 0 {
		c.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// args is terminal-notifier's argument list. terminal-notifier reads its
// options through NSUserDefaults: a value that starts with "-" would be read
// as another option (-execute), and one that starts with "(", "{", "<" or a
// quote as a property list. It removes one leading backslash from every
// value, so each value is given with one: it is then always that exact
// string. The body is not an argument: it goes on standard input.
func args(title, subtitle string) []string {
	a := []string{"-title", `\` + title}
	if subtitle != "" {
		a = append(a, "-subtitle", `\`+subtitle)
	}
	return a
}

// childEnv is terminal-notifier's environment: what a macOS program needs,
// nothing else of the plugin's (no shell or loader variable).
func childEnv(getenv func(string) string) []string {
	var env []string
	for _, k := range []string{"HOME", "TMPDIR", "LANG", "LC_ALL", "LC_CTYPE", "TZ"} {
		if v := getenv(k); v != "" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "PATH=/usr/bin:/bin:/usr/sbin:/sbin")
}

// deliver runs terminal-notifier once and tells what its exit means.
func deliver(ctx context.Context, backend string, env []string, title, subtitle, body string) Response {
	cmd := exec.CommandContext(ctx, backend, args(title, subtitle)...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(body)
	stdout := &capped{max: maxOutput} // never forwarded: stdout is the response's
	stderr := &capped{max: maxOutput}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	cmd.WaitDelay = waitDelay
	if err := cmd.Start(); err != nil {
		return rejected("rejected", "%s could not be started: %v", backend, startError(err))
	}
	err := cmd.Wait()
	diag := firstLine(stderr.buf.String())
	with := func(s string) string {
		if diag != "" {
			return s + ": " + diag
		}
		return s
	}
	if ctx.Err() != nil {
		return unknown("%s", with("terminal-notifier did not finish before the deadline and was stopped; the notification may have been posted"))
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		return Response{ProtocolVersion: ProtocolVersion, Status: "delivered", Acceptance: "confirmed"}
	case !errors.As(err, &ee):
		return unknown("%s", with(fmt.Sprintf("terminal-notifier: %v; the notification may have been posted", err)))
	case !ee.Exited():
		return unknown("%s", with(fmt.Sprintf("terminal-notifier was stopped (%v); the notification may have been posted", ee)))
	}
	switch ee.ExitCode() {
	case exitNotAuthorized:
		return rejected("rejected", "%s", with("notifications are not allowed for terminal-notifier (System Settings, Notifications)"))
	case exitUsage, exitBadArgument:
		return rejected("internal", "%s", with(fmt.Sprintf("terminal-notifier refused its arguments (exit %d)", ee.ExitCode())))
	case exitDeliveryFailed:
		// "Failed to deliver notification" is printed only when the request
		// was refused; exit 5 otherwise ends the handling of a click on an
		// older notification, which may come after this one was posted
		if strings.Contains(stderr.buf.String(), "Failed to deliver notification") {
			return rejected("rejected", "%s", with("the notification center refused the notification"))
		}
	case exitTimeout:
		return unknown("%s", with("the notification service did not answer (no GUI session?); the notification may have been posted"))
	}
	return unknown("%s", with(fmt.Sprintf("terminal-notifier exited with status %d; the notification may have been posted", ee.ExitCode())))
}

// startError is why a program could not be started, without the noise.
func startError(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// firstLine is the first non-empty line of s (terminal-notifier's own
// reason; the next lines are advice), on one line, at most 300 characters
// (kjak shortens it again).
func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if l := oneLine(line); l != "" {
			if r := []rune(l); len(r) > 300 {
				l = string(r[:299]) + "…"
			}
			return l
		}
	}
	return ""
}
