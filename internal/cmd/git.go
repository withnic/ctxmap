package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// changedFiles lists files that differ from HEAD (staged, unstaged and
// untracked) and, when base is set, everything changed since the merge-base
// of base and HEAD. Keys are pathKey(abs path).
func changedFiles(root, base string) (map[string]bool, error) {
	dir, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if st, statErr := os.Stat(dir); statErr == nil && !st.IsDir() {
		dir = filepath.Dir(dir)
	}
	top, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("--changed needs a git repository: %w", err)
	}
	ref := "HEAD"
	if strings.HasPrefix(base, "-") {
		// would otherwise reach git as an option (argument injection)
		return nil, fmt.Errorf("--base %q: not a valid ref", base)
	}
	if base != "" {
		if ref, err = git(dir, "merge-base", base, "HEAD"); err != nil {
			return nil, fmt.Errorf("--base %s: %w", base, err)
		}
	}
	// `git diff <ref>` compares the working tree (incl. the index) with ref.
	diff, err := git(dir, "diff", "--name-only", "--no-renames", ref, "--")
	if err != nil {
		return nil, err
	}
	untracked, err := git(dir, "ls-files", "--others", "--exclude-standard", "--full-name")
	if err != nil {
		return nil, err
	}
	if r, evalErr := filepath.EvalSymlinks(top); evalErr == nil {
		top = r // deleted files cannot be resolved one by one; resolve the root once
	}
	set := map[string]bool{}
	for _, l := range strings.Split(diff+"\n"+untracked, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			set[pathKey(filepath.Join(top, filepath.FromSlash(l)))] = true
		}
	}
	return set, nil
}

func git(dir string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return "", fmt.Errorf("git %s: %s", args[0], msg)
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return strings.TrimSpace(string(out)), nil
}

// pathKey normalizes a path for comparisons. git reports the resolved
// repository root (macOS /var -> /private/var, Windows 8.3 short names
// expanded) with forward slashes, while walked paths keep the spelling the
// user gave, so symlinks and short names are resolved when the path exists;
// Windows paths are also case-insensitive.
func pathKey(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	p = filepath.Clean(p)
	if runtime.GOOS == "windows" {
		p = strings.ToLower(p)
	}
	return p
}
