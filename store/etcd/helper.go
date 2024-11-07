package etcd

import (
	"reflect"
	"strings"

	"k8s.io/apimachinery/pkg/selection"
	"xiaoshiai.cn/common/store"
)

func RequirementMatchLabels(r store.Requirement, obj map[string]string) bool {
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
		return RequirementMatchIn(r.Values, obj[r.Key])
	case selection.NotEquals:
		return len(r.Values) == 1 && obj[r.Key] != r.Values[0]
	case selection.NotIn:
		return !RequirementMatchIn(r.Values, obj[r.Key])
	case selection.GreaterThan:
		return requirementValueCompare(obj[r.Key], r.Values[0]) > 0
	case selection.LessThan:
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
	for _, v := range in {
		if val == v {
			return true
		}
	}
	return false
}

func MatchLabelReqirements(obj store.Object, reqs store.Requirements) bool {
	if obj == nil {
		return false
	}
	return RequirementsMatchLabels(reqs, obj.GetLabels())
}

func MatchUnstructuredFieldRequirments(obj *store.Unstructured, reqs store.Requirements) bool {
	if obj == nil {
		return false
	}
	// TODO: implement this
	return true
}

func RequirementsMatchLabels(r store.Requirements, labels map[string]string) bool {
	for _, req := range r {
		if !RequirementMatchLabels(req, labels) {
			return false
		}
	}
	return true
}
