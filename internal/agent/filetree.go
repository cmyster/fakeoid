package agent

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScanFileTree returns a text representation of the directory tree rooted at dir,
// limited to maxDepth levels and excluding common noise directories.
// Output is capped at 200 lines; if exceeded, a truncation message is appended.
func ScanFileTree(dir string, maxDepth int) (string, error) {
	excludes := map[string]bool{
		".git":        true,
		"node_modules": true,
		"__pycache__":  true,
		".venv":       true,
		"vendor":      true,
		"build":       true,
		"dist":        true,
		".fakeoid":    true,
	}

	const maxLines = 200
	var lines []string
	var extraCount int

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		rel, _ := filepath.Rel(dir, path)
		if rel == "." {
			return nil // skip root itself
		}

		depth := strings.Count(rel, string(filepath.Separator))
		if depth >= maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() && excludes[d.Name()] {
			return filepath.SkipDir
		}

		// Skip symlinked directories that point outside the root
		if d.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err == nil {
				info, statErr := os.Stat(resolved)
				if statErr == nil && info.IsDir() {
					if !strings.HasPrefix(resolved, dir+"/") && resolved != dir {
						return filepath.SkipDir
					}
				}
			}
		}

		if len(lines) >= maxLines {
			extraCount++
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		indent := strings.Repeat("  ", depth)
		if d.IsDir() {
			lines = append(lines, fmt.Sprintf("%s%s/", indent, d.Name()))
		} else {
			lines = append(lines, fmt.Sprintf("%s%s", indent, d.Name()))
		}

		return nil
	})

	var buf strings.Builder
	for _, line := range lines {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if extraCount > 0 {
		fmt.Fprintf(&buf, "... (%d more entries)\n", extraCount)
	}

	return buf.String(), err
}
