package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sollie/tommedag/internal/avfallsor"
	"github.com/sollie/tommedag/internal/pickup"
)

type fakeResolver struct {
	resolveFn  func(ctx context.Context, term string) (avfallsor.Match, error)
	calendarFn func(ctx context.Context, id string) (avfallsor.PickupCalendar, error)
}

func (f fakeResolver) ResolveAddress(ctx context.Context, term string) (avfallsor.Match, error) {
	return f.resolveFn(ctx, term)
}

func (f fakeResolver) PickupCalendar(ctx context.Context, id string) (avfallsor.PickupCalendar, error) {
	return f.calendarFn(ctx, id)
}

func doRequest(t *testing.T, r fakeResolver, target string) *httptest.ResponseRecorder {
	t.Helper()
	router := NewRouter(r, nil)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestGetPickupDaysOK(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(_ context.Context, term string) (avfallsor.Match, error) {
			return avfallsor.Match{
				Candidate:         avfallsor.AddressCandidate{Label: "Dronningens Gate 24, Kristiansand"},
				PropertyID:        "1332c0c6-1c40-4ce8-86d7-a62717122e65",
				Ambiguous:         true,
				AlternativesCount: 11,
			}, nil
		},
		calendarFn: func(_ context.Context, _ string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{
				CollectionYear: 2026,
				HasNextYear:    false,
				CutoffDate:     "2026-08-31",
				Collections: []avfallsor.PickupCollection{
					{
						DateIndex: "2026-07-15",
						Heading:   "Onsdag 15. juli",
						Items: []avfallsor.PickupItem{
							{Fraksjon: "Restavfall", FraksjonID: "9011", FrequencyText: "hver uke", WasteIcons: []string{"residual"}},
						},
					},
				},
			}, nil
		},
	}

	rec := doRequest(t, r, "/api/v1/pickup-days?address=Dronningens+gate+24")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var resp pickup.Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.PropertyID != "1332c0c6-1c40-4ce8-86d7-a62717122e65" {
		t.Errorf("property id = %q", resp.PropertyID)
	}
	if resp.MatchedAddress != "Dronningens Gate 24, Kristiansand" {
		t.Errorf("matched_address = %q", resp.MatchedAddress)
	}
	if !resp.Ambiguous || resp.AlternativesCount != 11 {
		t.Errorf("ambiguous=%v alternatives=%d", resp.Ambiguous, resp.AlternativesCount)
	}
	if len(resp.PickupDays) != 1 || len(resp.PickupDays[0].WasteTypes) != 1 {
		t.Fatalf("unexpected pickup days shape: %+v", resp.PickupDays)
	}
	if resp.PickupDays[0].WasteTypes[0].Fraksjon != "Restavfall" {
		t.Errorf("waste type = %q", resp.PickupDays[0].WasteTypes[0].Fraksjon)
	}
}

func TestGetPickupDaysMissingAddress(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(context.Context, string) (avfallsor.Match, error) {
			t.Fatal("should not resolve")
			return avfallsor.Match{}, nil
		},
		calendarFn: func(context.Context, string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{}, nil
		},
	}
	rec := doRequest(t, r, "/api/v1/pickup-days")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestGetPickupDaysNotFound(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(context.Context, string) (avfallsor.Match, error) {
			return avfallsor.Match{}, avfallsor.ErrAddressNotFound
		},
		calendarFn: func(context.Context, string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{}, nil
		},
	}
	rec := doRequest(t, r, "/api/v1/pickup-days?address=zzz")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestGetPickupDaysUpstreamDown(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(context.Context, string) (avfallsor.Match, error) {
			return avfallsor.Match{}, avfallsor.ErrUpstreamUnavailable
		},
		calendarFn: func(context.Context, string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{}, nil
		},
	}
	rec := doRequest(t, r, "/api/v1/pickup-days?address=test")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestGetPickupDaysCalendarUpstreamDown(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(context.Context, string) (avfallsor.Match, error) {
			return avfallsor.Match{PropertyID: "223cdde9-6a18-43ca-885f-78babd62968c"}, nil
		},
		calendarFn: func(context.Context, string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{}, avfallsor.ErrUpstreamUnavailable
		},
	}
	rec := doRequest(t, r, "/api/v1/pickup-days?address=test")
	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	r := fakeResolver{
		resolveFn: func(context.Context, string) (avfallsor.Match, error) { return avfallsor.Match{}, nil },
		calendarFn: func(context.Context, string) (avfallsor.PickupCalendar, error) {
			return avfallsor.PickupCalendar{}, nil
		},
	}
	rec := doRequest(t, r, "/healthz")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}
