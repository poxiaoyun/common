package api

import (
	"fmt"
	"regexp"
	"strconv"
	"time"
)

// Regex to capture:
//
//	Group 1: +/- (operator, optional)
//	Group 2: number (value, optional)
//	Group 3: unit (offset unit, optional)
//	Group 4: / (separator, optional) -- Not captured directly, part of group 5's prefix
//	Group 5: rounding unit (optional)
var expressionPattern = regexp.MustCompile(`^(?:now)?(([+-])(\d+)([smhdwMy]))?(/([smhdwMy]))?$`)

// TimeOp represents a parsed time operation that hasn't been applied yet.
type TimeOp struct {
	Offset    time.Duration
	RoundUnit string // "d", "w", "M", "y", "h", "m", "s"
}

// Apply applies the time operation to a given time.
func (op *TimeOp) Apply(t time.Time) time.Time {
	if op.Offset != 0 {
		t = t.Add(op.Offset)
	}
	if op.RoundUnit != "" {
		t = TruncateTime(t, op.RoundUnit)
	}
	return t
}

// ParseTimeExpr parses a time expression relative to the current time.
// Supported syntax:
//
//	now
//	now-30m
//	now+1h
//	now/d (beginning of today)
//	now-1d/d (beginning of yesterday)
//	today (alias for now/d)
//	yesterday (alias for now-1d/d)
//	thisWeek (alias for now/w)
func ParseTimeExpr(expr string) (time.Time, error) {
	return ParseTimeExprWithNow(time.Now(), expr)
}

// ParseTimeOp parses a string expression into a structured TimeOp.
// Returns nil if the expression cannot be parsed as a relative time operation
// (e.g. it might be an absolute time string, or invalid).
func ParseTimeOp(expr string) (*TimeOp, error) {
	// Alias handling
	switch expr {
	case "now", "":
		return &TimeOp{}, nil
	case "today": // now/d
		return &TimeOp{RoundUnit: "d"}, nil
	case "yesterday": // now-1d/d
		return &TimeOp{Offset: -24 * time.Hour, RoundUnit: "d"}, nil
	case "thisWeek": // now/w
		return &TimeOp{RoundUnit: "w"}, nil
	}

	// Regex parsing
	matches := expressionPattern.FindStringSubmatch(expr)
	if matches == nil {
		return nil, nil // Not a relative expression match
	}

	op := &TimeOp{}

	// Handle Offset
	if matches[1] != "" {
		signStr := matches[2]
		valStr := matches[3]
		unit := matches[4]

		val, err := strconv.Atoi(valStr)
		if err != nil {
			return nil, fmt.Errorf("invalid number in expression %q: %v", expr, err)
		}

		if signStr == "-" {
			val = -val
		}

		switch unit {
		case "s":
			op.Offset = time.Duration(val) * time.Second
		case "m":
			op.Offset = time.Duration(val) * time.Minute
		case "h":
			op.Offset = time.Duration(val) * time.Hour
		case "d":
			op.Offset = time.Duration(val) * 24 * time.Hour
		case "w":
			op.Offset = time.Duration(val) * 7 * 24 * time.Hour
		case "M":
			op.Offset = time.Duration(val) * 30 * 24 * time.Hour
		case "y":
			op.Offset = time.Duration(val) * 365 * 24 * time.Hour
		}
	}

	// Handle Rounding
	if matches[5] != "" {
		op.RoundUnit = matches[6]
	}

	return op, nil
}

// ParseTimeExprWithNow is the core parsing logic allowing a custom 'now' time.
func ParseTimeExprWithNow(now time.Time, expr string) (time.Time, error) {
	// Try parsing as a relative operation first
	op, err := ParseTimeOp(expr)
	if err != nil {
		return time.Time{}, err
	}
	if op != nil {
		return op.Apply(now), nil
	}

	// Fallback: Try parsing as absolute time (RFC3339)
	return time.Parse(time.RFC3339, expr)
}

func TruncateTime(t time.Time, unit string) time.Time {
	switch unit {
	case "s":
		return t.Truncate(time.Second)
	case "m":
		return t.Truncate(time.Minute)
	case "h":
		return t.Truncate(time.Hour)
	case "d":
		// Truncate to day (00:00:00)
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	case "w":
		// Truncate to start of week (assuming Monday is start, or Sunday?
		// User request said: "周一 00:00:00 (或周日，取决于配置)".
		// Let's standard on Monday for ISO-like behavior commonly used in industry tools unless specified otherwise.
		weekday := t.Weekday()
		// If Sunday (0), we need to subtract 6 days to get to previous Monday.
		// If Monday (1), subtract 0.
		// If Tuesday (2), subtract 1.
		daysToSubtract := int(weekday) - 1
		if weekday == time.Sunday {
			daysToSubtract = 6
		}
		// First truncate to day
		dayStart := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		return dayStart.AddDate(0, 0, -daysToSubtract)
	case "M":
		// Truncate to start of month
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
	case "y":
		// Truncate to start of year
		return time.Date(t.Year(), 1, 1, 0, 0, 0, 0, t.Location())
	default:
		return t
	}
}

// Helper for format interval, kept for compatibility if used elsewhere,
// though the original file had it.
func FormatInterval(inter time.Duration) string {
	var res string
	year := time.Hour * 24 * 365
	day := time.Hour * 24
	if inter >= year {
		res += fmt.Sprintf("%dy", inter/year)
		inter %= year
	}
	if inter >= day {
		res += fmt.Sprintf("%dd", inter/day)
		inter %= day
	}
	if inter >= time.Hour {
		res += fmt.Sprintf("%dh", inter/time.Hour)
		inter %= time.Hour
	}
	if inter >= time.Minute {
		res += fmt.Sprintf("%dm", inter/time.Minute)
		inter %= time.Minute
	}
	if inter >= time.Second {
		res += fmt.Sprintf("%ds", inter/time.Second)
		inter %= time.Second
	}
	if inter >= time.Millisecond {
		res += fmt.Sprintf("%dms", inter/time.Millisecond)
		inter %= time.Millisecond
	}
	if inter >= time.Microsecond {
		res += fmt.Sprintf("%dµs", inter/time.Microsecond)
		inter %= time.Microsecond
	}
	if inter >= time.Nanosecond {
		res += fmt.Sprintf("%dns", inter/time.Nanosecond)
		inter %= time.Nanosecond
	}
	if res == "" {
		// Should be unreachable if inter > 0 because smallest unit is ns
		return "0s"
	}
	return res
}
