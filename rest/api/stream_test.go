package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestSSEWriterEncodeEventWithoutNamePreservesRawMessage(t *testing.T) {
	response := httptest.NewRecorder()
	writer := NewSSEWriter[any](response)
	data := json.RawMessage(`{"stage":"Pushing", "detail":{"layer":2}}`)
	if err := writer.EncodeEvent("", data); err != nil {
		t.Fatal(err)
	}
	if got, want := response.Body.String(), "data: "+string(data)+"\n\n"; got != want {
		t.Fatalf("event = %q, want %q", got, want)
	}
}
