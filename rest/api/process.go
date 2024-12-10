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
		Raw(w, int(val.Code), val, nil)
	case ResponseStatusOnly:
		w.WriteHeader(int(val))
	default:
		Success(w, obj)
	}
}
