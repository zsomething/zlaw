package cron

import (
	"testing"
	"time"
)

func TestMatchesCron(t *testing.T) {
	// fixed reference time: Tuesday 2026-04-07 08:00 UTC
	ref := time.Date(2026, 4, 7, 8, 0, 0, 0, time.UTC)

	cases := []struct {
		schedule string
		at       time.Time
		want     bool
		wantErr  bool
	}{
		// exact match
		{"0 8 * * *", ref, true, false},
		// wrong minute
		{"1 8 * * *", ref, false, false},
		// wrong hour
		{"0 9 * * *", ref, false, false},
		// wildcard everything
		{"* * * * *", ref, true, false},
		// range match (hour 7-9)
		{"0 7-9 * * *", ref, true, false},
		// step: every 2 hours, starting from 0 → 0,2,4,6,8 matches
		{"0 */2 * * *", ref, true, false},
		// step: every 2 hours from 1 → 1,3,5,7 — 8 does not match
		{"0 1-7/2 * * *", ref, false, false},
		// day-of-week: Tuesday = 2
		{"0 8 * * 2", ref, true, false},
		// day-of-week: Wednesday = 3
		{"0 8 * * 3", ref, false, false},
		// month
		{"0 8 * 4 *", ref, true, false},
		{"0 8 * 5 *", ref, false, false},
		// comma list
		{"0 6,8,10 * * *", ref, true, false},
		{"0 7,9,11 * * *", ref, false, false},
		// wrong field count
		{"0 8 * *", time.Time{}, false, true},
		// invalid step
		{"0 */0 * * *", time.Time{}, false, true},
		// invalid value
		{"0 foo * * *", time.Time{}, false, true},
	}

	for _, tc := range cases {
		got, err := matchesCron(tc.schedule, tc.at)
		if (err != nil) != tc.wantErr {
			t.Errorf("matchesCron(%q) error = %v, wantErr %v", tc.schedule, err, tc.wantErr)
			continue
		}
		if !tc.wantErr && got != tc.want {
			t.Errorf("matchesCron(%q) = %v, want %v", tc.schedule, got, tc.want)
		}
	}
}
