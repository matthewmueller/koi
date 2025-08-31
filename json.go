package koi

import (
	"encoding/json"
	"net/http"

	"github.com/matthewmueller/koi/internal/request"
)

// JSON is an http.Handler for responding to JSON requests.
func JSON[In, Out any](fn func(w http.ResponseWriter, r *http.Request, in In) (out Out, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in In

		// Decode the request
		if err := request.Unmarshal(r, &in); err != nil {
			status := http.StatusBadRequest
			if se, ok := err.(statuser); ok {
				status = se.Status()
			}
			// TODO return the invalid fields in the error response
			jsonError(w, http.StatusText(status), status)
			return
		}

		// Validate the input
		if err := validate(r.Context(), in); err != nil {
			status := http.StatusBadRequest
			if se, ok := err.(statuser); ok {
				status = se.Status()
			}
			// TODO: return the invalid fields in the error response
			jsonError(w, http.StatusText(status), status)
			return
		}

		// Call the handler
		out, err := fn(w, r, in)
		if err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(statuser); ok {
				status = se.Status()
			}
			jsonError(w, http.StatusText(status), status)
			return
		}

		// Encode the response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(statuser); ok {
				status = se.Status()
			}
			jsonError(w, http.StatusText(status), status)
			return
		}
	})
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorJson{Message: msg})
}

type errorJson struct {
	Message string            `json:"error"`
	Fields  map[string]string `json:"fields,omitzero"`
}

func (e *errorJson) Error() string {
	return e.Message
}
