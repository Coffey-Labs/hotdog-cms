package editor

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"git.coffeylabs.org/coffey-labs/hotdog-cms/internal/forge"
)

// Signing in is the platform's OAuth authorization-code flow with PKCE.
// The editor never sees a password. It asks for the person's profile and for
// repository access, which saving needs (pushing draft branches, opening and
// merging pull requests); the platform still decides, per repository, what
// that person may actually do. The state and PKCE verifier travel in a
// sealed, short-lived cookie rather than in server memory.

type endpoints struct{ authorize, token, user, scope string }

func (f *ForgeConfig) endpoints() endpoints {
	b := f.base()
	switch f.Kind {
	case forge.GitHub:
		api := "https://api.github.com"
		if !strings.EqualFold(f.Host, "github.com") {
			api = b + "/api/v3"
		}
		return endpoints{b + "/login/oauth/authorize", b + "/login/oauth/access_token", api + "/user", "read:user repo"}
	case forge.GitLab:
		return endpoints{b + "/oauth/authorize", b + "/oauth/token", b + "/api/v4/user", "read_user api"}
	case forge.Bitbucket:
		return endpoints{"https://bitbucket.org/site/oauth2/authorize", "https://bitbucket.org/site/oauth2/access_token", "https://api.bitbucket.org/2.0/user", ""}
	default: // Gitea, Forgejo
		return endpoints{b + "/login/oauth/authorize", b + "/login/oauth/access_token", b + "/api/v1/user", "read:user write:repository"}
	}
}

type oauthState struct {
	Forge    string    `json:"f"`
	State    string    `json:"s"`
	Verifier string    `json:"v"`
	Return   string    `json:"r"`
	Expires  time.Time `json:"e"`
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (s *Server) redirectURI() string { return s.cfg.PublicURL + "/auth/callback" }

// login sends the browser to the platform's consent page.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	f := s.cfg.forge(r.URL.Query().Get("forge"))
	if f == nil {
		http.Error(w, "no such platform here", http.StatusBadRequest)
		return
	}
	st := oauthState{Forge: f.Host, State: randomString(24), Verifier: randomString(48), Return: safeReturn(r.URL.Query().Get("return")), Expires: time.Now().Add(10 * time.Minute)}
	if err := s.seal.setCookie(w, r, stateCookie, st, 10*time.Minute); err != nil {
		http.Error(w, "couldn't start sign-in", http.StatusInternalServerError)
		return
	}
	sum := sha256.Sum256([]byte(st.Verifier))
	q := url.Values{
		"client_id": {f.ClientID}, "redirect_uri": {s.redirectURI()}, "response_type": {"code"},
		"state": {st.State}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"},
	}
	if sc := f.endpoints().scope; sc != "" {
		q.Set("scope", sc)
	}
	http.Redirect(w, r, f.endpoints().authorize+"?"+q.Encode(), http.StatusFound)
}

// safeReturn keeps the after-sign-in destination on the editor itself.
func safeReturn(p string) string {
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") || strings.ContainsAny(p, "\\\r\n") {
		return "/"
	}
	return p
}

// callback finishes sign-in: checks state, trades the code for a token,
// asks the platform who this is, and seals it all into the session cookie.
func (s *Server) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(stateCookie)
	var st oauthState
	if err != nil || s.seal.open(stateCookie, c.Value, &st) != nil || time.Now().After(st.Expires) {
		http.Error(w, "Sign-in expired or was started elsewhere. Start again from the editor.", http.StatusBadRequest)
		return
	}
	clearCookie(w, r, stateCookie)
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		http.Error(w, "The platform declined the sign-in: "+e, http.StatusForbidden)
		return
	}
	if q.Get("state") == "" || q.Get("state") != st.State {
		http.Error(w, "Sign-in state didn't match. Start again from the editor.", http.StatusBadRequest)
		return
	}
	f := s.cfg.forge(st.Forge)
	if f == nil {
		http.Error(w, "no such platform here", http.StatusBadRequest)
		return
	}
	token, err := s.exchange(r.Context(), f, q.Get("code"), st.Verifier)
	if err != nil {
		s.log.Printf("sign-in with %s: %v", f.Host, err)
		http.Error(w, "The platform didn't accept the sign-in. Try again.", http.StatusBadGateway)
		return
	}
	login, name, email, err := s.whoami(r.Context(), f, token)
	if err != nil {
		s.log.Printf("sign-in with %s: %v", f.Host, err)
		http.Error(w, "Couldn't read your profile from the platform.", http.StatusBadGateway)
		return
	}
	sess := Session{Forge: f.Host, Login: login, Name: name, Email: email, Token: token, Expires: time.Now().Add(sessionTTL)}
	if err := s.seal.setCookie(w, r, sessionCookie, sess, sessionTTL); err != nil {
		http.Error(w, "couldn't start the session", http.StatusInternalServerError)
		return
	}
	s.log.Printf("%s signed in with %s", login, f.Host)
	http.Redirect(w, r, st.Return, http.StatusFound)
}

func (s *Server) exchange(ctx context.Context, f *ForgeConfig, code, verifier string) (string, error) {
	secret := os.Getenv(f.ClientSecretEnv)
	if secret == "" {
		return "", fmt.Errorf("%s is not set", f.ClientSecretEnv)
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {s.redirectURI()}, "code_verifier": {verifier}, "client_id": {f.ClientID}}
	if f.Kind != forge.Bitbucket {
		form.Set("client_secret", secret)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.endpoints().token, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if f.Kind == forge.Bitbucket {
		req.SetBasicAuth(f.ClientID, secret)
	}
	res, err := s.http.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	var d struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err := json.Unmarshal(body, &d); err != nil {
		return "", fmt.Errorf("token response: %s", res.Status)
	}
	if d.AccessToken == "" {
		return "", fmt.Errorf("no token: %s %s", d.Error, d.Description)
	}
	return d.AccessToken, nil
}

func (s *Server) whoami(ctx context.Context, f *ForgeConfig, token string) (login, name, email string, err error) {
	var d struct {
		Login       string `json:"login"`
		Username    string `json:"username"`
		Nickname    string `json:"nickname"`
		Name        string `json:"name"`
		FullName    string `json:"full_name"`
		DisplayName string `json:"display_name"`
		Email       string `json:"email"`
	}
	if err := s.api(ctx, f, token, f.endpoints().user, &d); err != nil {
		return "", "", "", err
	}
	login = firstNonEmpty(d.Login, d.Username, d.Nickname)
	name = firstNonEmpty(d.FullName, d.Name, d.DisplayName, login)
	if login == "" {
		return "", "", "", errors.New("the platform didn't say who you are")
	}
	// The address commits are attributed to: the one the platform shows, or
	// a no-reply address on the platform when the person keeps theirs private.
	email = d.Email
	if email == "" {
		if f.Kind == forge.GitHub {
			email = login + "@users.noreply.github.com"
		} else {
			email = login + "@noreply." + f.Host
		}
	}
	return login, name, email, nil
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

// api GETs a platform API URL as the signed-in person and decodes JSON.
func (s *Server) api(ctx context.Context, f *ForgeConfig, token, u string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "hotdog-cms-editor")
	res, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusUnauthorized {
		return errSignedOut
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", u, res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 8<<20)).Decode(v)
}

var errSignedOut = errors.New("the platform no longer accepts this session")
