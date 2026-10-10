package forms

import (
	"bytes"
	_ "embed"
	"html/template"
)

// The script and styles every form uses. The build publishes them with
// fingerprinted names when a site has forms.
var (
	//go:embed static/hotdog-forms.js
	Script []byte
	//go:embed static/hotdog-forms.css
	Style []byte
)

var formTpl = template.Must(template.New("form").Parse(`<link rel="stylesheet" href="{{ .CSS }}">
<form class="hotdog-form" method="post" action="/_hotdog/forms/{{ .F.Name }}" data-hotdog-form="{{ .F.Name }}"{{ with .F.Success }} data-success="{{ . }}"{{ end }}>
{{- range .F.Fields }}{{ $id := printf "cl-%s-%s" $.F.Name .Name }}
  <div class="hotdog-field cl-{{ .Type }}">
    {{- if eq .Type "checkbox" }}
    <label for="{{ $id }}"><input id="{{ $id }}" name="{{ .Name }}" type="checkbox"{{ if .Required }} required{{ end }} aria-describedby="{{ $id }}-error{{ if .Help }} {{ $id }}-help{{ end }}"> {{ .Label }}{{ if .Required }}<span class="hotdog-req" aria-hidden="true"> *</span>{{ end }}</label>
    {{- else }}
    <label for="{{ $id }}">{{ .Label }}{{ if .Required }}<span class="hotdog-req" aria-hidden="true"> *</span>{{ end }}</label>
    {{- if eq .Type "textarea" }}
    <textarea id="{{ $id }}" name="{{ .Name }}" rows="6" maxlength="{{ .MaxLen }}"{{ if .Required }} required{{ end }}{{ with .Placeholder }} placeholder="{{ . }}"{{ end }} aria-describedby="{{ $id }}-error{{ if .Help }} {{ $id }}-help{{ end }}"></textarea>
    {{- else if eq .Type "select" }}
    <select id="{{ $id }}" name="{{ .Name }}"{{ if .Required }} required{{ end }} aria-describedby="{{ $id }}-error{{ if .Help }} {{ $id }}-help{{ end }}">
      <option value="">Choose one</option>
      {{- range .Options }}
      <option>{{ . }}</option>
      {{- end }}
    </select>
    {{- else }}
    <input id="{{ $id }}" name="{{ .Name }}" type="{{ .Type }}" maxlength="{{ .MaxLen }}"{{ if .Required }} required{{ end }}{{ with .Autocomplete }} autocomplete="{{ . }}"{{ end }}{{ with .Placeholder }} placeholder="{{ . }}"{{ end }} aria-describedby="{{ $id }}-error{{ if .Help }} {{ $id }}-help{{ end }}">
    {{- end }}
    {{- end }}
    {{- with .Help }}
    <p class="hotdog-help" id="{{ $id }}-help">{{ . }}</p>
    {{- end }}
    <p class="hotdog-error" id="{{ $id }}-error"></p>
  </div>
{{- end }}
  <div class="hotdog-hp" aria-hidden="true"><label>Leave this empty <input name="_website" tabindex="-1" autocomplete="off"></label></div>
  <input type="hidden" name="_t" value="">
  {{- with .F.Turnstile }}
  <div class="cf-turnstile" data-sitekey="{{ . }}"></div>
  <script src="https://challenges.cloudflare.com/turnstile/v0/api.js" async defer></script>
  {{- end }}
  <p class="hotdog-status" role="status" aria-live="polite"></p>
  <button type="submit">{{ .F.Submit }}</button>
  <noscript><p class="hotdog-noscript">This form uses JavaScript to keep spam out.{{ with .F.Mailto }} You can also email <a href="mailto:{{ . }}">{{ . }}</a>.{{ end }}</p></noscript>
</form>
<script src="{{ .JS }}" defer></script>
`))

// MaxLen is the field's length limit, for the maxlength attribute.
func (f Field) MaxLen() int { return f.maxLen() }

// Autocomplete is the browser autofill hint for the field, if one fits.
func (f Field) Autocomplete() string {
	switch f.Type {
	case "email":
		return "email"
	case "tel":
		return "tel"
	case "url":
		return "url"
	}
	if f.Name == "name" {
		return "name"
	}
	return ""
}

// Render returns the form's HTML. js and css are the published addresses of
// the forms script and stylesheet.
func (f *Form) Render(js, css string) (template.HTML, error) {
	var b bytes.Buffer
	if err := formTpl.Execute(&b, map[string]any{"F": f, "JS": js, "CSS": css}); err != nil {
		return "", err
	}
	// Safe: produced by html/template, which escaped every value from the YAML.
	return template.HTML(b.String()), nil
}
