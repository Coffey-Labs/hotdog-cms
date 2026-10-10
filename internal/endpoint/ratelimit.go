package endpoint

import (
	"sync"
	"time"
)

// limiter counts attempts per key in a sliding window, in memory. Behind a
// load balancer each endpoint counts on its own, so the effective limit is
// the configured one times the number of endpoints; for a contact form that
// is fine. A shared limit would need shared state, which is the first thing
// here that would ask for a data store.
type limiter struct {
	mu     sync.Mutex
	count  int
	window time.Duration
	hits   map[string][]time.Time
	calls  int
}

func newLimiter(count int, window time.Duration) *limiter {
	return &limiter{count: count, window: window, hits: map[string][]time.Time{}}
}

// allow records an attempt and reports whether it is within the limit.
func (l *limiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.calls%1000 == 0 { // now and then, forget addresses that have gone quiet
		for k, ts := range l.hits {
			if len(ts) == 0 || now.Sub(ts[len(ts)-1]) > l.window {
				delete(l.hits, k)
			}
		}
	}
	ts := l.hits[key]
	cut := 0
	for cut < len(ts) && now.Sub(ts[cut]) > l.window {
		cut++
	}
	ts = ts[cut:]
	if len(ts) >= l.count {
		l.hits[key] = ts
		return false
	}
	l.hits[key] = append(ts, now)
	return true
}
