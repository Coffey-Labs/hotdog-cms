package forms

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const contact = `title: Contact
to: hello@example.org
subject: "Contact: {{ .topic }} from {{ .name }}"
reply_to: email
fields:
  - { name: name, required: true, max: 20 }
  - { name: email, type: email, required: true }
  - { name: topic, type: select, options: [Press, Other] }
  - { name: message, type: textarea, required: true }
  - { name: agree, type: checkbox }
`

func load(t *testing.T, yaml string) *Form {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "forms"), 0o755)
	os.WriteFile(filepath.Join(dir, "forms", "contact.yaml"), []byte(yaml), 0o644)
	fs, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return fs["contact"]
}

func TestCheck(t *testing.T) {
	f := load(t, contact)
	good := map[string][]string{"name": {" Ada "}, "email": {"ada@example.net"}, "topic": {"Press"}, "message": {"Hello\r\nthere"}, "_t": {"x"}, "_website": {""}}
	sub, problems := f.Check(good)
	if problems != nil {
		t.Fatalf("good submission refused: %v", problems)
	}
	if sub.Values["name"] != "Ada" || sub.Values["message"] != "Hello\nthere" {
		t.Errorf("values not cleaned: %q", sub.Values)
	}
	if sub.Subject() != "Contact: Press from Ada" {
		t.Errorf("subject %q", sub.Subject())
	}
	bad := []map[string][]string{
		{"name": {"Ada\r\nBcc: victim@example.com"}, "email": {"a@b.co"}, "message": {"x"}}, // header injection
		{"name": {"Ada"}, "email": {"not-an-address"}, "message": {"x"}},
		{"name": {"Ada"}, "email": {"Ada <a@b.co>"}, "message": {"x"}}, // a name in an address field
		{"name": {"Ada"}, "email": {"a@b.co"}, "topic": {"Sales"}, "message": {"x"}},
		{"name": {strings.Repeat("a", 21)}, "email": {"a@b.co"}, "message": {"x"}},
		{"name": {"Ada"}, "email": {"a@b.co"}},                                   // required message missing
		{"name": {"Ada"}, "email": {"a@b.co"}, "message": {"x"}, "admin": {"1"}}, // undeclared field
		{"name": {"Ada", "Bob"}, "email": {"a@b.co"}, "message": {"x"}},          // sent twice
		{"name": {"Ada"}, "email": {"a@b.co"}, "message": {"x"}, "agree": {"maybe"}},
	}
	for i, v := range bad {
		if _, problems := f.Check(v); problems == nil {
			t.Errorf("bad submission %d accepted: %v", i, v)
		}
	}
}

func TestDefinitionErrors(t *testing.T) {
	cases := []string{
		"to: nobody\nfields: [{name: a}]\n",
		"to: a@b.co\nfields: []\n",
		"to: a@b.co\nfields: [{name: a, type: password}]\n",
		"to: a@b.co\nfields: [{name: a, type: select}]\n",
		"to: a@b.co\nfields: [{name: _t}]\n",
		"to: a@b.co\nreply_to: a\nfields: [{name: a}]\n",
		"to: a@b.co\nsuccess: https://evil.example/\nfields: [{name: a}]\n",
		"to: a@b.co\nfields: [{name: a}]\nunknown_key: 1\n",
	}
	for _, c := range cases {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "forms"), 0o755)
		os.WriteFile(filepath.Join(dir, "forms", "x.yaml"), []byte(c), 0o644)
		if _, err := Load(dir); err == nil {
			t.Errorf("accepted bad definition:\n%s", c)
		}
	}
}

func TestRenderEscapes(t *testing.T) {
	f := load(t, strings.Replace(contact, "title: Contact", "title: Contact\nsubmit: '<script>x</script>'", 1))
	h, err := f.Render("/f.js", "/f.css")
	if err != nil {
		t.Fatal(err)
	}
	s := string(h)
	if strings.Contains(s, "<script>x") {
		t.Error("submit text not escaped")
	}
	for _, want := range []string{`action="/_hotdog/forms/contact"`, `name="_website"`, `name="_t"`, `type="email"`, `maxlength="20"`, `<option>Press</option>`, `required`} {
		if !strings.Contains(s, want) {
			t.Errorf("form is missing %s", want)
		}
	}
}
