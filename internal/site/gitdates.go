package site

import (
	"bytes"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

// gitDates dates the files under content/ by git history, for `lastmod:
// git`: the last commit that touched each one, keyed by its path under
// content/ (slash-separated). A file with changes not yet committed, or not
// yet added, is dated today, so building before a commit gives the date the
// commit will have. Outside a git repository, or without git, it returns
// nothing and pages keep their front matter dates.
//
// Only history git already has is read: in a shallow clone, files last
// changed before the clone's depth have no date.
func gitDates(dir string, today time.Time) map[string]time.Time {
	prefix, err := git(dir, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	content := strings.TrimSpace(string(prefix)) + "content/"
	dates := map[string]time.Time{}
	// Newest commit first: the first date seen for a file is its latest.
	// Merge commits list no files, so a file gets its own commit's date.
	out, err := git(dir, "log", "--format=%x00%cI", "--name-only", "--no-renames", "-z", "--", "content")
	if err != nil {
		return nil
	}
	var when time.Time
	for _, f := range bytes.Split(out, []byte{0}) {
		s := strings.Trim(string(f), "\n")
		if s == "" {
			continue
		}
		// A commit's date comes right after the NUL that --format starts
		// with; its files follow, one per NUL.
		if t, err := time.Parse(time.RFC3339, s); err == nil && !strings.Contains(s, "/") {
			when = t
			continue
		}
		rel, ok := strings.CutPrefix(s, content)
		if !ok || when.IsZero() {
			continue
		}
		if _, seen := dates[rel]; !seen {
			dates[rel] = when
		}
	}
	// Changed, added or new files under content/: today.
	out, err = git(dir, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames", "--", "content")
	if err != nil {
		return dates
	}
	for _, e := range bytes.Split(out, []byte{0}) {
		if len(e) < 4 || e[2] != ' ' {
			continue
		}
		if rel, ok := strings.CutPrefix(string(e[3:]), content); ok && path.Clean(rel) == rel {
			dates[rel] = today
		}
	}
	return dates
}

// git runs a read-only git command in dir. Optional locks are off, so it
// never writes to the index, and the file-system monitor is off, so a
// repository's own config can't make it run a program.
func git(dir string, args ...string) ([]byte, error) {
	args = append([]string{"-C", dir, "--no-optional-locks", "-c", "core.fsmonitor=false", "-c", "core.quotePath=false"}, args...)
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0")
	return cmd.Output()
}
