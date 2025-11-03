package store

import (
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
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
		Operator: Equals,
		Values:   []any{value},
	}
}

func NewRequirement(key string, operator Operator, values ...any) Requirement {
	return Requirement{Key: key, Operator: operator, Values: values}
}

func NewCreationRangeRequirement(start, end time.Time) []Requirement {
	ret := make([]Requirement, 0, 2)
	if !start.IsZero() {
		ret = append(ret, NewRequirement("creationTimestamp", GreaterThanOrEqual, start))
	}
	if !end.IsZero() {
		ret = append(ret, NewRequirement("creationTimestamp", LessThanOrEqual, end))
	}
	return ret
}

type Operator string

const (
	DoesNotExist       Operator = "!"
	Equals             Operator = "="
	DoubleEquals       Operator = "=="
	In                 Operator = "in"
	NotEquals          Operator = "!="
	NotIn              Operator = "notin"
	Exists             Operator = "exists"
	GreaterThan        Operator = "gt"
	LessThan           Operator = "lt"
	GreaterThanOrEqual Operator = "gte"
	LessThanOrEqual    Operator = "lte"
	Contains           Operator = "contains" // slice contains element, string contains substring
	Like               Operator = "like"     // string contains substring
)

type Requirement struct {
	Key      string
	Operator Operator
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
	if r.Operator == DoesNotExist {
		sb.WriteString("!")
	}
	sb.WriteString(r.Key)

	switch r.Operator {
	case Equals:
		sb.WriteString("=")
	case DoubleEquals:
		sb.WriteString("==")
	case NotEquals:
		sb.WriteString("!=")
	case In:
		sb.WriteString(" in ")
	case NotIn:
		sb.WriteString(" notin ")
	case GreaterThan:
		sb.WriteString(">")
	case LessThan:
		sb.WriteString("<")
	case Exists, DoesNotExist:
		return sb.String()
	}

	switch r.Operator {
	case In, NotIn:
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
	case In, NotIn:
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
		list = append(list, Requirement{Key: r.Key(), Operator: Operator(r.Operator()), Values: StringsToAny(r.Values().List())})
	}
	return list
}

func FieldsSelectorToReqirements(fields fields.Selector) Requirements {
	reqs := fields.Requirements()
	list := make([]Requirement, 0, len(reqs))
	for _, r := range reqs {
		list = append(list, Requirement{Key: r.Field, Operator: Operator(r.Operator), Values: []any{r.Value}})
	}
	return list
}

func ParseRequirements(expr string) (Requirements, error) {
	if expr == "" {
		return nil, nil
	}
	sel, err := labels.Parse(expr)
	if err != nil {
		return nil, err
	}
	return LabelsSelectorToReqirements(sel), nil
}

func MatchLabelReqirements(obj Object, reqs Requirements) bool {
	if obj == nil {
		return false
	}
	return RequirementsMatchLabels(reqs, obj.GetLabels())
}

func MatchLabels(obj Object, labels map[string]string) bool {
	if len(labels) == 0 {
		return true
	}
	target := obj.GetLabels()
	if len(target) == 0 {
		return false
	}
	for k, v := range labels {
		if target[k] != v {
			return false
		}
	}
	return true
}

func RequirementsMatchLabels(r Requirements, labels map[string]string) bool {
	for _, req := range r {
		if !RequirementMatchLabels(req, labels) {
			return false
		}
	}
	return true
}

func RequirementMatchLabels(r Requirement, obj map[string]string) bool {
	switch r.Operator {
	case DoesNotExist:
		_, ok := obj[r.Key]
		return !ok
	case Exists:
		_, ok := obj[r.Key]
		return ok
	case Equals, DoubleEquals:
		return len(r.Values) == 1 && obj[r.Key] == r.Values[0]
	case In:
		return RequirementMatchIn(r.Values, obj[r.Key])
	case NotEquals:
		return len(r.Values) == 1 && obj[r.Key] != r.Values[0]
	case NotIn:
		return !RequirementMatchIn(r.Values, obj[r.Key])
	case GreaterThan:
		return requirementValueCompare(obj[r.Key], r.Values[0]) > 0
	case LessThan:
		return requirementValueCompare(obj[r.Key], r.Values[0]) < 0
	}
	return false
}

func requirementValueCompare(a, b any) int {
	refA := reflect.ValueOf(a)
	refB := reflect.ValueOf(b)
	if refA.Kind() != refB.Kind() {
		return -1
	}
	switch refA.Kind() {
	case reflect.String:
		return strings.Compare(refA.String(), refB.String())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(refA.Int() - refB.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(refA.Uint() - refB.Uint())
	case reflect.Float32, reflect.Float64:
		return int(refA.Float() - refB.Float())
	case reflect.Bool:
		if refA.Bool() == refB.Bool() {
			return 0
		}
		return -1
	default:
		return -1
	}
}

func RequirementMatchIn(val any, in ...any) bool {
	return slices.Contains(in, val)
}

func MatchUnstructuredFieldRequirments(obj *Unstructured, reqs Requirements) bool {
	if len(reqs) == 0 {
		return true
	}
	if obj == nil {
		return false
	}
	// TODO: implement this
	for _, req := range reqs {
		val, ok := GetNestedField(obj.Object, strings.Split(req.Key, ".")...)
		if !ok {
			if req.Operator == DoesNotExist {
				continue
			}
			return false
		}
		if req.Operator == DoesNotExist {
			return false
		}
		if req.Operator == Exists {
			continue
		}
		switch req.Operator {
		case Equals, DoubleEquals:
			if val != req.Values[0] {
				return false
			}
		case NotEquals:
			if val == req.Values[0] {
				return false
			}
		case In:
			if !slices.Contains(req.Values, val) {
				return false
			}
		case NotIn:
			if slices.Contains(req.Values, val) {
				return false
			}
		case GreaterThan:
			if requirementValueCompare(val, req.Values[0]) <= 0 {
				return false
			}
		case LessThan:
			if requirementValueCompare(val, req.Values[0]) >= 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}
