package api_test

import (
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	commonerrors "xiaoshiai.cn/common/errors"
	"xiaoshiai.cn/common/meta"
	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func TestPageFromPaginationForms(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	tests := []struct {
		name string
		page int
		size int
		want meta.Page[int]
	}{
		{
			name: "unpaginated",
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{1, 2, 3}},
		},
		{
			name: "page",
			page: 2,
			size: 2,
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{3}, Page: 2, Size: 2},
		},
		{
			name: "negative page becomes first page",
			page: -2,
			size: 2,
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{1, 2}, Page: 1, Size: 2},
		},
		{
			name: "negative size becomes unpaginated",
			page: 2,
			size: -2,
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{1, 2, 3}},
		},
		{
			name: "maximum size returns available items",
			page: 1,
			size: maxInt,
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{1, 2, 3}, Page: 1, Size: maxInt},
		},
		{
			name: "maximum page and size return an empty page",
			page: maxInt,
			size: maxInt,
			want: meta.Page[int]{Total: meta.Ptr(3), Items: []int{}, Page: maxInt, Size: maxInt},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := api.PageFrom([]int{1, 2, 3}, test.page, test.size, nil, nil)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("PageFrom() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestPageFromListOptionsHandlesMaximumContinuationLimit(t *testing.T) {
	items := []pageListItem{{name: "a"}, {name: "b"}}
	limit := int(^uint(0) >> 1)
	page, err := api.PageFromListOptions(items, meta.ListOptions{Limit: limit}, pageListItemID, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := meta.Page[pageListItem]{Items: items, Limit: limit}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("PageFromListOptions() = %#v, want %#v", page, want)
	}
}

func TestPageFromPreparedListSlicesWithoutCollectionPreparation(t *testing.T) {
	got := api.PageFromPreparedList([]int{3, 1, 2}, 2, 1)
	want := meta.Page[int]{Total: meta.Ptr(3), Items: []int{1}, Page: 2, Size: 1}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PageFromPreparedList() = %#v, want %#v", got, want)
	}
}

func TestPageFromFilteringDoesNotClearInputTail(t *testing.T) {
	items := []int{1, 2, 3}
	page := api.PageFrom(items, 0, 0, func(item int) bool { return item%2 == 1 }, nil)
	if !reflect.DeepEqual(page.Items, []int{1, 3}) {
		t.Fatalf("PageFrom() items = %v, want [1 3]", page.Items)
	}
	if items[2] != 3 {
		t.Fatalf("PageFrom() cleared input tail: %v", items)
	}
}

type pageListItem struct {
	name string
}

type pageUIDOnlyItem string

func (item pageUIDOnlyItem) GetUID() string {
	return string(item)
}

func pageListItemName(item pageListItem) string {
	return item.name
}

func pageListItemID(item pageListItem) string {
	return "id-" + item.name
}

func TestPageFromListOptionsFollowsContinuationCursor(t *testing.T) {
	items := []pageListItem{{name: "c"}, {name: "a"}, {name: "b"}}
	first, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Page: 99, Size: 99, Sort: "name", Limit: 2},
		pageListItemID,
		pageListItemName,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantFirst := meta.Page[pageListItem]{
		Items:    []pageListItem{{name: "a"}, {name: "b"}},
		Continue: "id-b",
		Limit:    2,
	}
	if !reflect.DeepEqual(first, wantFirst) {
		t.Fatalf("first page = %#v, want %#v", first, wantFirst)
	}

	last, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Sort: "name", Continue: first.Continue, Limit: 2},
		pageListItemID,
		pageListItemName,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantLast := meta.Page[pageListItem]{Items: []pageListItem{{name: "c"}}, Limit: 2}
	if !reflect.DeepEqual(last, wantLast) {
		t.Fatalf("last page = %#v, want %#v", last, wantLast)
	}
}

