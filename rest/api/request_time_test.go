package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTimeExpr(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected *TimeExpr
	}{
		{
			name:     "now",
			expr:     "now",
			expected: &TimeExpr{},
		},
		{
			name:     "today",
			expr:     "today",
			expected: &TimeExpr{RoundUnit: "d"},
		},
		{
			name: "yesterday",
			expr: "yesterday",
			expected: &TimeExpr{
				Offset:    -24 * time.Hour,
				RoundUnit: "d",
			},
		},
		{
			name: "Offset only: now-30m",
			expr: "now-30m",
			expected: &TimeExpr{
				Offset: -30 * time.Minute,
			},
		},
		{
			name: "Round only: now/h",
			expr: "now/h",
			expected: &TimeExpr{
				RoundUnit: "h",
			},
		},
		{
			name: "Offset and Round: now-1d/d",
			expr: "now-1d/d",
			expected: &TimeExpr{
				Offset:    -24 * time.Hour,
				RoundUnit: "d",
			},
		},
		{
			name:     "RFC3339",
			expr:     "2023-01-01T00:00:00Z",
			expected: &TimeExpr{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:     "RFC3339Nano",
			expr:     "2023-01-01T00:00:00.123456789Z",
			expected: &TimeExpr{Time: time.Date(2023, 1, 1, 0, 0, 0, 123456789, time.UTC)},
		},
		{
			name:     "RFC3339 with +08:00",
			expr:     "2023-01-01T12:00:00+08:00",
			expected: &TimeExpr{Time: time.Date(2023, 1, 1, 12, 0, 0, 0, time.FixedZone("", 8*3600))},
		},
		{
			name:     "RFC3339 with -05:00",
			expr:     "2023-01-01T12:00:00-05:00",
			expected: &TimeExpr{Time: time.Date(2023, 1, 1, 12, 0, 0, 0, time.FixedZone("", -5*3600))},
		},
		{
			name:     "Date only: 2023-01-01",
			expr:     "2023-01-01",
			expected: &TimeExpr{Time: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		{
			name:     "thisWeek",
			expr:     "thisWeek",
			expected: &TimeExpr{RoundUnit: "w"},
		},
		{
			name:     "thisMonth",
			expr:     "thisMonth",
			expected: &TimeExpr{RoundUnit: "M"},
		},
		{
			name:     "thisYear",
			expr:     "thisYear",
			expected: &TimeExpr{RoundUnit: "y"},
		},
		{
			name: "Omitting now: -30m",
			expr: "-30m",
			expected: &TimeExpr{
				Offset: -30 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeExpr(tt.expr)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"10s", 10 * time.Second, false},
		{"5m", 5 * time.Minute, false},
		{"2h", 2 * time.Hour, false},
		{"1d", 24 * time.Hour, false},
		{"1w", 7 * 24 * time.Hour, false},
		{"1M", 30 * 24 * time.Hour, false},
		{"1y", 365 * 24 * time.Hour, false},
		{"-1M", -30 * 24 * time.Hour, false},
		{"-1y", -365 * 24 * time.Hour, false},
		{"1d2h", 26 * time.Hour, false},
		{"1y1M", 395 * 24 * time.Hour, false},
		{"-1d2h", -26 * time.Hour, false},
		{"-30m", -30 * time.Minute, false},
		{"100ns", 100 * time.Nanosecond, false},
		{"10us", 10 * time.Microsecond, false},
		{"10µs", 10 * time.Microsecond, false},
		{"500ms", 500 * time.Millisecond, false},
		{"1s500ms", 1500 * time.Millisecond, false},
		{"invalid", 0, true},
		{"10x", 0, true}, // invalid unit
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseDuration(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, got)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		input    time.Duration
		expected string
	}{
		{time.Hour + 30*time.Minute, "1h30m"},
		{25 * time.Hour, "1d1h"},
		{366 * 24 * time.Hour, "1y1d"},
		{time.Millisecond, "1ms"},
		{0, "0s"},
		{500 * time.Microsecond, "500µs"},
		{500 * time.Nanosecond, "500ns"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.input)
		assert.Equal(t, tt.expected, got)
	}
}
