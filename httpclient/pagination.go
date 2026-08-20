package httpclient

import (
	"context"

	"xiaoshiai.cn/common/meta"
)

// ListPageFunc retrieves one list response using the supplied options.
type ListPageFunc[T any] func(ctx context.Context, options meta.ListOptions) (meta.Page[T], error)

// ListAll retrieves list responses until the server reports no next page.
// It accepts page-style, continuation-style, and one-shot responses. An error
// discards partial results so callers cannot mistake incomplete data for a
// complete list. ListAll trusts the supplied list operation and its responses.
func ListAll[T any](ctx context.Context, options meta.ListOptions, list ListPageFunc[T]) (meta.Page[T], error) {
	request := options
	items := []T{}
	var resourceVersion int64
	stableResourceVersion := true
	firstResponse := true

	for {
		page, err := list(ctx, request)
		if err != nil {
			return meta.Page[T]{}, err
		}
		items = append(items, page.Items...)

		if firstResponse {
			resourceVersion = page.ResourceVersion
			stableResourceVersion = resourceVersion > 0
			firstResponse = false
		} else if page.ResourceVersion == 0 || page.ResourceVersion != resourceVersion {
			stableResourceVersion = false
		}

		if page.Limit > 0 {
			if page.Continue == "" {
				break
			}
			request.Continue = page.Continue
			request.Page = 0
			request.Size = 0
			request.Limit = page.Limit
			continue
		}

		if page.Page <= 0 {
			break
		}
		if len(page.Items) == 0 || *page.Total <= len(items) {
			break
		}
		request.Page = page.Page + 1
		request.Size = page.Size
		request.Continue = ""
		request.Limit = 0
	}

	total := len(items)
	result := meta.Page[T]{Total: &total, Items: items}
	if stableResourceVersion {
		result.ResourceVersion = resourceVersion
	}
	return result, nil
}