func TestPageObjectFromListOptionsUsesStoreUIDForContinuationCursor(t *testing.T) {
	items := []store.ObjectMeta{
		{Name: "c", UID: "uid-c"},
		{Name: "a", UID: "uid-a"},
		{Name: "b", UID: "uid-b"},
	}
	page, err := api.PageObjectFromListOptions(items, meta.ListOptions{Sort: "name", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Continue != "uid-b" {
		t.Fatalf("PageObjectFromListOptions() continue = %q, want %q", page.Continue, "uid-b")
	}
	if got := []string{page.Items[0].Name, page.Items[1].Name}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("PageObjectFromListOptions() names = %v, want [a b]", got)
	}
}

func TestPageObjectFromListOptionsUsesKubernetesUIDForContinuationCursor(t *testing.T) {
	items := []metav1.PartialObjectMetadata{
		{ObjectMeta: metav1.ObjectMeta{Name: "c", UID: types.UID("uid-c")}},
		{ObjectMeta: metav1.ObjectMeta{Name: "a", UID: types.UID("uid-a")}},
		{ObjectMeta: metav1.ObjectMeta{Name: "b", UID: types.UID("uid-b")}},
	}
	page, err := api.PageObjectFromListOptions(items, meta.ListOptions{Sort: "name", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Continue != "uid-b" {
		t.Fatalf("PageObjectFromListOptions() continue = %q, want %q", page.Continue, "uid-b")
	}
	if got := []string{page.Items[0].Name, page.Items[1].Name}; !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("PageObjectFromListOptions() names = %v, want [a b]", got)
	}
}

func TestPageObjectFromListOptionsRequiresOnlyUIDForContinuation(t *testing.T) {
	items := []pageUIDOnlyItem{"uid-a", "uid-b"}
	page, err := api.PageObjectFromListOptions(items, meta.ListOptions{Sort: "unsupported", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	want := meta.Page[pageUIDOnlyItem]{Items: []pageUIDOnlyItem{"uid-a"}, Continue: "uid-a", Limit: 1}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("PageObjectFromListOptions() = %#v, want %#v", page, want)
	}
}

func TestPageFromListOptionsAppliesPaginationPrecedence(t *testing.T) {
	items := []pageListItem{{name: "a"}, {name: "b"}, {name: "c"}}
	page, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Page: 2, Size: 1, Continue: "ignored"},
		nil,
		pageListItemName,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPage := meta.Page[pageListItem]{
		Total: meta.Ptr(3),
		Items: []pageListItem{{name: "b"}},
		Page:  2,
		Size:  1,
	}
	if !reflect.DeepEqual(page, wantPage) {
		t.Fatalf("page result = %#v, want %#v", page, wantPage)
	}

	firstPage, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Page: -2, Size: 1},
		nil,
		pageListItemName,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantFirstPage := meta.Page[pageListItem]{
		Total: meta.Ptr(3),
		Items: []pageListItem{{name: "a"}},
		Page:  1,
		Size:  1,
	}
	if !reflect.DeepEqual(firstPage, wantFirstPage) {
		t.Fatalf("normalized page result = %#v, want %#v", firstPage, wantFirstPage)
	}

	unpaginated, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Page: 2, Continue: "ignored"},
		nil,
		pageListItemName,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	wantUnpaginated := meta.Page[pageListItem]{Total: meta.Ptr(3), Items: items}
	if !reflect.DeepEqual(unpaginated, wantUnpaginated) {
		t.Fatalf("unpaginated result = %#v, want %#v", unpaginated, wantUnpaginated)
	}
}

func TestPageFromListOptionsRejectsUnavailableContinuationCursor(t *testing.T) {
	items := []pageListItem{{name: "a"}, {name: "b"}}
	_, err := api.PageFromListOptions(
		items,
		meta.ListOptions{Continue: "missing", Limit: 1},
		pageListItemID,
		pageListItemName,
		nil,
	)
	var status *commonerrors.Status
	if !errors.As(err, &status) || status.Reason != commonerrors.StatusReasonResourceExpired {
		t.Fatalf("PageFromListOptions() error = %#v, want ResourceExpired", err)
	}
}

func TestPageFromListOptionsRequiresContinuationID(t *testing.T) {
	tests := []struct {
		name  string
		getID func(int) string
	}{
		{name: "missing function"},
		{name: "empty ID", getID: func(int) string { return "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := api.PageFromListOptions([]int{1, 2}, meta.ListOptions{Limit: 1}, test.getID, nil, nil)
			var status *commonerrors.Status
			if !errors.As(err, &status) || status.Reason != commonerrors.StatusReasonUnsupported {
				t.Fatalf("PageFromListOptions() error = %#v, want Unsupported", err)
			}
			if status.Message != "continuation pagination is not supported for this resource" {
				t.Fatalf("PageFromListOptions() message = %q", status.Message)
			}
		})
	}
}

func TestPageFromRequestUsesContinuationPrecedence(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?page=1&size=1&limit=1", nil)
	page, err := api.PageFromRequest(request, []pageListItem{{name: "a"}}, pageListItemID, pageListItemName, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := meta.Page[pageListItem]{Items: []pageListItem{{name: "a"}}, Limit: 1}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("PageFromRequest() = %#v, want %#v", page, want)
	}
}

func TestPageFromRequestSupportsContinuation(t *testing.T) {
	request := httptest.NewRequest("GET", "/objects?limit=2&sort=name", nil)
	items := []pageListItem{{name: "c"}, {name: "a"}, {name: "b"}}
	page, err := api.PageFromRequest(request, items, pageListItemID, pageListItemName, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := meta.Page[pageListItem]{
		Items:    []pageListItem{{name: "a"}, {name: "b"}},
		Continue: "id-b",
		Limit:    2,
	}
	if !reflect.DeepEqual(page, want) {
		t.Fatalf("PageFromRequest() = %#v, want %#v", page, want)
	}
}
