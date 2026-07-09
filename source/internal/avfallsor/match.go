package avfallsor

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// extractPropertyID pulls the property UUID out of a finn-hentedag href,
// e.g. "https://avfallsor.no/hjemme-hos-deg/finn-hentedag/<uuid>/".
func extractPropertyID(href string) (string, error) {
	if href == "" {
		return "", fmt.Errorf("%w: empty href", ErrInvalidPropertyID)
	}

	u, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidPropertyID, err)
	}

	id := path.Base(strings.TrimSuffix(u.Path, "/"))
	if !uuidPattern.MatchString(id) {
		return "", fmt.Errorf("%w: %q is not a valid property id", ErrInvalidPropertyID, id)
	}

	return id, nil
}

// ValidatePropertyID reports whether id has the UUID shape the
// pickup-calendar endpoint expects, without making any network call.
func ValidatePropertyID(id string) error {
	if !uuidPattern.MatchString(id) {
		return fmt.Errorf("%w: %q", ErrInvalidPropertyID, id)
	}
	return nil
}

// NormalizeQuery trims surrounding whitespace and collapses any run of
// internal whitespace to a single space, preserving case. The upstream
// address search matches on the raw lookup_term with no normalization of
// its own (a double space breaks matching entirely), so queries must be
// normalized before being sent.
func NormalizeQuery(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalizeAddress is NormalizeQuery lowercased, for case-insensitive
// comparison, so "Dronningens  gate   24" and "dronningens gate 24" compare
// equal to "Dronningens Gate 24".
func normalizeAddress(s string) string {
	return strings.ToLower(NormalizeQuery(s))
}

// Match is the outcome of resolving a free-text address to a single best
// candidate out of possibly many returned by the address search.
type Match struct {
	Candidate         AddressCandidate
	PropertyID        string
	Ambiguous         bool
	AlternativesCount int
}

// BestMatch picks the single most likely candidate for query out of
// results, auto-resolving ambiguity rather than asking the caller to choose:
//
//  1. Candidates with no resolvable href (street-only entries) are dropped.
//  2. If a candidate's Value matches query exactly (case/whitespace
//     insensitive), it wins.
//  3. Otherwise, among candidates whose Value starts with the normalized
//     query, the shortest one wins (e.g. "Dronningens Gate 24" beats
//     "24 A"/"24 B"/... sub-units), since it's the closest to a "base"
//     address.
//  4. Otherwise, the first candidate as ordered by upstream is used.
//
// Ambiguous reports whether more than one resolvable candidate existed at
// all, regardless of which rule picked the winner.
func BestMatch(query string, results []AddressCandidate) (Match, error) {
	resolvable := make([]AddressCandidate, 0, len(results))
	for _, r := range results {
		if r.Resolvable() {
			resolvable = append(resolvable, r)
		}
	}

	if len(resolvable) == 0 {
		return Match{}, ErrAddressNotFound
	}

	normQuery := normalizeAddress(query)

	winner := resolvable[0]
	found := false

	for _, r := range resolvable {
		if normalizeAddress(r.Value) == normQuery {
			winner = r
			found = true
			break
		}
	}

	if !found {
		bestLen := -1
		for _, r := range resolvable {
			normValue := normalizeAddress(r.Value)
			if !strings.HasPrefix(normValue, normQuery) {
				continue
			}
			if bestLen == -1 || len(normValue) < bestLen {
				winner = r
				bestLen = len(normValue)
			}
		}
	}

	id, err := winner.PropertyID()
	if err != nil {
		return Match{}, err
	}

	return Match{
		Candidate:         winner,
		PropertyID:        id,
		Ambiguous:         len(resolvable) > 1,
		AlternativesCount: len(resolvable) - 1,
	}, nil
}
