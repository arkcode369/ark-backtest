package bot

import (
	"sync"
	"time"
)

// UserRateLimiter provides per-user rate limiting for expensive operations.
// Each operation category has its own cooldown period.
type UserRateLimiter struct {
	mu       sync.Mutex
	limits   map[int64]map[string]time.Time // chatID -> category -> lastUsed
	cooldown map[string]time.Duration       // category -> cooldown
}

// NewUserRateLimiter creates a rate limiter with default cooldowns.
func NewUserRateLimiter() *UserRateLimiter {
	return &UserRateLimiter{
		limits: make(map[int64]map[string]time.Time),
		cooldown: map[string]time.Duration{
			"price":    2 * time.Second,
			"backtest": 5 * time.Second,
			"ai":       5 * time.Second,
		},
	}
}

// Allow returns true if the user is allowed to perform the given operation category.
// If allowed, it records the current time as last usage.
func (r *UserRateLimiter) Allow(chatID int64, category string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	cd, ok := r.cooldown[category]
	if !ok {
		return true // unknown category, no limit
	}

	userLimits, ok := r.limits[chatID]
	if !ok {
		userLimits = make(map[string]time.Time)
		r.limits[chatID] = userLimits
	}

	last, ok := userLimits[category]
	if ok && time.Since(last) < cd {
		return false
	}

	userLimits[category] = time.Now()
	return true
}

// Cleanup removes entries for users who haven't been active recently.
// Call periodically to prevent memory leaks.
func (r *UserRateLimiter) Cleanup(maxAge time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for chatID, cats := range r.limits {
		allExpired := true
		for _, t := range cats {
			if time.Since(t) < maxAge {
				allExpired = false
				break
			}
		}
		if allExpired {
			delete(r.limits, chatID)
		}
	}
}
