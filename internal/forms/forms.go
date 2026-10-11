// Package forms defines the forms a site declares in forms/*.yaml: how they
// render in a page and how a submission is checked. The build uses it to
// render and validate; the endpoint uses it to accept and deliver.
//
// A form accepts the fields it declares and nothing else. Each field has a
// type, a length limit and, for a select, the options it may take; single-line
// fields refuse line breaks, so nothing typed into one can add an email header.
package forms

import (
	"bytes"
	"fmt"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Form is one forms/<name>.yaml.
type Form struct {
	Name    string  `yaml:"-" json:"name"`
	Title   string  `yaml:"title" json:"title,omitempty"`
	To      string  `yaml:"to" json:"to,omitempty"`             // where submissions are emailed
	Subject string  `yaml:"subject" json:"subject,omitempty"`   // a template over the fields: "Contact: {{ .topic }}"
	ReplyTo string  `yaml:"reply_to" json:"reply_to,omitempty"` // the email field to reply to, if any
	Fields  []Field `yaml:"fields" json:"fields,omitempty"`
	Submit  string  `yaml:"submit" json:"submit,omitempty"`   // the button's text
	Success string  `yaml:"success" json:"success,omitempty"` // where to send the visitor after a good submission
	// Turnstile adds Cloudflare Turnstile with this site key; the endpoint
	// checks it with HOTDOG_TURNSTILE_SECRET.
	Turnstile string `yaml:"turnstile_sitekey" json:"turnstile_sitekey,omitempty"`
	// Store appends each submission to a JSON-lines file as well as emailing it.
	Store bool `yaml:"store" json:"store,omitempty"`
	// Mailto is shown to visitors without JavaScript, who can't fetch the
	// form's anti-spam token, as another way to get in touch.
	Mailto string `yaml:"mailto" json:"mailto,omitempty"`

	subject *template.Template
}

// Field is one input.
type Field struct {
	Name     string   `yaml:"name" json:"name,omitempty"`
	Type     string   `yaml:"type" json:"type,omitempty"` // text, email, url, tel, textarea, select, checkbox
	Label    string   `yaml:"label" json:"label,omitempty"`
	Help     string   `yaml:"help" json:"help,omitempty"`
	Required bool     `yaml:"required" json:"required,omitempty"`
	Max      int      `yaml:"max" json:"max,omitempty"` // characters; defaults per type
	Options  []string `yaml:"options" json:"options,omitempty"`
	// Placeholder is shown inside an empty field.
	Placeholder string `yaml:"placeholder" json:"placeholder,omitempty"`
}

var nameRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// Reserved names the endpoint uses for its own fields.
var reserved = map[string]bool{"_t": true, "_website": true, "cf-turnstile-response": true}

func (f *Field) maxLen() int {
	if f.Max > 0 {
		return f.Max
	}
	switch f.Type {
	case "textarea":
		return 5000
	case "email", "url":
		return 320
	}
	return 200
}

// Load reads every form in a site's forms/ folder. A site without the folder
// has no forms.
func Load(siteDir string) (map[string]*Form, error) {
	dir := filepath.Join(siteDir, "forms")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return map[string]*Form{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]*Form{}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		f, err := Parse(name, raw)
		if err != nil {
			return nil, fmt.Errorf("forms/%s: %w", e.Name(), err)
		}
		out[name] = f
	}
	return out, nil
}

// Parse reads one form's YAML, as Load does for each file.
func Parse(name string, raw []byte) (*Form, error) {
	var f Form
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		return nil, err
	}
	f.Name = name
	if err := f.check(); err != nil {
		return nil, err
	}
	return &f, nil
}

// ValidName reports whether name can be a form's name (its file and field).
func ValidName(name string) bool { return nameRe.MatchString(name) }

func (f *Form) check() error {
	if !nameRe.MatchString(f.Name) {
		return fmt.Errorf("form names are lower case letters, digits and _, starting with a letter")
	}
	if _, err := mail.ParseAddress(f.To); err != nil {
		return fmt.Errorf("to %q is not an email address", f.To)
	}
	if len(f.Fields) == 0 {
		return fmt.Errorf("no fields")
	}
	seen := map[string]bool{}
	for i := range f.Fields {
		fd := &f.Fields[i]
		if !nameRe.MatchString(fd.Name) || reserved[fd.Name] {
			return fmt.Errorf("field %q: names are lower case letters, digits and _", fd.Name)
		}
		if seen[fd.Name] {
			return fmt.Errorf("field %q appears twice", fd.Name)
		}
		seen[fd.Name] = true
		switch fd.Type {
		case "":
			fd.Type = "text"
		case "text", "email", "url", "tel", "textarea", "checkbox":
		case "select":
			if len(fd.Options) == 0 {
				return fmt.Errorf("field %q: a select needs options", fd.Name)
			}
		default:
			return fmt.Errorf("field %q: type %q is not text, email, url, tel, textarea, select or checkbox", fd.Name, fd.Type)
		}
		if fd.Label == "" {
			fd.Label = strings.ToUpper(fd.Name[:1]) + strings.ReplaceAll(fd.Name[1:], "_", " ")
		}
	}
	if f.ReplyTo != "" {
		ok := false
		for _, fd := range f.Fields {
			if fd.Name == f.ReplyTo && fd.Type == "email" {
				ok = true
			}
		}
		if !ok {
			return fmt.Errorf("reply_to %q is not an email field of this form", f.ReplyTo)
		}
	}
	if f.Subject == "" {
		f.Subject = "Website form: " + f.Name
	}
	t, err := template.New("subject").Option("missingkey=zero").Parse(f.Subject)
	if err != nil {
		return fmt.Errorf("subject: %w", err)
	}
	f.subject = t
	if f.Submit == "" {
		f.Submit = "Send"
	}
	if f.Success != "" && !strings.HasPrefix(f.Success, "/") {
		return fmt.Errorf("success must be a path on this site, such as /contact/thanks/")
	}
	if f.Mailto != "" {
		if _, err := mail.ParseAddress(f.Mailto); err != nil {
			return fmt.Errorf("mailto %q is not an email address", f.Mailto)
		}
	}
	return nil
}

