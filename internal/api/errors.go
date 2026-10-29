package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"gokku/internal/auth"
	"gokku/internal/store"
)

// apiError is an error with its HTTP status and API error code (SPEC.md, section 8).
type apiError struct {
	Status  int
	Code    ErrorCode
	Message string
	Fields  map[string]string
}

func (e *apiError) Error() string { return string(e.Code) + ": " + e.Message }

func newError(status int, code ErrorCode, msg string) *apiError {
	return &apiError{Status: status, Code: code, Message: msg}
}

func validationError(fields map[string]string) *apiError {
	return &apiError{Status: http.StatusBadRequest, Code: ErrorCodeValidationFailed, Message: "Some fields are invalid.", Fields: fields}
}

// toAPIError maps domain errors to API errors. Anything unknown is an internal error, and
// its message is not shown to the client (SEC-5).
func toAPIError(err error) *apiError {
	var ae *apiError
	if errors.As(err, &ae) {
		return ae
	}
	var ve *auth.ValidationError
	if errors.As(err, &ve) {
		return validationError(ve.Fields)
	}
	switch {
	case errors.Is(err, auth.ErrInvalidCredentials):
		return newError(http.StatusUnauthorized, ErrorCodeInvalidCredentials, "Wrong username or password.")
	case errors.Is(err, auth.ErrRateLimited):
		return newError(http.StatusTooManyRequests, ErrorCodeRateLimited, "Too many failed sign-ins. Try again in 15 minutes.")
	case errors.Is(err, auth.ErrSetupComplete):
		return newError(http.StatusConflict, ErrorCodeSetupComplete, "Setup is already complete.")
	case errors.Is(err, auth.ErrInviteInvalid):
		return newError(http.StatusNotFound, ErrorCodeInviteInvalid, "This invite link is invalid or has expired. Ask an admin for a new one.")
	case errors.Is(err, auth.ErrNotInvited):
		return newError(http.StatusConflict, ErrorCodeNotInvited, "This user has already accepted their invite.")
	case errors.Is(err, auth.ErrWrongPassword):
		ae := newError(http.StatusBadRequest, ErrorCodeWrongPassword, "The current password is wrong.")
		ae.Fields = map[string]string{"current_password": "The current password is wrong."}
		return ae
	case errors.Is(err, auth.ErrUnauthenticated):
		return errUnauthorized
	case errors.Is(err, store.ErrLastAdmin):
		return newError(http.StatusConflict, ErrorCodeLastAdmin, "Gokku needs at least one active admin.")
	case errors.Is(err, store.ErrNotFound):
		return newError(http.StatusNotFound, ErrorCodeNotFound, "Not found.")
	default:
		return newError(http.StatusInternalServerError, ErrorCodeInternal, "Something went wrong. The server log has details.")
	}
}

var (
	errUnauthorized = newError(http.StatusUnauthorized, ErrorCodeUnauthorized, "Sign in to continue.")
	errForbidden    = newError(http.StatusForbidden, ErrorCodeForbidden, "Your role does not allow this.")
)

func writeError(w http.ResponseWriter, ae *apiError) {
	body := Error{}
	body.Error.Code = ae.Code
	body.Error.Message = ae.Message
	if len(ae.Fields) > 0 {
		f := ae.Fields
		body.Error.Fields = &f
	}
	writeJSON(w, ae.Status, body)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the status is sent; a write error is a closed connection
}

// maxBody matches client_max_body_size in deploy/nginx.conf.example.
const maxBody = 1 << 20

// decode reads a JSON body strictly: unknown fields and trailing data are errors.
func decode(r *http.Request, dst any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return newError(http.StatusBadRequest, ErrorCodeValidationFailed, "The request body is not valid JSON for this endpoint.")
	}
	if dec.More() {
		return newError(http.StatusBadRequest, ErrorCodeValidationFailed, "The request body has trailing data.")
	}
	return nil
}
