package koi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/matthewmueller/koi/internal/request"
)

// JSON is an http.Handler for responding to JSON requests.
func JSON[In, Out any](fn func(w http.ResponseWriter, r *http.Request, in In) (out Out, err error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in In

		// Decode the request
		if err := request.Unmarshal(r, &in); err != nil {
			status := http.StatusBadRequest
			if se, ok := err.(Statuser); ok {
				status = se.Status()
			}
			// TODO return the invalid fields in the error response
			jsonError(w, err, status)
			return
		}

		// Validate the input.
		// Works for both `func (T) Validate()` and `func (*T) Validate()`
		if v, ok := any(in).(Validator); ok {
			if err := v.Validate(); err != nil {
				status := http.StatusBadRequest
				if se, ok := err.(Statuser); ok {
					status = se.Status()
				}
				jsonError(w, err, status)
				return
			}
		} else if v, ok := any(&in).(Validator); ok {
			if err := v.Validate(); err != nil {
				status := http.StatusBadRequest
				if se, ok := err.(Statuser); ok {
					status = se.Status()
				}
				jsonError(w, err, status)
				return
			}
		}

		// Call the handler
		out, err := fn(w, r, in)
		if err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(Statuser); ok {
				status = se.Status()
			}
			jsonError(w, err, status)
			return
		}

		// Encode the response
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(out); err != nil {
			status := http.StatusInternalServerError
			if se, ok := err.(Statuser); ok {
				status = se.Status()
			}
			jsonError(w, err, status)
			return
		}
	})
}

func jsonError(w http.ResponseWriter, err error, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(encodableError(err))
}

func encodableError(err error) json.Marshaler {
	if err == nil {
		return nil
	}
	if em, ok := err.(json.Marshaler); ok {
		return em
	}
	return json.RawMessage(`{"error":` + strconv.Quote(err.Error()) + `}`)
}
