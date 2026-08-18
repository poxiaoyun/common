package store

import (
	"math"
	"math/big"
	"reflect"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"xiaoshiai.cn/common/meta"
)

type Requirements []Requirement

// ListOptionsFromMeta converts caller-facing list options into Store list
// options, including the label and field selector grammars.
func ListOptionsFromMeta(options meta.ListOptions) (ListOptions, error) {
	result := ListOptions{
		Page:     options.Page,
		Size:     options.Size,
		Search:   options.Search,
		Sort:     options.Sort,
		Continue: options.Continue,
	}
	if options.LabelSelector != "" {
		selector, err := labels.Parse(options.LabelSelector)
		if err != nil {
			return ListOptions{}, err
		}
		result.LabelRequirements = LabelsSelectorToReqirements(selector)
	}
	if options.FieldSelector != "" {
		selector, err := fields.ParseSelector(options.FieldSelector)
		if err != nil {
			return ListOptions{}, err
		}
		result.FieldRequirements = FieldsSelectorToReqirements(selector)
	}
	return result, nil
}

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
	case GreaterThanOrEqual:
		sb.WriteString(">=")
	case LessThanOrEqual:
		sb.WriteString("<=")
	case Contains:
		sb.WriteString(" contains ")
	case Like:
		sb.WriteString(" like ")
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
		return Requirements{}, nil
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
	value, exists := obj[r.Key]
	return requirementMatches(value, exists, r)
}

func requirementMatches(value any, exists bool, requirement Requirement) bool {
	values := requirement.Values
	switch requirement.Operator {
	case DoesNotExist:
		return !exists
	case Exists:
		return exists
	case Equals, DoubleEquals:
		return exists && len(values) == 1 && requirementValueEqual(value, values[0])
	case NotEquals:
		return len(values) == 1 && (!exists || !requirementValueEqual(value, values[0]))
	case In:
		return exists && len(values) > 0 && RequirementMatchIn(value, values...)
	case NotIn:
		return len(values) > 0 && (!exists || !RequirementMatchIn(value, values...))
	case GreaterThan, LessThan, GreaterThanOrEqual, LessThanOrEqual:
		if !exists || len(values) != 1 {
			return false
		}
		comparison, comparable := compareRequirementValues(value, values[0])
		if !comparable {
			return false
		}
		switch requirement.Operator {
		case GreaterThan:
			return comparison > 0
		case LessThan:
			return comparison < 0
		case GreaterThanOrEqual:
			return comparison >= 0
		case LessThanOrEqual:
			return comparison <= 0
		}
	case Contains:
		return exists && requirementContains(value, values)
	case Like:
		return exists && len(values) == 1 && requirementStringContains(value, values[0])
	}
	return false
}

func requirementValueEqual(a, b any) bool {
	if reflect.DeepEqual(a, b) {
		return true
	}
	comparison, comparable := compareRequirementValues(a, b)
	return comparable && comparison == 0
}

func compareRequirementValues(a, b any) (int, bool) {
	if timeA, okA := requirementTime(a); okA {
		if timeB, okB := requirementTime(b); okB {
			return timeA.Compare(timeB), true
		}
	}

	if numberA, okA := requirementNumber(a); okA {
		if numberB, okB := requirementNumber(b); okB {
			return numberA.Cmp(numberB), true
		}
	}

	valueA := indirectRequirementValue(a)
	valueB := indirectRequirementValue(b)
	if !valueA.IsValid() || !valueB.IsValid() || valueA.Kind() != valueB.Kind() {
		return 0, false
	}
	switch valueA.Kind() {
	case reflect.String:
		return strings.Compare(valueA.String(), valueB.String()), true
	case reflect.Bool:
		switch {
		case valueA.Bool() == valueB.Bool():
			return 0, true
		case !valueA.Bool():
			return -1, true
		default:
			return 1, true
		}
	}
	return 0, false
}

func indirectRequirementValue(value any) reflect.Value {
	ref := reflect.ValueOf(value)
	for ref.IsValid() && (ref.Kind() == reflect.Interface || ref.Kind() == reflect.Ptr) {
		if ref.IsNil() {
			return reflect.Value{}
		}
		ref = ref.Elem()
	}
	return ref
}

func requirementTime(value any) (time.Time, bool) {
	ref := indirectRequirementValue(value)
	if !ref.IsValid() {
		return time.Time{}, false
	}
	switch value := ref.Interface().(type) {
	case time.Time:
		return value, true
	case meta.Time:
		return value.Time, true
	case string:
		parsed, err := time.Parse(time.RFC3339Nano, value)
		return parsed, err == nil
	default:
		return time.Time{}, false
	}
}

func requirementNumber(value any) (*big.Rat, bool) {
	ref := indirectRequirementValue(value)
	if !ref.IsValid() {
		return nil, false
	}
	number := new(big.Rat)
	switch ref.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number.SetInt64(ref.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		number.SetInt(new(big.Int).SetUint64(ref.Uint()))
	case reflect.Float32, reflect.Float64:
		value := ref.Float()
		if math.IsNaN(value) || math.IsInf(value, 0) || number.SetFloat64(value) == nil {
			return nil, false
		}
	case reflect.String:
		if _, ok := number.SetString(ref.String()); !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	return number, true
}

func requirementContains(value any, expected []any) bool {
	if len(expected) == 0 {
		return false
	}
	ref := indirectRequirementValue(value)
	if !ref.IsValid() {
		return false
	}
	switch ref.Kind() {
	case reflect.String:
		for _, item := range expected {
			if !requirementStringContains(ref.String(), item) {
				return false
			}
		}
		return true
	case reflect.Slice, reflect.Array:
		for _, item := range expected {
			found := false
			for i := 0; i < ref.Len(); i++ {
				if requirementValueEqual(ref.Index(i).Interface(), item) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func requirementStringContains(value, expected any) bool {
	ref := indirectRequirementValue(value)
	if !ref.IsValid() || ref.Kind() != reflect.String {
		return false
	}
	expectedRef := indirectRequirementValue(expected)
	if !expectedRef.IsValid() || expectedRef.Kind() != reflect.String {
		return false
	}
	return strings.Contains(ref.String(), expectedRef.String())
}

func RequirementMatchIn(val any, in ...any) bool {
	for _, candidate := range in {
		if requirementValueEqual(val, candidate) {
			return true
		}
	}
	return false
}

func MatchUnstructuredFieldRequirments(obj *Unstructured, reqs Requirements) bool {
	if len(reqs) == 0 {
		return true
	}
	if obj == nil {
		return false
	}
	for _, req := range reqs {
		value, exists := GetNestedField(obj.Object, strings.Split(req.Key, ".")...)
		if !requirementMatches(value, exists, req) {
			return false
		}
	}
	return true
}
