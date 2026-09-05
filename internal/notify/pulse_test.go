package notify

import (
	"testing"
	"time"
)

func TestShouldPulse(t *testing.T) {
	cases := []struct {
		name               string
		done, total, lastN int
		since              time.Duration
		want               bool
	}{
		{"start is the caller's job", 0, 106, 0, 0, false},
		{"last file is the caller's job", 106, 106, 100, time.Minute, false},
		{"under both thresholds", 5, 106, 0, 3 * time.Second, false},
		{"every 10 files", 10, 106, 0, time.Second, true},
		{"every 10 from a later mark", 23, 106, 13, time.Second, true},
		{"15 seconds elapsed", 3, 106, 0, 15 * time.Second, true},
		{"just under 15s", 3, 106, 0, 14 * time.Second, false},
		{"empty total", 1, 0, 0, time.Minute, false},
		{"negative done", -1, 10, 0, time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ShouldPulse(tc.done, tc.total, tc.lastN, tc.since)
			if got != tc.want {
				t.Fatalf("ShouldPulse(%d,%d,%d,%s) = %v, want %v",
					tc.done, tc.total, tc.lastN, tc.since, got, tc.want)
			}
		})
	}
}
