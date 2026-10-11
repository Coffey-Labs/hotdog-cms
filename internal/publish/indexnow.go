package publish

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
)

// IndexNow: after a publish, tell search engines (Bing, Yandex, Seznam and
// others that share IndexNow) which pages changed, were added or went away,
// so they re-read those instead of guessing. Only for targets that ask for
// it (indexnow: true), only when the site has a key, and only what changed
// since that target's last publish, which is remembered in the user's cache
// folder. A failed ping is reported, never a failed publish.

// indexNowEndpoint is where pings go; tests point it elsewhere.
var indexNowEndpoint = "https://api.indexnow.org/indexnow"

func readManifest(data []byte) map[string]string {
	out := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		if p, h, ok := strings.Cut(line, "\t"); ok {
			out[p] = h
		}
	}
	return out
}

func statePath(name string, t Target) (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(name + "\x00" + t.Type + "\x00" + t.Host + "\x00" + t.Path + "\x00" + t.Image))
	return filepath.Join(base, "hotdog-cms", "indexnow", hex.EncodeToString(sum[:8])), nil
}

// changedPages compares two manifests: pages added or changed, then removed.
func changedPages(before, after map[string]string) []string {
	var out []string
	for p, h := range after {
		if before[p] != h {
			out = append(out, p)
		}
	}
	for p := range before {
		if _, ok := after[p]; !ok {
			out = append(out, p) // gone: search engines drop it on seeing the 404
		}
	}
	sort.Strings(out)
	return out
}

// indexNow pings for what changed since this target's last publish.
func indexNow(name string, t Target, out string, log io.Writer) {
	note, err := os.ReadFile(filepath.Join(out, build.IndexNowFile))
	if err != nil {
		fmt.Fprintln(log, "indexnow: the site has no indexnow.key in site.yaml; nothing sent")
		return
	}
	var info build.IndexNowInfo
	if json.Unmarshal(note, &info) != nil || info.Key == "" {
		return
	}
	now, err := os.ReadFile(filepath.Join(out, build.PagesManifest))
	if err != nil {
		return
	}
	state, err := statePath(name, t)
	if err != nil {
		return
	}
	before, _ := os.ReadFile(state)
	pages := changedPages(readManifest(before), readManifest(now))
	if len(pages) == 0 {
		fmt.Fprintln(log, "indexnow: no pages changed")
		return
	}
	u, err := url.Parse(info.URL)
	if err != nil {
		return
	}
	for start := 0; start < len(pages); start += 10000 {
		batch := pages[start:min(start+10000, len(pages))]
		urls := make([]string, len(batch))
		for i, p := range batch {
			urls[i] = strings.TrimRight(info.URL, "/") + p
		}
		body, _ := json.Marshal(map[string]any{"host": u.Hostname(), "key": info.Key, "keyLocation": strings.TrimRight(info.URL, "/") + "/" + info.Key + ".txt", "urlList": urls})
		req, _ := http.NewRequest(http.MethodPost, indexNowEndpoint, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		res, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			fmt.Fprintf(log, "indexnow: couldn't send: %v (the publish itself went fine)\n", err)
			return
		}
		res.Body.Close()
		if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusAccepted {
			fmt.Fprintf(log, "indexnow: search engines answered %s; is %s/%s.txt live yet? (the publish itself went fine)\n", res.Status, strings.TrimRight(info.URL, "/"), info.Key)
			return
		}
	}
	fmt.Fprintf(log, "indexnow: told search engines about %d changed page(s)\n", len(pages))
	if err := os.MkdirAll(filepath.Dir(state), 0o700); err == nil {
		_ = os.WriteFile(state, now, 0o600)
	}
}
