package reflect

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"
)

type textValue struct {
	Value   string
	Decoder string
}

func (value *textValue) MarshalText() ([]byte, error) {
	return []byte(value.Value), nil
}

func (value *textValue) UnmarshalText(text []byte) error {
	value.Value = string(text)
	value.Decoder = "text"
	return nil
}

type jsonValue struct {
	Value   string
	Decoder string
}

func (value jsonValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Value string `json:"value"`
	}{Value: value.Value})
}

func (value *jsonValue) UnmarshalJSON(data []byte) error {
	decoded := struct {
		Value string `json:"value"`
	}{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	value.Value = decoded.Value
	value.Decoder = "json"
	return nil
}

type textAndJSONValue struct {
	Value   string
	Decoder string
}

func (value textAndJSONValue) MarshalText() ([]byte, error) {
	return []byte(value.Value), nil
}

func (value textAndJSONValue) MarshalJSON() ([]byte, error) {
	return json.Marshal("json:" + value.Value)
}

func (value *textAndJSONValue) UnmarshalText(text []byte) error {
	value.Value = string(text)
	value.Decoder = "text"
	return nil
}

func (value *textAndJSONValue) UnmarshalJSON([]byte) error {
	value.Decoder = "json"
	return nil
}

var errInvalidText = errors.New("invalid text")

type failingTextValue struct {
	Decoder string
}

func (failingTextValue) MarshalText() ([]byte, error) {
	return nil, errInvalidText
}

func (failingTextValue) MarshalJSON() ([]byte, error) {
	return []byte(`"json"`), nil
}

func (value *failingTextValue) UnmarshalText([]byte) error {
	value.Decoder = "text"
	return errInvalidText
}

func (value *failingTextValue) UnmarshalJSON([]byte) error {
	value.Decoder = "json"
	return nil
}

func TestFormatValueAndSetTextValue(t *testing.T) {
	instant := time.Date(2026, time.August, 18, 22, 20, 57, 0, time.UTC)
	tests := []struct {
		name   string
		value  any
		text   string
		target any
		want   any
	}{
		{name: "string", value: "value", text: "value", target: new(string), want: "value"},
		{name: "duration", value: 30 * time.Second, text: "30s", target: new(time.Duration), want: 30 * time.Second},
		{name: "time", value: instant, text: "2026-08-18T22:20:57Z", target: new(time.Time), want: instant},
		{
			name:   "text interfaces",
			value:  textValue{Value: "text"},
			text:   "text",
			target: new(textValue),
			want:   textValue{Value: "text", Decoder: "text"},
		},
		{
			name:   "JSON interfaces",
			value:  jsonValue{Value: "json"},
			text:   `{"value":"json"}`,
			target: new(jsonValue),
			want:   jsonValue{Value: "json", Decoder: "json"},
		},
		{
			name:   "text before JSON",
			value:  textAndJSONValue{Value: "text"},
			text:   "text",
			target: new(textAndJSONValue),
			want:   textAndJSONValue{Value: "text", Decoder: "text"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			formatted, err := FormatValue(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if formatted != test.text {
				t.Fatalf("FormatValue() = %q, want %q", formatted, test.text)
			}
			if err := SetTextValue(reflect.ValueOf(test.target).Elem(), formatted); err != nil {
				t.Fatal(err)
			}
			if got := reflect.ValueOf(test.target).Elem().Interface(); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("parsed value = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTextValueDoesNotFallbackAfterTextError(t *testing.T) {
	value := failingTextValue{}
	if _, err := FormatValue(value); !errors.Is(err, errInvalidText) {
		t.Fatalf("format error = %v, want %v", err, errInvalidText)
	}
	err := SetTextValue(reflect.ValueOf(&value).Elem(), "invalid")
	if !errors.Is(err, errInvalidText) {
		t.Fatalf("error = %v, want %v", err, errInvalidText)
	}
	if value.Decoder != "text" {
		t.Fatalf("decoder = %q, want text", value.Decoder)
	}
}
