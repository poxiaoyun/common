package rest

import (
	"net/http"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

// ListOptionsFromRequest converts public request list options, including
// caller-owned request defaults, into Store modifiers.
func ListOptionsFromRequest(r *http.Request, defaults ...meta.ListOption) ([]store.ListOption, error) {
	options, err := store.ListOptionsFromMeta(api.GetListOptions(r, defaults...))
	if err != nil {
		return nil, errors.NewBadRequest(err.Error())
	}
	return options, nil
}
