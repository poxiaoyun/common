package httpclient_test

import (
	"net/url"
	"strings"
	"testing"

	"xiaoshiai.cn/common/httpclient"
)

func TestBuilderSetsDeclaredContentLength(t *testing.T) {
	baseURL, err := url.Parse("https://files.example")
	if err != nil {
		t.Fatal(err)
	}
	builder := httpclient.Post("/assets").
		BaseAddr(baseURL).
		Body(strings.NewReader("content"), "text/plain").
		ContentLength(7)
	request, err := httpclient.BuildRequest(t.Context(), builder.R)
	if err != nil {
		t.Fatal(err)
	}
	if request.ContentLength != 7 {
		t.Fatalf("ContentLength = %d, want 7", request.ContentLength)
	}
}
