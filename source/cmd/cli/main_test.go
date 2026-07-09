package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sollie/tommedag/internal/pickup"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}
	return data
}

// fakeUpstream serves the address search and pickup calendar fixtures,
// standing in for avfallsor.no.
func fakeUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/wp-json/addresses/v1/address"):
			_, _ = w.Write(fixture(t, "address_search_unique_match.json"))
		case strings.HasPrefix(r.URL.Path, "/wp-json/pickup-calendar/v1/collections/property-id/"):
			_, _ = w.Write(fixture(t, "pickup_calendar_223cdde9.json"))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRunSuccess(t *testing.T) {
	srv := fakeUpstream(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-base-url", srv.URL, "Dronningens", "gate", "33"}, &stdout, &stderr)

	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr=%s", code, exitOK, stderr.String())
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr not empty: %s", stderr.String())
	}

	var resp pickup.Response
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", err, stdout.String())
	}
	if resp.PropertyID != "bdd61d75-e427-46b2-bdfc-1962dacaf0ba" {
		t.Errorf("property id = %q", resp.PropertyID)
	}
	if resp.Query != "Dronningens gate 33" {
		t.Errorf("query = %q, want joined args", resp.Query)
	}
	if len(resp.PickupDays) == 0 {
		t.Error("expected pickup days")
	}
}

func TestRunIndentedByDefault(t *testing.T) {
	srv := fakeUpstream(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-base-url", srv.URL, "Dronningens gate 33"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "\n  ") {
		t.Error("expected indented JSON by default")
	}
}

func TestRunCompact(t *testing.T) {
	srv := fakeUpstream(t)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	if code := run([]string{"-base-url", srv.URL, "-compact", "Dronningens gate 33"}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, stderr.String())
	}
	out := strings.TrimRight(stdout.String(), "\n")
	if strings.Contains(out, "\n") {
		t.Errorf("expected single-line JSON with -compact, got:\n%s", out)
	}
}

func TestRunMissingAddress(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(nil, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on usage error, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "address argument is required") {
		t.Errorf("stderr missing usage hint: %s", stderr.String())
	}
}

func TestRunUnknownFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"-nope"}, &stdout, &stderr)
	if code != exitUsage {
		t.Errorf("exit = %d, want %d", code, exitUsage)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty, got: %s", stdout.String())
	}
}

func TestRunAddressNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(fixture(t, "address_search_street_only.json"))
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-base-url", srv.URL, "Øvre"}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout should be empty on error, got: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "no matching address found") {
		t.Errorf("stderr = %q, want not-found message", stderr.String())
	}
}

func TestRunUpstreamDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := run([]string{"-base-url", srv.URL, "Dronningens gate 33"}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "unavailable") {
		t.Errorf("stderr = %q, want unavailable message", stderr.String())
	}
}
