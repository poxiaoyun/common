package api

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestParseTimeExpr(t *testing.T) {
	// Fixed "now" for testing: 2023-10-05 14:35:50
	refTime := time.Date(2023, 10, 5, 14, 35, 50, 0, time.UTC)

	tests := []struct {
		name     string
		expr     string
		expected time.Time
		wantErr  bool
	}{
		{
			name:     "Basic now",
			expr:     "now",
			expected: refTime,
		},
		{
			name:     "now-30m",
			expr:     "now-30m",
			expected: refTime.Add(-30 * time.Minute),
		},
		{
			name:     "now+1h",
			expr:     "now+1h",
			expected: refTime.Add(1 * time.Hour),
		},
		{
			name:     "Round to day (now/d)",
			expr:     "now/d",
			expected: time.Date(2023, 10, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Round to hour (now/h)",
			expr:     "now/h",
			expected: time.Date(2023, 10, 5, 14, 0, 0, 0, time.UTC),
		},
		{
			name:     "Yesterday logic (now-1d/d)",
			expr:     "now-1d/d",
			expected: time.Date(2023, 10, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Round to Month (now/M)",
			expr:     "now/M",
			expected: time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Round to Year (now/y)",
			expr:     "now/y",
			expected: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Alias today",
			expr:     "today",
			expected: time.Date(2023, 10, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Alias yesterday",
			expr:     "yesterday",
			expected: time.Date(2023, 10, 4, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "Alias thisWeek (Thursday, start of week logic)",
			expr: "thisWeek",
			// Thursday. Week start Monday -> Mon Oct 2
			expected: time.Date(2023, 10, 2, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Complex: now-1M/M (Start of last month)",
			expr:     "now-1M/M",
			expected: time.Date(2023, 9, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name:     "Omitting now (-30m)",
			expr:     "-30m",
			expected: refTime.Add(-30 * time.Minute),
		},
		{
			name:     "Omitting now (+1h)",
			expr:     "+1h",
			expected: refTime.Add(1 * time.Hour),
		},
		{
			name:     "Omitting now (/d)",
			expr:     "/d",
			expected: time.Date(2023, 10, 5, 0, 0, 0, 0, time.UTC),
		},
		{
			name:    "Invalid",
			expr:    "now-invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeExprWithNow(refTime, tt.expr)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				if !got.Equal(tt.expected) {
					t.Errorf("expected %v, got %v", tt.expected, got)
				}
			}
		})
	}
}

func TestParseTimeOp(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected *TimeOp
	}{
		{
			name:     "now",
			expr:     "now",
			expected: &TimeOp{},
		},
		{
			name:     "today",
			expr:     "today",
			expected: &TimeOp{RoundUnit: "d"},
		},
		{
			name: "yesterday",
			expr: "yesterday",
			expected: &TimeOp{
				Offset:    -24 * time.Hour,
				RoundUnit: "d",
			},
		},
		{
			name: "Offset only: now-30m",
			expr: "now-30m",
			expected: &TimeOp{
				Offset: -30 * time.Minute,
			},
		},
		{
			name: "Round only: now/h",
			expr: "now/h",
			expected: &TimeOp{
				RoundUnit: "h",
			},
		},
		{
			name: "Offset and Round: now-1d/d",
			expr: "now-1d/d",
			expected: &TimeOp{
				Offset:    -24 * time.Hour,
				RoundUnit: "d",
			},
		},
		{
			name:     "Invalid (e.g. absolute time) returns nil",
			expr:     "2023-01-01T00:00:00Z",
			expected: nil,
		},
		{
			name: "Omitting now: -30m",
			expr: "-30m",
			expected: &TimeOp{
				Offset: -30 * time.Minute,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseTimeOp(tt.expr)
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFormatInterval(t *testing.T) {
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
		got := FormatInterval(tt.input)
		assert.Equal(t, tt.expected, got)
	}
}
