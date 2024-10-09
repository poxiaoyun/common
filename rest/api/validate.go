package api

import (
	"net/http"
)

// ValidateBody is a function that validates the body of a request.
// It should return an error if the body is invalid.
// Users can override this function to implement their own body validation.
var ValidateBody = func(r *http.Request, data any) error {
	return nil
}
