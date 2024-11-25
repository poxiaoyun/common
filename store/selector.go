package store

import (
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type Requirements []Requirement

func RequirementEqual(key string, value any) Requirement {
	return Requirement{
		Key:      key,
		Operator: selection.Equals,
		Values:   []any{value},
	}
}

func NewRequirement(key string, operator selection.Operator, values ...any) Requirement {
	return Requirement{Key: key, Operator: operator, Values: values}
}

func NewCreationRangeRequirement(start, end time.Time) []Requirement {
	ret := make([]Requirement, 0, 2)
	if !start.IsZero() {
		ret = append(ret, NewRequirement("creationTimestamp", selection.GreaterThan, start))
	}
	if !end.IsZero() {
		ret = append(ret, NewRequirement("creationTimestamp", selection.LessThan, end))
	}
	return ret
}

type Requirement struct {
	Key      string
	Operator selection.Operator
	Values   []any
}

func RequirementsFromMap(kvs map[string]string) Requirements {
	var reqs Requirements
	for k, v := range kvs {
		reqs = append(reqs, RequirementEqual(k, v))
	}
	return reqs
}

func LabelsSelectorToReqirements(labels labels.Selector) Requirements {
	reqs, _ := labels.Requirements()
	list := make([]Requirement, 0, len(reqs))
	for _, r := range reqs {
		list = append(list, Requirement{Key: r.Key(), Operator: r.Operator(), Values: StringsToAny(r.Values().List())})
	}
	return list
}

func FieldsSelectorToReqirements(fields fields.Selector) Requirements {
	reqs := fields.Requirements()
	list := make([]Requirement, 0, len(reqs))
	for _, r := range reqs {
		list = append(list, Requirement{Key: r.Field, Operator: r.Operator, Values: []any{r.Value}})
	}
	return list
}

func ParseRequirements(expr string) (Requirements, error) {
	sel, err := labels.Parse(expr)
	if err != nil {
		return nil, err
	}
	return LabelsSelectorToReqirements(sel), nil
}
