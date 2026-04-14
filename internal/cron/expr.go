package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// matchesCron reports whether the 5-field cron schedule matches time t.
// Field order: minute hour dom month dow.
// Supported syntax per field: *, N, N-M, */N, N-M/N, and comma-separated
// combinations of the above.
func matchesCron(schedule string, t time.Time) (bool, error) {
	fields := strings.Fields(schedule)
	if len(fields) != 5 {
		return false, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), schedule)
	}
	checks := []struct {
		field    string
		val      int
		min, max int
	}{
		{fields[0], t.Minute(), 0, 59},
		{fields[1], t.Hour(), 0, 23},
		{fields[2], t.Day(), 1, 31},
		{fields[3], int(t.Month()), 1, 12},
		{fields[4], int(t.Weekday()), 0, 6},
	}
	for _, c := range checks {
		ok, err := matchField(c.field, c.val, c.min, c.max)
		if err != nil {
			return false, fmt.Errorf("cron field %q: %w", c.field, err)
		}
		if !ok {
			return false, nil
		}
	}
	return true, nil
}

// matchField checks if val is matched by a single cron field expression,
// which may be a comma-separated list of parts.
func matchField(expr string, val, lo, hi int) (bool, error) {
	for _, part := range strings.Split(expr, ",") {
		ok, err := matchPart(part, val, lo, hi)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

// matchPart checks a single (non-comma) cron field part.
// Supports: *, N, N-M, */N, N-M/N.
func matchPart(part string, val, lo, hi int) (bool, error) {
	step := 1
	if idx := strings.Index(part, "/"); idx != -1 {
		s, err := strconv.Atoi(part[idx+1:])
		if err != nil || s < 1 {
			return false, fmt.Errorf("invalid step in %q", part)
		}
		step = s
		part = part[:idx]
	}

	var low, high int
	switch {
	case part == "*":
		low, high = lo, hi
	case strings.Contains(part, "-"):
		idx := strings.Index(part, "-")
		a, err1 := strconv.Atoi(part[:idx])
		b, err2 := strconv.Atoi(part[idx+1:])
		if err1 != nil || err2 != nil {
			return false, fmt.Errorf("invalid range in %q", part)
		}
		low, high = a, b
	default:
		n, err := strconv.Atoi(part)
		if err != nil {
			return false, fmt.Errorf("invalid value in %q", part)
		}
		low, high = n, n
	}

	for v := low; v <= high; v += step {
		if v == val {
			return true, nil
		}
	}
	return false, nil
}
