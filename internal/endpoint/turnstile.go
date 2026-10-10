package endpoint

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var turnstileURL = "https://challenges.cloudflare.com/turnstile/v0/siteverify"

// turnstileOK asks Cloudflare whether a Turnstile response is genuine.
func turnstileOK(ctx context.Context, secret, response, ip string) bool {
	if secret == "" || response == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	form := url.Values{"secret": {secret}, "response": {response}}
	if ip != "" {
		form.Set("remoteip", ip)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, turnstileURL, strings.NewReader(form.Encode()))
	if err != nil {
		return false
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	var d struct {
		Success bool `json:"success"`
	}
	return json.NewDecoder(res.Body).Decode(&d) == nil && d.Success
}
