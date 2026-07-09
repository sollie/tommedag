package avfallsor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

func TestSearchAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != addressSearchPath {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.URL.Query().Get("lookup_term"); got != "Dronningens gate 24" {
			t.Errorf("lookup_term = %q, want normalized", got)
		}
		w.Header().Set("Content-Type", "application/json; charset=UTF-8")
		_, _ = w.Write(fixture(t, "address_search_dronningens_gate.json"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	// Deliberately messy whitespace to exercise normalization.
	got, err := c.SearchAddress(context.Background(), "  Dronningens   gate  24 ")
	if err != nil {
		t.Fatalf("SearchAddress error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected candidates")
	}
	id, err := got[4].PropertyID()
	if err != nil {
		t.Fatalf("PropertyID: %v", err)
	}
	if id != "1332c0c6-1c40-4ce8-86d7-a62717122e65" {
		t.Errorf("candidate[4] id = %q", id)
	}
}

func TestSearchAddressEmptyTerm(t *testing.T) {
	c := NewClient(WithBaseURL("http://never.invalid"))
	got, err := c.SearchAddress(context.Background(), "   ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for empty term, got %v", got)
	}
}

func TestResolveAddress(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "address_search_unique_match.json"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	m, err := c.ResolveAddress(context.Background(), "Dronningens gate 33")
	if err != nil {
		t.Fatalf("ResolveAddress error: %v", err)
	}
	if m.PropertyID != "bdd61d75-e427-46b2-bdfc-1962dacaf0ba" {
		t.Errorf("property id = %q", m.PropertyID)
	}
	if m.Ambiguous {
		t.Error("single match should not be ambiguous")
	}
}

func TestResolveAddressCommaFallback(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		term := r.URL.Query().Get("lookup_term")
		w.Header().Set("Content-Type", "application/json")
		// First call includes the city and matches nothing; the retry
		// (without the ", Kristiansand" suffix) succeeds.
		if term == "Dronningens gate 33, Kristiansand" {
			_, _ = w.Write(fixture(t, "address_search_empty.json"))
			return
		}
		if term == "Dronningens gate 33" {
			_, _ = w.Write(fixture(t, "address_search_unique_match.json"))
			return
		}
		t.Errorf("call %d: unexpected lookup_term %q", n, term)
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	m, err := c.ResolveAddress(context.Background(), "Dronningens gate 33, Kristiansand")
	if err != nil {
		t.Fatalf("ResolveAddress error: %v", err)
	}
	if m.PropertyID != "bdd61d75-e427-46b2-bdfc-1962dacaf0ba" {
		t.Errorf("property id = %q", m.PropertyID)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 upstream calls (initial + retry), got %d", got)
	}
}

func TestResolveAddressNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "address_search_street_only.json"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	if _, err := c.ResolveAddress(context.Background(), "Øvre"); !errors.Is(err, ErrAddressNotFound) {
		t.Errorf("got %v, want ErrAddressNotFound", err)
	}
}

func TestPickupCalendar(t *testing.T) {
	// Count upstream hits to prove the second lookup is served from cache.
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		wantPath := "/wp-json/pickup-calendar/v1/collections/property-id/223cdde9-6a18-43ca-885f-78babd62968c"
		if r.URL.Path != wantPath {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "pickup_calendar_223cdde9.json"))
	}))
	defer srv.Close()
	c := NewClient(WithBaseURL(srv.URL))

	id := "223cdde9-6a18-43ca-885f-78babd62968c"
	cal, err := c.PickupCalendar(context.Background(), id)
	if err != nil {
		t.Fatalf("PickupCalendar error: %v", err)
	}
	if cal.CollectionYear != 2026 {
		t.Errorf("CollectionYear = %d", cal.CollectionYear)
	}
	if len(cal.Collections) != 7 {
		t.Errorf("len(Collections) = %d, want 7", len(cal.Collections))
	}
	if cal.CutoffDate != "2026-08-31" {
		t.Errorf("CutoffDate = %q", cal.CutoffDate)
	}

	// Second call should be served from cache, not the server.
	if _, err := c.PickupCalendar(context.Background(), id); err != nil {
		t.Fatalf("cached PickupCalendar error: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Errorf("expected 1 upstream call with caching, got %d", got)
	}
}

func TestPickupCalendarCacheExpiry(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "pickup_calendar_223cdde9.json"))
	}))
	defer srv.Close()

	now := time.Now()
	clock := func() time.Time { return now }
	c := NewClient(WithBaseURL(srv.URL), WithCacheTTL(time.Hour), withClock(clock))

	id := "223cdde9-6a18-43ca-885f-78babd62968c"
	if _, err := c.PickupCalendar(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	// Advance the clock past the TTL; the cached entry must expire.
	now = now.Add(2 * time.Hour)
	if _, err := c.PickupCalendar(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Errorf("expected 2 calls after cache expiry, got %d", got)
	}
}

func TestPickupCalendarNullCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "pickup_calendar_null.json"))
	}))
	defer srv.Close()

	c := NewClient(WithBaseURL(srv.URL))
	cal, err := c.PickupCalendar(context.Background(), "223cdde9-6a18-43ca-885f-78babd62968c")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cal.Collections != nil {
		t.Errorf("expected nil collections, got %v", cal.Collections)
	}
}

func TestPickupCalendarInvalidID(t *testing.T) {
	c := NewClient(WithBaseURL("http://never.invalid"))
	if _, err := c.PickupCalendar(context.Background(), "not-a-uuid"); !errors.Is(err, ErrInvalidPropertyID) {
		t.Errorf("got %v, want ErrInvalidPropertyID", err)
	}
}

func TestGetJSONUpstreamErrors(t *testing.T) {
	t.Run("non-2xx status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()
		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.SearchAddress(context.Background(), "test")
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Errorf("got %v, want ErrUpstreamUnavailable", err)
		}
	})

	t.Run("non-json content type", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html>challenge</html>"))
		}))
		defer srv.Close()
		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.SearchAddress(context.Background(), "test")
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Errorf("got %v, want ErrUpstreamUnavailable", err)
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("{not json"))
		}))
		defer srv.Close()
		c := NewClient(WithBaseURL(srv.URL))
		_, err := c.SearchAddress(context.Background(), "test")
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Errorf("got %v, want ErrUpstreamUnavailable", err)
		}
	})
}
