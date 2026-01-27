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
var expressionPattern = regexp.MustCompile(`^(?:now)?([+-](?:\d+(?:ns|us|µs|ms|s|m|h|d|w|M|y))+)?(/([smhdwMy]))?$`)

// TimeExpr represents a parsed time expression that hasn't been applied yet.
type TimeExpr struct {
	Offset    time.Duration
	Time      time.Time
	RoundUnit string // "d", "w", "M", "y", "h", "m", "s"
}

// Apply applies the time operation to a given time.
func (op *TimeExpr) Apply(t time.Time) time.Time {
	if !op.Time.IsZero() {
		return op.Time
	}
	if op.Offset != 0 {
		t = t.Add(op.Offset)
	}
	if op.RoundUnit != "" {
		t = TruncateTime(t, op.RoundUnit)
	}
	return t
}

// ParseTimeExpr parses a string expression into a structured TimeExpr.
// example:
//
//	"now"
//	"now-30m"
//	"now+1h"
//	"now/d" (beginning of today)
//	"now-1d/d" (beginning of yesterday)
//	"today" (alias for now/d)
//	"yesterday" (alias for now-1d/d)
//	"thisWeek" (alias for now/w)
//	"thisMonth" (alias for now/M)
//	"thisYear" (alias for now/y)
//	"2022-01-01" (absolute time)
//	"2022-01-01T00:00:00Z" (RFC3339)
//	"2022-01-01T00:00:00.000000000+08:00" (RFC3339nano)
//
// Returns nil if the expression cannot be parsed as a relative time operation
// (e.g. it might be an absolute time string, or invalid).
func ParseTimeExpr(expr string) (*TimeExpr, error) {
	// Alias handling
	switch expr {
	case "now", "":
		return &TimeExpr{}, nil
	case "today": // now/d
		return &TimeExpr{RoundUnit: "d"}, nil
	case "yesterday": // now-1d/d
		return &TimeExpr{Offset: -24 * time.Hour, RoundUnit: "d"}, nil
	case "thisWeek": // now/w
		return &TimeExpr{RoundUnit: "w"}, nil
	case "thisMonth": // now/M
		return &TimeExpr{RoundUnit: "M"}, nil
	case "thisYear": // now/y
		return &TimeExpr{RoundUnit: "y"}, nil
	}

	// Regex parsing
	matches := expressionPattern.FindStringSubmatch(expr)
	if matches != nil {
		op := &TimeExpr{}

		// Handle Offset
		if matches[1] != "" {
			var err error
			op.Offset, err = ParseDuration(matches[1])
			if err != nil {
				return nil, err
			}
		}

		// Handle Rounding
		if matches[2] != "" {
			op.RoundUnit = matches[3]
		}

		return op, nil
	}

	// Try parsing as absolute time
	if t, err := time.Parse(time.RFC3339Nano, expr); err == nil {
		return &TimeExpr{Time: t}, nil
	}
	if t, err := time.Parse("2006-01-02", expr); err == nil {
		return &TimeExpr{Time: t}, nil
	}

	return nil, nil
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

// FormatDuration formats a duration into a string with support for d, w, M, y units.
// example:
//
//	FormatDuration(365 * 24 * time.Hour) // "1y"
//	FormatDuration(30 * 24 * time.Hour) // "1M"
//	FormatDuration(7 * 24 * time.Hour) // "1w"
//	FormatDuration(2 * time.Hour + 30 * time.Minute) // "2h30m"
func FormatDuration(inter time.Duration) string {
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

// specific units for day, week, month, year
var customUnitsPattern = regexp.MustCompile(`([+-]?\d+)(d|w|M|y)`)

// ParseDuration parses a duration string with support for d, w, M, y units.
func ParseDuration(interval string) (time.Duration, error) {
	// Replace custom units with hours
	// d -> 24h
	// w -> 7d -> 168h
	// M -> 30d -> 720h
	// y -> 365d -> 8760h
	standardInterval := customUnitsPattern.ReplaceAllStringFunc(interval, func(match string) string {
		// match is like "1d", "-2w"
		// remove unit to get val
		valStr, unit := match[:len(match)-1], match[len(match)-1:]
		val, err := strconv.Atoi(valStr)
		if err != nil {
			// Should not happen given regex
			return match
		}

		var hours int
		switch unit {
		case "d":
			hours = val * 24
		case "w":
			hours = val * 7 * 24
		case "M":
			hours = val * 30 * 24
		case "y":
			hours = val * 365 * 24
		}
		return fmt.Sprintf("%dh", hours)
	})

	return time.ParseDuration(standardInterval)
}
