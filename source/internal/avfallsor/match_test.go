package avfallsor

import (
	"errors"
	"testing"
)

func TestNormalizeQuery(t *testing.T) {
	cases := map[string]string{
		"  Dronningens   gate  24 ": "Dronningens gate 24",
		"Dronningens gate 24":       "Dronningens gate 24",
		"\tDronningens\ngate\t24":   "Dronningens gate 24",
		"":                          "",
		"   ":                       "",
	}
	for in, want := range cases {
		if got := NormalizeQuery(in); got != want {
			t.Errorf("NormalizeQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidatePropertyID(t *testing.T) {
	valid := "223cdde9-6a18-43ca-885f-78babd62968c"
	if err := ValidatePropertyID(valid); err != nil {
		t.Errorf("ValidatePropertyID(%q) unexpected error: %v", valid, err)
	}

	for _, bad := range []string{"", "not-a-uuid", "223cdde9-6a18-43ca-885f", "223cdde9_6a18_43ca_885f_78babd62968c"} {
		if err := ValidatePropertyID(bad); !errors.Is(err, ErrInvalidPropertyID) {
			t.Errorf("ValidatePropertyID(%q) = %v, want ErrInvalidPropertyID", bad, err)
		}
	}
}

func TestAddressCandidatePropertyID(t *testing.T) {
	c := AddressCandidate{
		Href: "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/1332c0c6-1c40-4ce8-86d7-a62717122e65/",
	}
	id, err := c.PropertyID()
	if err != nil {
		t.Fatalf("PropertyID() error: %v", err)
	}
	if want := "1332c0c6-1c40-4ce8-86d7-a62717122e65"; id != want {
		t.Errorf("PropertyID() = %q, want %q", id, want)
	}

	if _, err := (AddressCandidate{Href: ""}).PropertyID(); !errors.Is(err, ErrInvalidPropertyID) {
		t.Errorf("empty href: got %v, want ErrInvalidPropertyID", err)
	}
	if _, err := (AddressCandidate{Href: "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/"}).PropertyID(); !errors.Is(err, ErrInvalidPropertyID) {
		t.Errorf("href with no uuid: got %v, want ErrInvalidPropertyID", err)
	}
}

func TestBestMatch(t *testing.T) {
	uuidBase := "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/1332c0c6-1c40-4ce8-86d7-a62717122e65/"
	uuidA := "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/32251184-9d28-4f51-8e6f-e6640d5d6c44/"
	uuidB := "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/aa290198-6e8b-4f1c-adc0-6c40b8cd7287/"

	t.Run("exact match wins over longer prefixes", func(t *testing.T) {
		results := []AddressCandidate{
			{Value: "Dronningens Gate 24 A", Label: "24 A", Href: uuidA},
			{Value: "Dronningens Gate 24", Label: "24", Href: uuidBase},
			{Value: "Dronningens Gate 24 B", Label: "24 B", Href: uuidB},
		}
		m, err := BestMatch("Dronningens gate 24", results)
		if err != nil {
			t.Fatalf("BestMatch error: %v", err)
		}
		if m.PropertyID != "1332c0c6-1c40-4ce8-86d7-a62717122e65" {
			t.Errorf("got property id %q, want base", m.PropertyID)
		}
		if !m.Ambiguous {
			t.Error("expected Ambiguous=true with 3 resolvable candidates")
		}
		if m.AlternativesCount != 2 {
			t.Errorf("AlternativesCount = %d, want 2", m.AlternativesCount)
		}
	})

	t.Run("shortest prefix wins when no exact match", func(t *testing.T) {
		results := []AddressCandidate{
			{Value: "Dronningens Gate 24 A", Label: "24 A", Href: uuidA},
			{Value: "Dronningens Gate 24 B", Label: "24 B", Href: uuidB},
		}
		// query is a prefix of both; the shorter value wins (they're equal
		// length here, so first-seen shortest wins deterministically).
		m, err := BestMatch("Dronningens gate 24 A", results)
		if err != nil {
			t.Fatalf("BestMatch error: %v", err)
		}
		if m.Candidate.Value != "Dronningens Gate 24 A" {
			t.Errorf("got %q, want exact 24 A", m.Candidate.Value)
		}
	})

	t.Run("street-only entries are filtered out", func(t *testing.T) {
		results := []AddressCandidate{
			{Value: "Øvre Brattbakken ", Label: "Øvre Brattbakken ", Href: ""},
			{Value: "Øvre ringvei ", Label: "Øvre ringvei ", Href: ""},
		}
		if _, err := BestMatch("Øvre", results); !errors.Is(err, ErrAddressNotFound) {
			t.Errorf("got %v, want ErrAddressNotFound", err)
		}
	})

	t.Run("empty results", func(t *testing.T) {
		if _, err := BestMatch("anything", nil); !errors.Is(err, ErrAddressNotFound) {
			t.Errorf("got %v, want ErrAddressNotFound", err)
		}
	})

	t.Run("single resolvable candidate is not ambiguous", func(t *testing.T) {
		results := []AddressCandidate{
			{Value: "Dronningens Gate 33", Label: "33", Href: uuidBase},
		}
		m, err := BestMatch("Dronningens gate 33", results)
		if err != nil {
			t.Fatalf("BestMatch error: %v", err)
		}
		if m.Ambiguous || m.AlternativesCount != 0 {
			t.Errorf("Ambiguous=%v AlternativesCount=%d, want false/0", m.Ambiguous, m.AlternativesCount)
		}
	})

	t.Run("no prefix match falls back to first resolvable", func(t *testing.T) {
		results := []AddressCandidate{
			{Value: "Something Else 1", Label: "first", Href: uuidBase},
			{Value: "Another Road 2", Label: "second", Href: uuidA},
		}
		m, err := BestMatch("Totally Different", results)
		if err != nil {
			t.Fatalf("BestMatch error: %v", err)
		}
		if m.Candidate.Label != "first" {
			t.Errorf("got %q, want first resolvable", m.Candidate.Label)
		}
	})
}
