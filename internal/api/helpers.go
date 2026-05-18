package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, status int, msg string, err error) {
	attrs := []any{
		"method", r.Method,
		"path", r.URL.Path,
		"status", status,
		"msg", msg,
	}
	if err != nil {
		attrs = append(attrs, "err", err)
	}
	switch {
	case status >= 500:
		s.logger.Error("api_error", attrs...)
	case status >= 400:
		s.logger.Warn("api_error", attrs...)
	default:
		s.logger.Info("api_error", attrs...)
	}
	writeJSON(w, status, map[string]string{"error": msg})
}

const maxJSONBodyBytes = 1 << 20 // 1 MiB

func readJSON(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, maxJSONBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return errors.New("trailing data in JSON body")
		}
		return err
	}
	return nil
}

// readJSONRequest decodes the body into v and writes an error response on failure.
// Returns true only when decoding succeeded.
func (s *Server) readJSONRequest(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := readJSON(r, v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			s.writeError(w, r, http.StatusRequestEntityTooLarge, "request body too large", err)
			return false
		}
		s.writeError(w, r, http.StatusBadRequest, "invalid request body", err)
		return false
	}
	return true
}
