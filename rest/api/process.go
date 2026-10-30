package api

import (
	"context"
	"net/http"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/log"
)

type ResponseStatusOnly int32

var (
	Empty     = ResponseStatusOnly(http.StatusOK)
	Accepted  = ResponseStatusOnly(http.StatusAccepted)
	Created   = ResponseStatusOnly(http.StatusCreated)
	NoContent = ResponseStatusOnly(http.StatusNoContent)
)

// ResponseStatus is an explicit status code with an optional response body.
type ResponseStatus struct {
	Status int
	Body   any
}

// Response returns an explicit HTTP response with a status and body. Handlers
// use it instead of writing the response writer directly and returning nil.
func Response(status int, body any) ResponseStatus {
	return ResponseStatus{Status: status, Body: body}
}

func On(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context) (any, error)) {
	ctx := r.Context()
	obj, err := fn(ctx)
	if err != nil {
		Error(w, err)
		log.FromContext(ctx).Error(err, "response")
		return
	}
	switch val := obj.(type) {
	case nil:
		// no action
	case *errors.Status:
		Raw(w, int(val.Code), val)
	case ResponseStatusOnly:
		w.WriteHeader(int(val))
	case ResponseStatus:
		if val.Body == nil {
			w.WriteHeader(val.Status)
			return
		}
		Raw(w, val.Status, WrapOK(val.Body))
	default:
		Success(w, obj)
	}
}

func OnCurrentSubject(w http.ResponseWriter, r *http.Request, fn func(ctx context.Context, subjectID string) (any, error)) {
	On(w, r, func(ctx context.Context) (any, error) {
		authentication := AuthenticationFromContext(ctx)
		if authentication.ID == "" {
			return nil, errors.NewBadRequest("require login")
		}
		return fn(ctx, authentication.ID)
	})
}
