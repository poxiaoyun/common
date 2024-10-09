package api

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
// https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
type ServerSendEventWriter struct {
	w   http.ResponseWriter
	buf bytes.Buffer
}

func NewSSEWriter(w http.ResponseWriter) *ServerSendEventWriter {
	w.Header().Set("Content-Type", "text/event-stream")
	return &ServerSendEventWriter{w: w}
}

func (w *ServerSendEventWriter) WriteEvent(event string, data any) error {
	w.buf.Reset()
	w.buf.WriteString("event: ")
	w.buf.WriteString(event)
	w.buf.WriteString("\n")
	w.buf.WriteString("data: ")
	if err := json.NewEncoder(&w.buf).Encode(data); err != nil {
		return err
	}
	w.buf.WriteString("\n\n")
	_, err := w.w.Write(w.buf.Bytes())
	flusher, ok := w.w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return err
}
