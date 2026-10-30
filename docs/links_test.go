// Package docs holds the documentation; its test checks the relative links
// of every Markdown file of the repository.
package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRelativeLinks(t *testing.T) {
	link := regexp.MustCompile(`\]\(([^)\s]+)\)`)
	var files []string
	err := filepath.WalkDir("..", func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == ".git" || d.Name() == "build") {
			return filepath.SkipDir
		}
		if strings.HasSuffix(p, ".md") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil || len(files) < 7 {
		t.Fatalf("%v %v", files, err)
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range link.FindAllStringSubmatch(string(b), -1) {
			target, _, _ := strings.Cut(m[1], "#")
			if target == "" || strings.Contains(target, "://") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(f), target)); err != nil {
				t.Errorf("%s: broken link %s", f, m[1])
			}
		}
	}
}
