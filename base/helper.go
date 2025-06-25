package base

import (
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func ListOptionsToStoreListOptions(opts api.ListOptions) []store.ListOption {
	listOpts := []store.ListOption{}
	if opts.Size > 0 {
		listOpts = append(listOpts, store.WithPageSize(opts.Page, opts.Size))
	}
	if opts.Sort != "" {
		listOpts = append(listOpts, store.WithSort(opts.Sort))
	}
	if opts.Search != "" {
		listOpts = append(listOpts, store.WithSearch(opts.Search))
	}
	return listOpts
}
