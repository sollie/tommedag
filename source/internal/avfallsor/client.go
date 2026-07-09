package avfallsor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://avfallsor.no"
	defaultUserAgent = "tommedag/1.0 (+https://github.com/sollie/tommedag)"
	defaultTimeout   = 10 * time.Second
	defaultCacheTTL  = time.Hour

	addressSearchPath = "/wp-json/addresses/v1/address"
	pickupCalendarFmt = "/wp-json/pickup-calendar/v1/collections/property-id/%s"

	// maxResponseBytes caps how much of an upstream response we will read,
	// guarding against an unexpectedly huge or hostile body.
	maxResponseBytes = 4 << 20 // 4 MiB
)

// Option configures a Client.
type Option func(*Client)

// WithBaseURL overrides the upstream base URL (primarily for tests).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = strings.TrimSuffix(u, "/") }
}

// WithHTTPClient sets the underlying HTTP client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.httpClient = h }
}

// WithUserAgent sets the User-Agent header sent on every request.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithCacheTTL sets the pickup-calendar cache TTL. A value <= 0 disables
// caching entirely.
func WithCacheTTL(ttl time.Duration) Option {
	return func(c *Client) { c.cacheTTL = ttl }
}

// withClock overrides the cache clock (test-only).
func withClock(now Clock) Option {
	return func(c *Client) { c.clock = now }
}

// Client talks to the avfallsor.no public WordPress REST endpoints that back
// the "Finn hentedag" address search and pickup calendar.
type Client struct {
	baseURL    string
	userAgent  string
	httpClient *http.Client
	cacheTTL   time.Duration
	clock      Clock

	cache *calendarCache
}

// NewClient builds a Client with sensible defaults, applying any options.
func NewClient(opts ...Option) *Client {
	c := &Client{
		baseURL:   defaultBaseURL,
		userAgent: defaultUserAgent,
		httpClient: &http.Client{
			Timeout: defaultTimeout,
		},
		cacheTTL: defaultCacheTTL,
		clock:    time.Now,
	}

	for _, opt := range opts {
		opt(c)
	}

	c.cache = newCalendarCache(c.cacheTTL, c.clock)
	return c
}

// SearchAddress queries the address autocomplete endpoint for term and
// returns the raw candidate list, verbatim from upstream (including any
// street-only entries with empty hrefs). term is normalized before sending,
// since the upstream match is whitespace-sensitive.
func (c *Client) SearchAddress(ctx context.Context, term string) ([]AddressCandidate, error) {
	term = NormalizeQuery(term)
	if term == "" {
		return nil, nil
	}

	q := url.Values{}
	q.Set("lookup_term", term)
	endpoint := c.baseURL + addressSearchPath + "?" + q.Encode()

	var candidates []AddressCandidate
	if err := c.getJSON(ctx, endpoint, &candidates); err != nil {
		return nil, err
	}
	return candidates, nil
}

// ResolveAddress searches for term and auto-picks the single best matching
// property, returning the chosen match. It returns ErrAddressNotFound when
// no resolvable candidate exists. If the initial search yields nothing
// resolvable and term contains a comma (e.g. a pasted "Street 24,
// Kristiansand"), it retries once with the text before the last comma, since
// appending the city breaks the upstream match.
func (c *Client) ResolveAddress(ctx context.Context, term string) (Match, error) {
	candidates, err := c.SearchAddress(ctx, term)
	if err != nil {
		return Match{}, err
	}

	match, err := BestMatch(term, candidates)
	if err == nil {
		return match, nil
	}
	if !errors.Is(err, ErrAddressNotFound) {
		return Match{}, err
	}

	if idx := strings.LastIndex(term, ","); idx > 0 {
		retryTerm := strings.TrimSpace(term[:idx])
		if retryTerm != "" && retryTerm != term {
			retryCandidates, retryErr := c.SearchAddress(ctx, retryTerm)
			if retryErr != nil {
				return Match{}, retryErr
			}
			return BestMatch(retryTerm, retryCandidates)
		}
	}

	return Match{}, err
}

// PickupCalendar fetches the pickup schedule for a property UUID. The
// property ID is validated client-side first (upstream returns a bare 404
// for a malformed UUID). Results are cached per property ID for the
// configured TTL.
func (c *Client) PickupCalendar(ctx context.Context, propertyID string) (PickupCalendar, error) {
	if err := ValidatePropertyID(propertyID); err != nil {
		return PickupCalendar{}, err
	}

	if cal, ok := c.cache.get(propertyID); ok {
		return cal, nil
	}

	endpoint := c.baseURL + fmt.Sprintf(pickupCalendarFmt, propertyID)

	var cal PickupCalendar
	if err := c.getJSON(ctx, endpoint, &cal); err != nil {
		return PickupCalendar{}, err
	}

	c.cache.set(propertyID, cal)
	return cal, nil
}

// getJSON performs a GET against endpoint and decodes a JSON body into dst.
// Any transport error, non-2xx status, or non-JSON content type is wrapped
// as ErrUpstreamUnavailable so callers can react uniformly.
func (c *Client) getJSON(ctx context.Context, endpoint string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%w: building request: %v", ErrUpstreamUnavailable, err)
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUpstreamUnavailable, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%w: unexpected status %d", ErrUpstreamUnavailable, resp.StatusCode)
	}

	if ct := resp.Header.Get("Content-Type"); ct != "" {
		mediaType, _, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "application/json" {
			return fmt.Errorf("%w: unexpected content type %q", ErrUpstreamUnavailable, ct)
		}
	}

	body := io.LimitReader(resp.Body, maxResponseBytes)
	if err := json.NewDecoder(body).Decode(dst); err != nil {
		return fmt.Errorf("%w: decoding response: %v", ErrUpstreamUnavailable, err)
	}

	return nil
}
