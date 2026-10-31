// Package release holds no code; its tests check the release configuration
// (hephaestus.yaml, hephaestus.rehearsal.yaml) and the workflows that use it
// (.github/workflows), as text: the repository has no YAML dependency.
package release

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// hephaestusAction is the Hephaestus action every workflow uses: the commit
// of v1.5.0.
const hephaestusAction = "magicdrive/hephaestus@0f76752ad2a6598ba8fc6978eb90a790c71c4835 # v1.5.0"

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// body is a configuration without its leading comment.
func body(t *testing.T, s string) string {
	t.Helper()
	i := strings.Index(s, "\nversion: 1\n")
	if i < 0 {
		t.Fatal("no version: 1")
	}
	return s[i+1:]
}

func TestRehearsalConfig(t *testing.T) {
	prod := body(t, read(t, "hephaestus.yaml"))
	for _, r := range []struct{ from, to string }{
		{"    notes_file: RELEASE_NOTES.md\n", "    notes_file: RELEASE_NOTES.md\n    draft: true\n"},
		{"    skip_upload: auto\n", "    skip_upload: \"false\"\n"},
		{"      name: homebrew-tap\n", "      name: homebrew-tap-sandbox\n"},
	} {
		if strings.Count(prod, r.from) != 1 {
			t.Fatalf("hephaestus.yaml: %q is not there once", r.from)
		}
		prod = strings.Replace(prod, r.from, r.to, 1)
	}
	if rehearsal := body(t, read(t, "hephaestus.rehearsal.yaml")); rehearsal != prod {
		t.Errorf("hephaestus.rehearsal.yaml differs from hephaestus.yaml by more than draft, skip_upload and the sandbox tap")
	}
}

func TestReleaseConfig(t *testing.T) {
	c := read(t, "hephaestus.yaml")
	for _, want := range []string{
		"  name: kjak-notify-macos\n",                      // project and formula
		"      os: [darwin]\n      arch: [amd64, arm64]\n", // macOS, both architectures
		"        dst: kjak-feather-notify-macos\n",         // the name kjak looks for
		"      bin.install \"kjak-feather-notify-macos\"\n",
		"      - name: terminal-notifier\n        os: mac\n",
		"    skip_upload: auto\n", // a prerelease never updates the tap
		"      owner: magicdrive\n      name: homebrew-tap\n",
		"      pull_request:\n        enabled: true\n",
	} {
		if !strings.Contains(c, want) {
			t.Errorf("hephaestus.yaml: no %q", want)
		}
	}
	if strings.Contains(c, "goreleaser") || strings.Contains(c, "brews_from") {
		t.Error("hephaestus.yaml uses GoReleaser")
	}
}

// step is one step of a workflow job: its text, from "- " to the next.
func steps(t *testing.T, workflow string) []string {
	t.Helper()
	var out []string
	for _, part := range regexp.MustCompile(`(?m)^      - `).Split(read(t, workflow), -1)[1:] {
		out = append(out, "- "+part)
	}
	return out
}

func TestWorkflowsPinned(t *testing.T) {
	uses := regexp.MustCompile(`(?m)^\s*(?:- )?uses: (\S+)(.*)$`)
	for _, w := range []string{"ci.yml", "release.yml", "release-rehearsal.yml"} {
		s := read(t, ".github/workflows/"+w)
		if !strings.Contains(s, "\npermissions:\n  contents: read\n") {
			t.Errorf("%s: the default permissions are not contents: read", w)
		}
		for _, m := range uses.FindAllStringSubmatch(s, -1) {
			ref := m[1]
			switch {
			case strings.HasPrefix(ref, "./"):
			case strings.HasPrefix(ref, "magicdrive/hephaestus@"):
				if ref+m[2] != hephaestusAction {
					t.Errorf("%s: %s%s, not %s", w, ref, m[2], hephaestusAction)
				}
			case !regexp.MustCompile(`@[0-9a-f]{40}$`).MatchString(ref):
				t.Errorf("%s: %s is not pinned to a commit", w, ref)
			}
		}
		for _, st := range steps(t, ".github/workflows/"+w) {
			if strings.Contains(st, "actions/checkout@") && !strings.Contains(st, "persist-credentials: false") {
				t.Errorf("%s: a checkout keeps its credentials", w)
			}
		}
	}
}

// TestReleaseTapGate: in release.yml the tap token reaches only the steps
// that run when the plan publishes the tap; a prerelease skips them.
func TestReleaseTapGate(t *testing.T) {
	s := read(t, ".github/workflows/release.yml")
	if !strings.Contains(s, "on:\n  push:\n    tags: [\"v*\"]\n") {
		t.Error("release.yml does not run on v* tags only")
	}
	n := 0
	for _, st := range steps(t, ".github/workflows/release.yml") {
		if !strings.Contains(st, "HOMEBREW_TAP_GITHUB_TOKEN") {
			continue
		}
		n++
		if !strings.Contains(st, "    if: steps.plan.outputs.tap == 'publish'\n") {
			t.Errorf("a step that is not gated by the plan gets the tap token:\n%s", st)
		}
	}
	if n != 2 {
		t.Errorf("%d steps get the tap token, want 2 (dry run, publish)", n)
	}
	for _, want := range []string{
		"a prerelease must not update the tap",
		"if: steps.plan.outputs.tap == 'skip'",
		"verify-tag-provenance.sh --ref \"$GITHUB_REF\" --sha \"$GITHUB_SHA\" main",
		"needs: [provenance, ci, build]",
		"environment: release\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("release.yml: no %q", want)
		}
	}
	// the build job holds no secret and no write permission
	build := s[strings.Index(s, "\n  build:\n"):strings.Index(s, "\n  release:\n")]
	if strings.Contains(build, "secrets.") || strings.Contains(build, "write") {
		t.Error("the build job of release.yml has a secret or a write permission")
	}
}

func TestRehearsalWorkflow(t *testing.T) {
	s := read(t, ".github/workflows/release-rehearsal.yml")
	if strings.Count(s, "config: hephaestus.rehearsal.yaml") != 5 {
		t.Error("release-rehearsal.yml: every Hephaestus run but the checks must use hephaestus.rehearsal.yaml")
	}
	if strings.Contains(s, "homebrew-tap\n") || strings.Contains(s, "magicdrive/homebrew-tap ") {
		t.Error("release-rehearsal.yml names the production tap")
	}
	for _, want := range []string{"environment: release-rehearsal\n", `[[ "$R2" == no-op && "$T2" == no-op && "$T2_NOOP" == true ]]`} {
		if !strings.Contains(s, want) {
			t.Errorf("release-rehearsal.yml: no %q", want)
		}
	}
}

func TestReadmeInstall(t *testing.T) {
	r := read(t, "README.md")
	for _, want := range []string{
		"brew install magicdrive/tap/kjak-notify-macos",
		"https://github.com/magicdrive/kjak-notify-macos/releases",
		"kjak-notify-macos_checksums.txt",
	} {
		if !strings.Contains(r, want) {
			t.Errorf("README.md: no %q", want)
		}
	}
}
