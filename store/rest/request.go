package rest

import (
	"net/http"
	"strconv"
	"strings"

	"xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

// ListOptionsFromRequest converts HTTP list query parameters into Store list
// options.
func ListOptionsFromRequest(r *http.Request) (store.ListOptions, error) {
	options, err := store.ListOptionsFromMeta(api.GetListOptions(r))
	if err != nil {
		return store.ListOptions{}, errors.NewBadRequest(err.Error())
	}
	options.IncludeSubScopes = api.Query(r, "includeSubscopes", false)
	if resourceVersion := api.Query(r, "resourceVersion", ""); resourceVersion != "" {
		parsed, err := strconv.ParseInt(resourceVersion, 10, 64)
		if err != nil {
			return store.ListOptions{}, errors.NewBadRequest("resourceVersion must be an integer")
		}
		options.ResourceVersion = &parsed
	}
	if fields := api.Query(r, "fields", ""); fields != "" {
		options.Fields = strings.Split(fields, ",")
	}
	return options, nil
}

// WatchOptionsFromListOptions carries selectors and snapshot controls from a
// list request into a Watch request.
func WatchOptionsFromListOptions(options store.ListOptions) store.WatchOptions {
	return store.WatchOptions{
		LabelRequirements: options.LabelRequirements,
		FieldRequirements: options.FieldRequirements,
		ResourceVersion:   options.ResourceVersion,
		IncludeSubScopes:  options.IncludeSubScopes,
	}
}
