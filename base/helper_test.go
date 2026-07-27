package base

import (
	"testing"

	"xiaoshiai.cn/common/rest/api"
	"xiaoshiai.cn/common/store"
)

func TestListOptionsToStoreListOptionsIncludesContinue(t *testing.T) {
	listOptions, err := ListOptionsToStoreListOptions(api.ListOptions{
		Size:     25,
		Continue: "next-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	options := store.ListOptions{}
	for _, option := range listOptions {
		option(&options)
	}
	if options.Size != 25 || options.Continue != "next-token" {
		t.Fatalf("unexpected store options: %#v", options)
	}
}
