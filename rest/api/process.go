package api

import (
	"context"
	"net/http"
)

type ResponseStatusOnly int32

var (
	Empty     = ResponseStatusOnly(http.StatusOK)
	Accepted  = ResponseStatusOnly(http.StatusAccepted)
	Created   = ResponseStatusOnly(http.StatusCreated)
	NoContent = ResponseStatusOnly(http.StatusNoContent)
)

func On(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context) (any, error)) {
	ctx := r.Context()
	obj, err := fn(ctx)
	if err != nil {
		Error(w, err)
		return
	}
	switch val := obj.(type) {
	case nil:
		// no action
	case ResponseStatusOnly:
		w.WriteHeader(int(val))
	default:
		Success(w, obj)
	}
}
