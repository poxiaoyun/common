package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServeContentResponseRedirectsLocation(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/asset", nil)
	response := httptest.NewRecorder()

	ServeContentResponse(response, request, ContentResponse{
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

	ServeContentResponse(response, request, ContentResponse{
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
