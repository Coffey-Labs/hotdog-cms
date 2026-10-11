package editor

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/build"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forms"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/isolate"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/preview"
	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/site"
)

// The form builder: forms/*.yaml as forms, previewed on the page that uses
// them in the site's own styling, saved on a draft. A file's opening comments
// are kept; the rest is written fresh, only the settings that are set.

type formInfo struct {
	Name   string      `json:"name"`
	Form   *forms.Form `json:"form"`
	UsedBy []string    `json:"usedBy"` // pages with form: <name> in their front matter
}

func formsUsedBy(s *site.Site) map[string][]string {
	out := map[string][]string{}
	for _, p := range s.Pages {
		if n, ok := p.Params["form"].(string); ok && p.Source != "" {
			out[n] = append(out[n], "content/"+p.Source)
		}
	}
	return out
}

func (s *Server) apiForms(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, acc, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = st.Branch
	}
	loaded, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), branch)
	if loaded == nil {
		apiError(w, http.StatusBadGateway, "Couldn't load the site: "+trimErr(err))
		return
	}
	all, err := forms.Load(siteDir)
	if err != nil {
		apiError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	used := formsUsedBy(loaded)
	out := []formInfo{}
	for name, fm := range all {
		out = append(out, formInfo{Name: name, Form: fm, UsedBy: used[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	writeJSON(w, http.StatusOK, map[string]any{"forms": out, "commit": commit, "branch": branch, "canWrite": acc.Write, "submissions": st.Submissions.URL != ""})
}

// formYAML writes a form as YAML: its settings in a sensible order, only
// those that are set, each field the same way.
func formYAML(f *forms.Form) ([]byte, error) {
	m := &yaml.Node{Kind: yaml.MappingNode}
	put := func(n *yaml.Node, k string, v any) {
		switch x := v.(type) {
		case string:
			if x == "" {
				return
			}
		case bool:
			if !x {
				return
			}
		case int:
			if x == 0 {
				return
			}
		case []string:
			if len(x) == 0 {
				return
			}
		}
		var val yaml.Node
		_ = val.Encode(v)
		if _, ok := v.([]string); ok {
			val.Style = yaml.FlowStyle
		}
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: k}, &val)
	}
	put(m, "title", f.Title)
	put(m, "to", f.To)
	put(m, "subject", f.Subject)
	put(m, "reply_to", f.ReplyTo)
	put(m, "submit", f.Submit)
	put(m, "success", f.Success)
	put(m, "mailto", f.Mailto)
	put(m, "turnstile_sitekey", f.Turnstile)
	put(m, "store", f.Store)
	fields := &yaml.Node{Kind: yaml.SequenceNode}
	for _, fd := range f.Fields {
		fm := &yaml.Node{Kind: yaml.MappingNode}
		put(fm, "name", fd.Name)
		put(fm, "type", fd.Type)
		put(fm, "label", fd.Label)
		put(fm, "help", fd.Help)
		put(fm, "placeholder", fd.Placeholder)
		put(fm, "required", fd.Required)
		put(fm, "max", fd.Max)
		put(fm, "options", fd.Options)
		fields.Content = append(fields.Content, fm)
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: "fields"}, fields)
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(m); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// leadingComments is a file's opening comment block, kept when it's rewritten.
func leadingComments(raw []byte) string {
	var keep []string
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, "#") || (strings.TrimSpace(l) == "" && len(keep) > 0) {
			keep = append(keep, l)
			continue
		}
		break
	}
	return strings.TrimRight(strings.Join(keep, "\n"), "\n")
}

type formRequest struct {
	Branch     string      `json:"branch"`
	BaseCommit string      `json:"baseCommit"`
	Name       string      `json:"name"`
	Form       *forms.Form `json:"form"`
	Delete     bool        `json:"delete"`
}

// formLive shows the edited form on the first page that uses it.
func (s *Server) formLive(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req formRequest
	if !decode(w, r, &req) || req.Form == nil {
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	loaded, siteDir, commit, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if loaded == nil {
		s.writeError(w, r, err)
		return
	}
	s.ws.buildAndCheck(siteDir, commit)
	raw, err := formYAML(req.Form)
	if err == nil {
		_, err = forms.Parse(req.Name, raw)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	pages := formsUsedBy(loaded)[req.Name]
	if len(pages) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"error": "No page shows this form yet. Add form: " + req.Name + " to a page's front matter to see it here."})
		return
	}
	content, err := readSource(siteDir, pages[0])
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	html, pagePath, err := isolate.RenderPage(siteDir, pages[0], content, build.Options{URL: loaded.Config.URL, Forms: map[string][]byte{req.Name: raw}})
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"error": err.Error()})
		return
	}
	owner := sess.Forge + "|" + sess.Login + "|" + st.ID + "|" + req.Branch + "|form"
	writeJSON(w, http.StatusOK, map[string]any{"url": s.live.show(owner, siteDir+".editor-build", pagePath, html, nil), "page": pages[0]})
}

