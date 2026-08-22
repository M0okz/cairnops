package pushrelay

import (
	"sync"
	"time"
)

type rateWindow struct {
	started time.Time
	count   int
}

type RateLimiter struct {
	limit  int
	window time.Duration
	now    func() time.Time
	mu     sync.Mutex
	items  map[string]rateWindow
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, window: window, now: time.Now, items: make(map[string]rateWindow)}
}

func (limiter *RateLimiter) Allow(key string) bool {
	if limiter == nil || limiter.limit <= 0 || limiter.window <= 0 {
		return true
	}
	now := limiter.now().UTC()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	item, exists := limiter.items[key]
	if !exists || now.Sub(item.started) >= limiter.window {
		limiter.items[key] = rateWindow{started: now, count: 1}
		limiter.prune(now)
		return true
	}
	if item.count >= limiter.limit {
		return false
	}
	item.count++
	limiter.items[key] = item
	return true
}

func (limiter *RateLimiter) prune(now time.Time) {
	if len(limiter.items) < 1024 {
		return
	}
	for key, item := range limiter.items {
		if now.Sub(item.started) >= limiter.window {
			delete(limiter.items, key)
		}
	}
}
