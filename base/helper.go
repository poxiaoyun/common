package base

import (
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func ListOptionsToStoreListOptions(opts api.ListOptions) ([]store.ListOption, error) {
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
	if opts.LabelSelector != "" {
		labelsSelector, err := ParseLabelSelector(opts.LabelSelector)
		if err != nil {
			return nil, err
		}
		listOpts = append(listOpts, store.WithLabelRequirementsFromSelector(labelsSelector))
	}
	if opts.FieldSelector != "" {
		fieldsSelector, err := store.ParseRequirements(opts.FieldSelector)
		if err != nil {
			return nil, err
		}
		listOpts = append(listOpts, store.WithFieldRequirements(fieldsSelector...))
	}
	return listOpts, nil
}
