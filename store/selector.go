package store

import (
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type Requirements []Requirement

func (r Requirements) MatchLabels(labels map[string]string) bool {
	for _, req := range r {
		if !req.MatchLabels(labels) {
			return false
		}
	}
	return true
}

func RequirementEqual(key string, value string) Requirement {
	return Requirement{
		Key:      key,
		Operator: selection.Equals,
		Values:   []string{value},
	}
}

func NewRequirement(key string, operator selection.Operator, values ...string) Requirement {
	return Requirement{Key: key, Operator: operator, Values: values}
}

type Requirement struct {
	Key      string
	Operator selection.Operator
	Values   []string
}

func (r Requirement) MatchLabels(obj map[string]string) bool {
	switch r.Operator {
	case selection.DoesNotExist:
		_, ok := obj[r.Key]
		return !ok
	case selection.Exists:
		_, ok := obj[r.Key]
		return ok
	case selection.Equals, selection.DoubleEquals:
		return len(r.Values) == 1 && obj[r.Key] == r.Values[0]
	case selection.In:
		return slices.Contains(r.Values, obj[r.Key])
	case selection.NotEquals:
		return len(r.Values) == 1 && obj[r.Key] != r.Values[0]
	case selection.NotIn:
		return !slices.Contains(r.Values, obj[r.Key])
	case selection.GreaterThan:
		return strings.Compare(obj[r.Key], r.Values[0]) > 0
	case selection.LessThan:
		return strings.Compare(obj[r.Key], r.Values[0]) < 0
	}
	return false
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
		list = append(list, Requirement{Key: r.Key(), Operator: r.Operator(), Values: r.Values().List()})
	}
	return list
}

func FieldsSelectorToReqirements(fields fields.Selector) Requirements {
	reqs := fields.Requirements()
	list := make([]Requirement, 0, len(reqs))
	for _, r := range reqs {
		list = append(list, Requirement{Key: r.Field, Operator: r.Operator, Values: []string{r.Value}})
	}
	return list
}

func MatchLabelReqirements(obj Object, reqs Requirements) bool {
	if obj == nil {
		return false
	}
	return reqs.MatchLabels(obj.GetLabels())
}

func MatchUnstructuredFieldRequirments(obj *Unstructured, reqs Requirements) bool {
	if obj == nil {
		return false
	}
	// TODO: implement this
	return true
}
