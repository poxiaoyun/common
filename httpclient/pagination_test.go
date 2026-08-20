package httpclient_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"xiaoshiai.cn/common/httpclient"
	"xiaoshiai.cn/common/meta"
)

func TestListAllFollowsPageResponses(t *testing.T) {
	requested := []meta.ListOptions{}
	result, err := httpclient.ListAll(t.Context(), meta.ListOptions{Search: "worker"}, func(_ context.Context, options meta.ListOptions) (meta.Page[int], error) {
		requested = append(requested, options)
		if options.Page == 0 {
			return meta.Page[int]{ResourceVersion: 7, Total: meta.Ptr(3), Items: []int{1, 2}, Page: 1, Size: 2}, nil
		}
		return meta.Page[int]{ResourceVersion: 7, Total: meta.Ptr(3), Items: []int{3}, Page: 2, Size: 2}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Items, []int{1, 2, 3}) || result.Total == nil || *result.Total != 3 || result.ResourceVersion != 7 {
		t.Fatalf("result = %#v", result)
	}
	if len(requested) != 2 || requested[1].Page != 2 || requested[1].Size != 2 || requested[1].Search != "worker" {
		t.Fatalf("requested = %#v", requested)
	}
}

func TestListAllFollowsEmptyContinuationResponse(t *testing.T) {
	result, err := httpclient.ListAll(t.Context(), meta.ListOptions{}, func(_ context.Context, options meta.ListOptions) (meta.Page[string], error) {
		switch options.Continue {
		case "":
			return meta.Page[string]{ResourceVersion: 1, Items: []string{"a"}, Continue: "empty", Limit: 2}, nil
		case "empty":
			return meta.Page[string]{ResourceVersion: 2, Items: nil, Continue: "last", Limit: 2}, nil
		default:
			return meta.Page[string]{ResourceVersion: 2, Items: []string{"b"}, Limit: 2}, nil
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Items, []string{"a", "b"}) || result.Total == nil || *result.Total != 2 || result.ResourceVersion != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestListAllDiscardsPartialResultOnError(t *testing.T) {
	calls := 0
	result, err := httpclient.ListAll(t.Context(), meta.ListOptions{}, func(_ context.Context, _ meta.ListOptions) (meta.Page[int], error) {
		calls++
		if calls == 1 {
			return meta.Page[int]{Items: []int{1}, Continue: "next", Limit: 1}, nil
		}
		return meta.Page[int]{}, errors.New("unavailable")
	})
	if err == nil || !reflect.DeepEqual(result, meta.Page[int]{}) {
		t.Fatalf("result = %#v, err = %v", result, err)
	}
}
