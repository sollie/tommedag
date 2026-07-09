package avfallsor

// AddressCandidate is a single result from the address search endpoint.
// Value and Label mirror the upstream fields verbatim; Href, when non-empty,
// contains the full finn-hentedag URL with the property UUID as its last
// path segment. Candidates with an empty Href are street-level entries with
// no specific property attached and cannot be resolved to a pickup
// calendar.
type AddressCandidate struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Href  string `json:"href"`
}

// PropertyID reports the property UUID embedded in Href, if any.
func (c AddressCandidate) PropertyID() (string, error) {
	return extractPropertyID(c.Href)
}

// Resolvable reports whether this candidate carries a usable property href.
func (c AddressCandidate) Resolvable() bool {
	return c.Href != ""
}

// PickupCalendar is the pickup schedule for a single property, as returned
// by the pickup-calendar endpoint.
type PickupCalendar struct {
	Collections    []PickupCollection `json:"collections"`
	CurrentYear    string             `json:"currentYear"`
	CollectionYear int                `json:"collectionYear"`
	HasNextYear    bool               `json:"hasNextYear"`
	CutoffDate     string             `json:"cutoffDate"`
}

// PickupCollection groups the waste fractions collected on a single date.
type PickupCollection struct {
	DateIndex string       `json:"dateIndex"`
	Heading   string       `json:"heading"`
	Items     []PickupItem `json:"items"`
}

// PickupItem is a single waste fraction picked up as part of a collection.
type PickupItem struct {
	FraksjonID       string   `json:"fraksjonId"`
	Fraksjon         string   `json:"fraksjon"`
	Dato             string   `json:"dato"`
	DatoFormatted    string   `json:"datoFormatted"`
	WasteIcons       []string `json:"wasteIcons"`
	OnRequest        bool     `json:"onRequest"`
	FrequencyText    string   `json:"frequencyText"`
	ShowCalendarLink bool     `json:"showCalendarLink"`
}
