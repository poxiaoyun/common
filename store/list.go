package store

import (
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"xiaoshiai.cn/common/meta"
)

// metaListOption is the parsed public portion of ListOptions. It intentionally
// cannot set Store HTTP protocol fields such as ResourceVersion, Fields, or
// IncludeSubScopes.
type metaListOption struct {
	Page              int
	Size              int
	Continue          string
	Limit             int
	Search            string
	Sort              string
	LabelRequirements Requirements
	FieldRequirements Requirements
}

func (option metaListOption) ApplyToList(options *ListOptions) {
	options.Page = option.Page
	options.Size = option.Size
	options.Search = option.Search
	options.Sort = option.Sort
	options.Continue = option.Continue
	options.Limit = option.Limit
	options.LabelRequirements = append(options.LabelRequirements, option.LabelRequirements...)
	options.FieldRequirements = append(options.FieldRequirements, option.FieldRequirements...)
}

// ListOptionsFromMeta converts caller-facing list options into concrete Store
// modifiers, including selector parsing, followed by Store modifiers.
func ListOptionsFromMeta(options meta.ListOptions, modifiers ...ListOption) ([]ListOption, error) {
	resolved := metaListOption{
		Page:     options.Page,
		Size:     options.Size,
		Search:   options.Search,
		Sort:     options.Sort,
		Continue: options.Continue,
		Limit:    options.Limit,
	}
	if options.LabelSelector != "" {
		selector, err := labels.Parse(options.LabelSelector)
		if err != nil {
			return nil, err
		}
		resolved.LabelRequirements = LabelsSelectorToReqirements(selector)
	}
	if options.FieldSelector != "" {
		selector, err := fields.ParseSelector(options.FieldSelector)
		if err != nil {
			return nil, err
		}
		resolved.FieldRequirements = FieldsSelectorToReqirements(selector)
	}
	result := make([]ListOption, 0, 1+len(modifiers))
	result = append(result, resolved)
	return append(result, modifiers...), nil
}
