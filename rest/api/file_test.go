package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"xiaoshiai.cn/common/rest/api"
)

func TestServeContentResponseRedirectsLocation(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	response := httptest.NewRecorder()

	api.ServeContentResponse(response, request, api.ContentResponse{
		Location: "https://objects.example.test/file?signature=value",
		Headers:  http.Header{"X-Asset": []string{"image"}},
	})

	if response.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusTemporaryRedirect)
	}
	if got := response.Header().Get("Location"); got != "https://objects.example.test/file?signature=value" {
		t.Fatalf("location = %q", got)
	}
	if got := response.Header().Get("X-Asset"); got != "image" {
		t.Fatalf("X-Asset = %q", got)
	}
}

func TestServeContentResponseServesContent(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	response := httptest.NewRecorder()

	api.ServeContentResponse(response, request, api.ContentResponse{
		Content:       io.NopCloser(strings.NewReader("asset")),
		ContentLength: 5,
		Headers:       http.Header{"Content-Type": []string{"text/plain"}},
	})

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Body.String(); got != "asset" {
		t.Fatalf("body = %q", got)
	}
}

func TestServeContentResponseWritesResolvedRangeWithoutApplyingRangeAgain(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	request.Header.Set("Range", "bytes=1-3")
	response := httptest.NewRecorder()

	api.ServeContentResponse(response, request, api.ContentResponse{
		Headers:       http.Header{"Content-Range": []string{"bytes 1-3/5"}},
		Content:       io.NopCloser(strings.NewReader("sse")),
		ContentLength: 3,
	})

	if response.Code != http.StatusPartialContent || response.Body.String() != "sse" {
		t.Fatalf("response = status %d, body %q", response.Code, response.Body.String())
	}
}

func TestServePartialContentUsesArgumentsForMissingHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodHead, "/asset", nil)
	response := httptest.NewRecorder()

	api.ServePartialContent(response, request, strings.NewReader("sse"), 3, "bytes 1-3/5")

	if response.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusPartialContent)
	}
	if got := response.Header().Get("Content-Range"); got != "bytes 1-3/5" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := response.Header().Get("Content-Length"); got != "3" {
		t.Fatalf("Content-Length = %q", got)
	}
}

func TestServePartialContentPreservesExistingHeaders(t *testing.T) {
	request := httptest.NewRequest(http.MethodHead, "/asset", nil)
	response := httptest.NewRecorder()
	response.Header().Set("Content-Range", "bytes 4-6/10")
	response.Header().Set("Content-Length", "3")

	api.ServePartialContent(response, request, strings.NewReader("sse"), 7, "bytes 1-7/7")

	if got := response.Header().Get("Content-Range"); got != "bytes 4-6/10" {
		t.Fatalf("Content-Range = %q", got)
	}
	if got := response.Header().Get("Content-Length"); got != "3" {
		t.Fatalf("Content-Length = %q", got)
	}
}

func TestServePartialContentCopiesUnknownLengthToEOF(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	response := httptest.NewRecorder()

	api.ServePartialContent(response, request, strings.NewReader("sse"), 0, "bytes 1-3/5")

	if got := response.Body.String(); got != "sse" {
		t.Fatalf("body = %q", got)
	}
	if got := response.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q", got)
	}
}

func TestServePartialContentDoesNotLimitContentToDeclaredLength(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	response := httptest.NewRecorder()

	api.ServePartialContent(response, request, strings.NewReader("content"), 3, "bytes 0-2/7")

	if got := response.Header().Get("Content-Length"); got != "3" {
		t.Fatalf("Content-Length = %q", got)
	}
	if got := response.Body.String(); got != "content" {
		t.Fatalf("body = %q", got)
	}
}
