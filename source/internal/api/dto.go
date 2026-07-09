package api

// ErrorResponse is the JSON body returned for non-2xx responses.
type ErrorResponse struct {
	Error string `json:"error"`
}
