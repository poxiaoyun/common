package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	// json encoder writes a newline at the end
	if err := json.NewEncoder(&w.buf).Encode(data); err != nil {
		return err
	}
	w.buf.WriteString("\n")
	_, err := w.w.Write(w.buf.Bytes())
	flusher, ok := w.w.(http.Flusher)
	if ok {
		flusher.Flush()
	}
	return err
}

type Event struct {
	Id    string `json:"id,omitempty"`
	Event string `json:"event,omitempty"`
	Data  []byte `json:"data,omitempty"`
}

// NewSSEDecode decodes Server-Sent Events from the given reader.
// Caution: This data in event is []byte, it reused among events.
// If you want to keep the data, you should copy it.
func NewSSEDecode(ctx context.Context, r io.Reader, on func(Event) error) error {
	scan := bufio.NewReader(r)
	var dataBuffer bytes.Buffer
	var lastEventID string
	var eventType string
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		line, err := scan.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				return nil
			}
			if err == context.Canceled {
				return nil
			}
			return err
		}
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		if len(line) == 0 {
			// If the line is empty (a blank line). Dispatch the event.
			if err := on(Event{Id: lastEventID, Event: eventType, Data: dataBuffer.Bytes()}); err != nil {
				return err
			}
			dataBuffer.Reset()
			eventType = ""
			continue
		}
		// if the line starts with a colon, ignore the line
		if line[0] == byte(':') {
			continue
		}
		var field, value []byte
		if colonIndex := bytes.IndexRune(line, ':'); colonIndex != -1 {
			field, value = line[:colonIndex], line[colonIndex+1:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
		} else {
			field, value = line, []byte{}
		}
		switch string(field) {
		case "event":
			eventType = string(value)
		case "id":
			// If the field value does not contain U+0000 NULL, then set the last event ID buffer to the field value. Otherwise, ignore the field.
			if bytes.IndexByte(value, 0) == -1 {
				lastEventID = string(value)
			}
		case "data":
			dataBuffer.Write(value)
		case "retry":
		}
	}
}
