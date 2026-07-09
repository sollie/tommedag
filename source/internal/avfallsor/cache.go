package avfallsor

import (
	"sync"
	"time"
)

// Clock returns the current time. It exists so tests can control cache
// expiry deterministically.
type Clock func() time.Time

type cacheEntry struct {
	calendar  PickupCalendar
	expiresAt time.Time
}

// calendarCache is a small concurrency-safe TTL cache keyed by property
// UUID. It exists to spare avfallsor.no repeated identical lookups; the
// upstream pickup-calendar responses advertise Cache-Control max-age=3600,
// so a default TTL of one hour matches their own intent.
type calendarCache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     time.Duration
	now     Clock
}

func newCalendarCache(ttl time.Duration, now Clock) *calendarCache {
	if now == nil {
		now = time.Now
	}
	return &calendarCache{
		entries: make(map[string]cacheEntry),
		ttl:     ttl,
		now:     now,
	}
}

func (c *calendarCache) get(id string) (PickupCalendar, bool) {
	if c.ttl <= 0 {
		return PickupCalendar{}, false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[id]
	if !ok {
		return PickupCalendar{}, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, id)
		return PickupCalendar{}, false
	}
	return entry.calendar, true
}

func (c *calendarCache) set(id string, calendar PickupCalendar) {
	if c.ttl <= 0 {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[id] = cacheEntry{
		calendar:  calendar,
		expiresAt: c.now().Add(c.ttl),
	}
}
