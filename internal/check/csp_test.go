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