// Submission is a checked set of values, in the form's field order.
type Submission struct {
	Form   *Form
	Values map[string]string
}

// Problem is a field that didn't pass, in words a visitor can act on.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Check validates submitted values against the form. Anything the form
// doesn't declare is refused outright rather than ignored: a field nobody
// asked for is either a mistake or a probe.
func (f *Form) Check(values map[string][]string) (*Submission, []Problem) {
	var problems []Problem
	known := map[string]bool{}
	for _, fd := range f.Fields {
		known[fd.Name] = true
	}
	var extra []string
	for k := range values {
		if !known[k] && !reserved[k] {
			extra = append(extra, k)
		}
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		return nil, []Problem{{Field: "", Message: "This form doesn't take: " + strings.Join(extra, ", ")}}
	}
	sub := &Submission{Form: f, Values: map[string]string{}}
	for _, fd := range f.Fields {
		vs := values[fd.Name]
		if len(vs) > 1 {
			problems = append(problems, Problem{fd.Name, fd.Label + " was sent more than once."})
			continue
		}
		v := ""
		if len(vs) == 1 {
			v = vs[0]
		}
		if !utf8.ValidString(v) || strings.ContainsRune(v, 0) {
			problems = append(problems, Problem{fd.Name, fd.Label + " contains characters that can't be read."})
			continue
		}
		if fd.Type != "textarea" {
			v = strings.TrimSpace(v)
			if strings.ContainsAny(v, "\r\n") {
				problems = append(problems, Problem{fd.Name, fd.Label + " must be a single line."})
				continue
			}
		} else {
			v = strings.ReplaceAll(strings.TrimSpace(v), "\r\n", "\n")
		}
		if fd.Type == "checkbox" {
			if v != "" && v != "on" && v != "yes" {
				problems = append(problems, Problem{fd.Name, fd.Label + " has an unexpected value."})
				continue
			}
			if v != "" {
				v = "yes"
			}
		}
		if v == "" {
			if fd.Required {
				problems = append(problems, Problem{fd.Name, fd.Label + " is required."})
			}
			sub.Values[fd.Name] = ""
			continue
		}
		if n := utf8.RuneCountInString(v); n > fd.maxLen() {
			problems = append(problems, Problem{fd.Name, fmt.Sprintf("%s is too long (%d characters, at most %d).", fd.Label, n, fd.maxLen())})
			continue
		}
		switch fd.Type {
		case "email":
			a, err := mail.ParseAddress(v)
			if err != nil || a.Name != "" || !strings.Contains(a.Address, ".") {
				problems = append(problems, Problem{fd.Name, fd.Label + " doesn't look like an email address."})
				continue
			}
			v = a.Address
		case "url":
			if !strings.HasPrefix(v, "https://") && !strings.HasPrefix(v, "http://") {
				problems = append(problems, Problem{fd.Name, fd.Label + " should start with https://."})
				continue
			}
		case "select":
			ok := false
			for _, o := range fd.Options {
				if v == o {
					ok = true
				}
			}
			if !ok {
				problems = append(problems, Problem{fd.Name, fd.Label + " has a value that isn't one of the choices."})
				continue
			}
		}
		sub.Values[fd.Name] = v
	}
	if len(problems) > 0 {
		return nil, problems
	}
	return sub, nil
}

// Subject renders the email subject from the values, on one line.
func (s *Submission) Subject() string {
	var b strings.Builder
	if err := s.Form.subject.Execute(&b, s.Values); err != nil {
		return "Website form: " + s.Form.Name
	}
	out := strings.Join(strings.Fields(b.String()), " ")
	if r := []rune(out); len(r) > 200 {
		out = string(r[:200])
	}
	return out
}

// Text is the submission as a plain-text email body.
func (s *Submission) Text() string {
	var b strings.Builder
	for _, fd := range s.Form.Fields {
		v := s.Values[fd.Name]
		if v == "" {
			v = "(left blank)"
		}
		if fd.Type == "textarea" {
			fmt.Fprintf(&b, "%s:\n%s\n\n", fd.Label, v)
		} else {
			fmt.Fprintf(&b, "%s: %s\n", fd.Label, v)
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
