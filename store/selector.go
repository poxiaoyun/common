package store

import (
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

type Requirements []Requirement

func (r Requirements) String() string {
	var sb strings.Builder
	for i, requirement := range r {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(requirement.String())
	}
	return sb.String()
}

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

func (r Requirement) String() string {
	var sb strings.Builder
	sb.Grow(
		// length of r.key
		len(r.Key) +
			// length of 'r.operator' + 2 spaces for the worst case ('in' and 'notin')
			len(r.Operator) + 2 +
			// length of 'r.strValues' slice times. Heuristically 5 chars per word
			+5*len(r.Values))
	if r.Operator == selection.DoesNotExist {
		sb.WriteString("!")
	}
	sb.WriteString(r.Key)

	switch r.Operator {
	case selection.Equals:
		sb.WriteString("=")
	case selection.DoubleEquals:
		sb.WriteString("==")
	case selection.NotEquals:
		sb.WriteString("!=")
	case selection.In:
		sb.WriteString(" in ")
	case selection.NotIn:
		sb.WriteString(" notin ")
	case selection.GreaterThan:
		sb.WriteString(">")
	case selection.LessThan:
		sb.WriteString("<")
	case selection.Exists, selection.DoesNotExist:
		return sb.String()
	}

	switch r.Operator {
	case selection.In, selection.NotIn:
		sb.WriteString("(")
	}
	if len(r.Values) == 1 {
		sb.WriteString(AnyToString(r.Values[0]))
	} else {
		strValues := make([]string, 0, len(r.Values))
		for _, val := range r.Values {
			strValues = append(strValues, AnyToString(val))
		}
		sort.Strings(strValues)
		sb.WriteString(strings.Join(strValues, ","))
	}
	switch r.Operator {
	case selection.In, selection.NotIn:
		sb.WriteString(")")
	}
	return sb.String()
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
