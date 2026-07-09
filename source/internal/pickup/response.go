// Package pickup defines the public, transport-agnostic JSON shape for a
// resolved pickup schedule, shared by the HTTP API and the CLI so both emit
// identical output.
package pickup

import "github.com/sollie/tommedag/internal/avfallsor"

// Response is a cleaned-up, English-keyed projection of the upstream
// avfallsor.no data, with the resolved address and property id surfaced so
// callers can see which property the free-text query mapped to.
type Response struct {
	Query             string `json:"query"`
	MatchedAddress    string `json:"matched_address"`
	PropertyID        string `json:"property_id"`
	Ambiguous         bool   `json:"ambiguous"`
	AlternativesCount int    `json:"alternatives_count"`
	CollectionYear    int    `json:"collection_year"`
	HasNextYear       bool   `json:"has_next_year"`
	CutoffDate        string `json:"cutoff_date,omitempty"`
	PickupDays        []Day  `json:"pickup_days"`
}

// Day is a single collection date with the waste types picked up.
type Day struct {
	Date       string      `json:"date"`
	Label      string      `json:"label"`
	WasteTypes []WasteType `json:"waste_types"`
}

// WasteType is a single waste fraction collected on a pickup day.
type WasteType struct {
	Fraksjon   string   `json:"fraksjon"`
	FraksjonID string   `json:"fraksjon_id"`
	OnRequest  bool     `json:"on_request"`
	Frequency  string   `json:"frequency"`
	Icons      []string `json:"icons"`
}

// NewResponse projects an upstream match and calendar into the public
// response shape.
func NewResponse(query string, match avfallsor.Match, cal avfallsor.PickupCalendar) Response {
	days := make([]Day, 0, len(cal.Collections))
	for _, coll := range cal.Collections {
		types := make([]WasteType, 0, len(coll.Items))
		for _, item := range coll.Items {
			icons := item.WasteIcons
			if icons == nil {
				icons = []string{}
			}
			types = append(types, WasteType{
				Fraksjon:   item.Fraksjon,
				FraksjonID: item.FraksjonID,
				OnRequest:  item.OnRequest,
				Frequency:  item.FrequencyText,
				Icons:      icons,
			})
		}
		days = append(days, Day{
			Date:       coll.DateIndex,
			Label:      coll.Heading,
			WasteTypes: types,
		})
	}

	return Response{
		Query:             query,
		MatchedAddress:    match.Candidate.Label,
		PropertyID:        match.PropertyID,
		Ambiguous:         match.Ambiguous,
		AlternativesCount: match.AlternativesCount,
		CollectionYear:    cal.CollectionYear,
		HasNextYear:       cal.HasNextYear,
		CutoffDate:        cal.CutoffDate,
		PickupDays:        days,
	}
}
