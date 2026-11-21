package check

import (
	"strings"
	"testing"
)

func TestPolicyFramesAndObjects(t *testing.T) {
	// No frames or objects: both stay shut.
	p := policy([]*page{{}})
	if !strings.Contains(p, "frame-src 'none'") || !strings.Contains(p, "object-src 'none'") {
		t.Errorf("empty page: %s", p)
	}
	// A diagram embedded from the site itself: Chrome checks it against
	// frame-src as well as object-src, so both must allow 'self'.
	p = policy([]*page{{objects: []string{"/img/diagram.1234567890.svg"}}})
	if !strings.Contains(p, "frame-src 'self'") || !strings.Contains(p, "object-src 'self'") {
		t.Errorf("own object: %s", p)
	}
	// A same-site iframe and a third-party one.
	p = policy([]*page{{frames: []string{"/embed/map/", "https://video.example.com/v/1"}}})
	if !strings.Contains(p, "frame-src 'self' https://video.example.com") || !strings.Contains(p, "object-src 'none'") {
		t.Errorf("frames: %s", p)
	}
	// An <embed> from another host.
	p = policy([]*page{{objects: []string{"https://files.example.org/a.pdf"}}})
	if !strings.Contains(p, "object-src https://files.example.org") || !strings.Contains(p, "frame-src https://files.example.org") {
		t.Errorf("third-party object: %s", p)
	}
}

func TestPolicyTurnstileFrame(t *testing.T) {
	// Turnstile's script draws the check in a frame from its own host.
	p := policy([]*page{{scripts: []string{"https://challenges.cloudflare.com/turnstile/v0/api.js"}}})
	if !strings.Contains(p, "script-src 'self' https://challenges.cloudflare.com;") || !strings.Contains(p, "frame-src https://challenges.cloudflare.com;") {
		t.Errorf("turnstile: %s", p)
	}
	// Any other third-party script leaves frames shut.
	p = policy([]*page{{scripts: []string{"https://cdn.example.net/x.js"}}})
	if !strings.Contains(p, "frame-src 'none'") {
		t.Errorf("plain script: %s", p)
	}
}

func TestPolicyFormAction(t *testing.T) {
	p := policy([]*page{{forms: []string{"/_hotdog/form/contact"}}})
	if !strings.Contains(p, "form-action 'self';") {
		t.Errorf("own form: %s", p)
	}
	p = policy([]*page{{forms: []string{"https://shop.example.com/cart/checkout", "https://pay.example.org/x"}}, {forms: []string{"https://shop.example.com/cart/checkout?x=1"}}})
	if !strings.Contains(p, "form-action 'self' https://pay.example.org https://shop.example.com;") {
		t.Errorf("checkout forms: %s", p)
	}
}
