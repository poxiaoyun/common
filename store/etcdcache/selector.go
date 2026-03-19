package etcdcache

import (
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"k8s.io/apiserver/pkg/storage"
	"xiaoshiai.cn/common/store"
)

func ConvertPredicate(l store.Requirements, f store.Requirements) (storage.SelectionPredicate, error) {
	labelssel := labels.Everything()
	fieldsel := fields.Everything()
	if l != nil {
		newlabelssel, err := requirementsToLabelsSelector(l)
		if err != nil {
			return storage.SelectionPredicate{}, err
		}
		labelssel = newlabelssel
	}
	if f != nil {
		newfieldsel, err := requirementsToFieldsSelector(f)
		if err != nil {
			return storage.SelectionPredicate{}, err
		}
		fieldsel = newfieldsel
	}
	fieldkeys := make([]string, 0, len(f))
	for _, req := range f {
		fieldkeys = append(fieldkeys, req.Key)
	}
	return storage.SelectionPredicate{
		Label:    labelssel,
		Field:    fieldsel,
		GetAttrs: GetAttrsFunc(fieldkeys),
	}, nil
}

func requirementsToLabelsSelector(reqs store.Requirements) (labels.Selector, error) {
	selector := labels.Everything()
	for _, req := range reqs {
		labelreq, err := labels.NewRequirement(req.Key, selection.Operator(req.Operator), store.AnyToStrings(req.Values))
		if err != nil {
			return nil, err
		}
		selector = selector.Add(*labelreq)
	}
	return selector, nil
}

func requirementsToFieldsSelector(reqs store.Requirements) (fields.Selector, error) {
	selectors := make([]fields.Selector, 0, len(reqs))
	for _, req := range reqs {
		switch req.Operator {
		case store.Equals, store.DoubleEquals:
			selectors = append(selectors, fields.OneTermEqualSelector(req.Key, store.AnyToString(req.Values[0])))
		case store.NotEquals:
			selectors = append(selectors, fields.OneTermNotEqualSelector(req.Key, store.AnyToString(req.Values[0])))
		case store.In:
			selectors = append(selectors, OneTermInSelector(req.Key, store.AnyToStrings(req.Values)))
		case store.NotIn:
			selectors = append(selectors, OneTermNotInSelector(req.Key, store.AnyToStrings(req.Values)))
		default:
			return nil, fmt.Errorf("unsupported field selector operator: %s", req.Operator)
		}
	}
	return fields.AndSelectors(selectors...), nil
}

func OneTermInSelector(key string, values []string) fields.Selector {
	return inTerm{field: key, values: values}
}

type inTerm struct {
	field  string
	values []string
}

// DeepCopySelector implements [fields.Selector].
func (i inTerm) DeepCopySelector() fields.Selector {
	valuesCopy := make([]string, len(i.values))
	copy(valuesCopy, i.values)
	return inTerm{field: i.field, values: valuesCopy}
}

// Empty implements [fields.Selector].
func (i inTerm) Empty() bool {
	return len(i.values) == 0
}

// Matches implements [fields.Selector].
func (i inTerm) Matches(fields fields.Fields) bool {
	return slices.Contains(i.values, fields.Get(i.field))
}

// Requirements implements [fields.Selector].
func (i inTerm) Requirements() fields.Requirements {
	return fields.Requirements{
		fields.Requirement{Field: i.field, Operator: selection.In, Value: strings.Join(i.values, ",")},
	}
}

// RequiresExactMatch implements [fields.Selector].
func (i inTerm) RequiresExactMatch(field string) (value string, found bool) {
	if i.field != field || len(i.values) != 1 {
		return "", false
	}
	return i.values[0], true
}

// String implements [fields.Selector].
func (i inTerm) String() string {
	return fmt.Sprintf("%s in (%s)", i.field, strings.Join(i.values, ","))
}

// Transform implements [fields.Selector].
func (i inTerm) Transform(fn fields.TransformFunc) (fields.Selector, error) {
	newfield, _, err := fn(i.field, "")
	if err != nil {
		return nil, err
	}
	if len(newfield) == 0 {
		return fields.Everything(), nil
	}
	return inTerm{field: newfield, values: i.values}, nil
}

var _ fields.Selector = inTerm{}

func OneTermNotInSelector(key string, values []string) fields.Selector {
	return notInTerm{field: key, values: values}
}

type notInTerm struct {
	field  string
	values []string
}

// DeepCopySelector implements [fields.Selector].
func (n notInTerm) DeepCopySelector() fields.Selector {
	valuesCopy := make([]string, len(n.values))
	copy(valuesCopy, n.values)
	return notInTerm{field: n.field, values: valuesCopy}
}

// Empty implements [fields.Selector].
func (n notInTerm) Empty() bool {
	return len(n.values) == 0
}

// Matches implements [fields.Selector].
func (n notInTerm) Matches(fields fields.Fields) bool {
	return !slices.Contains(n.values, fields.Get(n.field))
}

// Requirements implements [fields.Selector].
func (n notInTerm) Requirements() fields.Requirements {
	return fields.Requirements{
		fields.Requirement{Field: n.field, Operator: selection.NotIn, Value: strings.Join(n.values, ",")},
	}
}

// RequiresExactMatch implements [fields.Selector].
func (n notInTerm) RequiresExactMatch(string) (string, bool) {
	return "", false
}

// String implements [fields.Selector].
func (n notInTerm) String() string {
	return fmt.Sprintf("%s notin (%s)", n.field, strings.Join(n.values, ","))
}

// Transform implements [fields.Selector].
func (n notInTerm) Transform(fn fields.TransformFunc) (fields.Selector, error) {
	newfield, _, err := fn(n.field, "")
	if err != nil {
		return nil, err
	}
	if len(newfield) == 0 {
		return fields.Everything(), nil
	}
	return notInTerm{field: newfield, values: n.values}, nil
}

var _ fields.Selector = notInTerm{}
