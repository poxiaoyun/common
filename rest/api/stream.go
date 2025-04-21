package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"

	"github.com/gorilla/websocket"
)

const (
	ContentTypeEventStream = "text/event-stream"
	ContentTypeJSONStream  = "application/stream+json"
)

type StreamEncoder[T any] interface {
	Encode(kind string, data T) error
	// Error sends an error to the client.
	SendError(err error) error
	Close() error
}

type StreamDecoder[T any] interface {
	Decode(ctx context.Context, on func(ctx context.Context, kind string, data T) error) error
}

func NewStreamDecoderFromResponse[T any](resp *http.Response) (StreamDecoder[T], error) {
	mimetype, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	return NewStreamDecoder[T](mimetype, resp.Body)
}

func NewStreamDecoder[T any](format string, r io.Reader) (StreamDecoder[T], error) {
	switch format {
	case "json", ContentTypeJSONStream, "application/json":
		return NewJSONStreamDecoder[T](r), nil
	case "sse", ContentTypeEventStream:
		return NewSSEReader[T](r), nil
	default:
		return NewSSEReader[T](r), nil
	}
}

func NewStreamEncoderFromRequest[T any](w http.ResponseWriter, r *http.Request) (StreamEncoder[T], error) {
	format := Query(r, "format", "")
	if format == "websocket" || websocket.IsWebSocketUpgrade(r) {
		return NewWebSocketEncoder[T](w, r)
	}
	if format == "" {
		// use accept header
		format, _, _ = mime.ParseMediaType(r.Header.Get("Accept"))
	}
	return NewStreamEncoder[T](w, format)
}

func NewStreamEncoder[T any](w http.ResponseWriter, format string) (StreamEncoder[T], error) {
	switch format {
	case "json", ContentTypeJSONStream:
		return NewJSONStreamEncoder[T](w), nil
	case "sse", ContentTypeEventStream:
		return NewSSEWriter[T](w), nil
	default:
		return NewSSEWriter[T](w), nil
	}
}

// https://developer.mozilla.org/en-US/docs/Web/API/Server-sent_events
// https://html.spec.whatwg.org/multipage/server-sent-events.html#server-sent-events
type ServerSendEventWriter[T any] struct {
	w   http.ResponseWriter
	buf bytes.Buffer
}

func NewSSEWriter[T any](w http.ResponseWriter) *ServerSendEventWriter[T] {
	w.Header().Set("Content-Type", ContentTypeEventStream)
	w.WriteHeader(http.StatusOK)
	return &ServerSendEventWriter[T]{w: w}
}

func (w *ServerSendEventWriter[T]) SendError(err error) error {
	return w.EncodeEvent("error", err.Error())
}

func (w *ServerSendEventWriter[T]) Encode(event string, data T) error {
	return w.EncodeEvent(event, data)
}

func (w *ServerSendEventWriter[T]) Close() error {
	return nil
}

func (w *ServerSendEventWriter[T]) EncodeEvent(event string, data any) error {
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

	return WriteFlush(w.w, w.buf.Bytes())
}

type Event struct {
	Id    string `json:"id,omitempty"`
	Event string `json:"event,omitempty"`
	Data  []byte `json:"data,omitempty"`
}

func NewSSEReader[T any](r io.Reader) *ServerSendEventDecoder[T] {
	return &ServerSendEventDecoder[T]{r: r}
}

type ServerSendEventDecoder[T any] struct {
	r io.Reader
}

func (d *ServerSendEventDecoder[T]) Decode(ctx context.Context, on func(ctx context.Context, kind string, data T) error) error {
	return NewSSEDecode(ctx, d.r, func(e Event) error {
		if e.Event == "error" {
			return fmt.Errorf("stream error: %s", string(e.Data))
		}
		data := *new(T)
		if err := json.Unmarshal(e.Data, &data); err != nil {
			return err
		}
		return on(ctx, e.Event, data)
	})
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

type JSONStreamDecoder[T any] struct {
	r *json.Decoder
}

func NewJSONStreamDecoder[T any](r io.Reader) *JSONStreamDecoder[T] {
	return &JSONStreamDecoder[T]{r: json.NewDecoder(r)}
}

type StreamItem[T any] struct {
	Kind  string `json:"kind"`
	Data  T      `json:"data"`
	Error string `json:"error,omitempty"`
}

func (d *JSONStreamDecoder[T]) Decode(ctx context.Context, on func(ctx context.Context, kind string, data T) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var data StreamItem[T]
			if err := d.r.Decode(&data); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if data.Error != "" {
				return fmt.Errorf("stream error: %s", data.Error)
			}
			if err := on(ctx, data.Kind, data.Data); err != nil {
				return err
			}
		}
	}
}

func NewJSONStreamEncoder[T any](w http.ResponseWriter) *JSONStreamEncoder[T] {
	w.Header().Set("Content-Type", ContentTypeJSONStream)
	w.WriteHeader(http.StatusOK)
	return &JSONStreamEncoder[T]{w: w, buf: &bytes.Buffer{}}
}

type JSONStreamEncoder[T any] struct {
	w   http.ResponseWriter
	buf *bytes.Buffer
}

func (w *JSONStreamEncoder[T]) Encode(kind string, data T) error {
	w.buf.Reset()
	if err := json.NewEncoder(w.buf).Encode(StreamItem[T]{Kind: kind, Data: data}); err != nil {
		return err
	}
	// w.buf.WriteByte('\n') // it's useful for client to decode json stream by lines.
	return WriteFlush(w.w, w.buf.Bytes())
}

func (w *JSONStreamEncoder[T]) SendError(err error) error {
	return json.NewEncoder(w.w).Encode(StreamItem[T]{Error: err.Error()})
}

func (w *JSONStreamEncoder[T]) Close() error {
	return nil
}

func WriteFlush(w io.Writer, data []byte) error {
	n, err := w.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
	return nil
}

type WebSocketEncoder[T any] struct {
	w    http.ResponseWriter
	conn *websocket.Conn
}

func NewWebSocketEncoder[T any](w http.ResponseWriter, r *http.Request) (*WebSocketEncoder[T], error) {
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return nil, err
	}
	return &WebSocketEncoder[T]{w: w, conn: conn}, nil
}

func (w *WebSocketEncoder[T]) Encode(kind string, data T) error {
	return w.conn.WriteJSON(StreamItem[T]{Kind: kind, Data: data})
}

func (w *WebSocketEncoder[T]) SendError(err error) error {
	return w.conn.WriteJSON(StreamItem[T]{Error: err.Error()})
}

func (w *WebSocketEncoder[T]) Close() error {
	return w.conn.Close()
}

type WebSocketDecoder[T any] struct {
	conn *websocket.Conn
}

func NewWebSocketDecoder[T any](conn *websocket.Conn) *WebSocketDecoder[T] {
	return &WebSocketDecoder[T]{conn: conn}
}

func (d *WebSocketDecoder[T]) Decode(ctx context.Context, on func(ctx context.Context, kind string, data T) error) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			var data StreamItem[T]
			if err := d.conn.ReadJSON(&data); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			if data.Error != "" {
				return fmt.Errorf("stream error: %s", data.Error)
			}
			if err := on(ctx, data.Kind, data.Data); err != nil {
				return err
			}
		}
	}
}