func (s *Server) formSave(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, st *SiteConfig) {
	var req formRequest
	if !decode(w, r, &req) {
		return
	}
	if !forms.ValidName(req.Name) {
		apiError(w, http.StatusBadRequest, "A form's name is lowercase letters, digits and _, starting with a letter.")
		return
	}
	if req.Branch == "" {
		req.Branch = st.Branch
	}
	_, siteDir, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), req.Branch)
	if siteDir == "" {
		s.writeError(w, r, err)
		return
	}
	rel := "forms/" + req.Name + ".yaml"
	old, readErr := readIn(siteDir, rel)
	exists := readErr == nil
	var data []byte
	msg := "Edit form " + req.Name
	switch {
	case req.Delete:
		if !exists {
			apiError(w, http.StatusNotFound, "There's no such form.")
			return
		}
		msg = "Delete form " + req.Name
	case req.Form == nil:
		apiError(w, http.StatusBadRequest, "No form sent.")
		return
	default:
		raw, err := formYAML(req.Form)
		if err == nil {
			_, err = forms.Parse(req.Name, raw)
		}
		if err != nil {
			apiError(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
		if c := leadingComments(old); c != "" {
			raw = append([]byte(c+"\n"), raw...)
		}
		data = raw
		if !exists {
			msg = "New form " + req.Name
		}
	}
	if !exists && !req.Delete && !errors.Is(readErr, fs.ErrNotExist) {
		apiError(w, http.StatusBadGateway, trimErr(readErr))
		return
	}
	target, create := req.Branch, false
	if req.Branch == st.Branch {
		target, create = s.draftName(r, sess, f, st, "form-"+req.Name+".md"), true
	}
	commit, err := s.ws.save(sess, f.Kind, st, s.cloneURL(st), req.Branch, target, create, req.BaseCommit, map[string][]byte{repoFile(st, rel): data}, msg)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"branch": target, "commit": commit, "created": create, "preview": st.Preview.url(preview.Slug(target))})
}

// apiSubmissions reads a form's stored submissions through the endpoint's
// viewer, for people who can edit the site. The editor holds the viewer's
// token; visitors' submissions never pass through the public endpoint.
func (s *Server) apiSubmissions(w http.ResponseWriter, r *http.Request, sess *Session, f *ForgeConfig, id string) {
	st, acc, err := s.siteByID(r.Context(), sess, f, id)
	if s.signedOut(w, r, err) {
		return
	}
	if err != nil || st == nil {
		apiError(w, http.StatusNotFound, "No such site, or you can't open it.")
		return
	}
	if !acc.Write {
		apiError(w, http.StatusForbidden, "Only people who can edit the site can read its form submissions.")
		return
	}
	sc := st.Submissions
	if sc.URL == "" {
		apiError(w, http.StatusNotFound, "This editor isn't set up to read the site's submissions (submissions: in editor.yaml).")
		return
	}
	form := r.URL.Query().Get("form")
	if !forms.ValidName(form) {
		apiError(w, http.StatusBadRequest, "No such form.")
		return
	}
	host := sc.Site
	if host == "" {
		loaded, _, _, err := s.ws.checkout(sess, f.Kind, st, s.cloneURL(st), st.Branch)
		if loaded == nil {
			apiError(w, http.StatusBadGateway, "Couldn't load the site: "+trimErr(err))
			return
		}
		if u, err := url.Parse(loaded.Config.URL); err == nil {
			host = u.Hostname()
		}
	}
	q := url.Values{"limit": {r.URL.Query().Get("limit")}, "offset": {r.URL.Query().Get("offset")}}
	req, _ := http.NewRequestWithContext(r.Context(), http.MethodGet, strings.TrimRight(sc.URL, "/")+"/sites/"+url.PathEscape(host)+"/forms/"+form+"?"+q.Encode(), nil)
	req.Header.Set("Authorization", "Bearer "+os.Getenv(sc.TokenEnv))
	res, err := s.http.Do(req)
	if err != nil {
		apiError(w, http.StatusBadGateway, "Couldn't reach the submissions viewer: "+trimErr(err))
		return
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode != http.StatusOK {
		apiError(w, http.StatusBadGateway, "The submissions viewer answered "+res.Status+": "+strings.TrimSpace(string(body)))
		return
	}
	s.log.Printf("%s read submissions of %s on %s", sess.Login, form, st.ID)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}
