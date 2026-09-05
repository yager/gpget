package xfer

import "testing"

func TestSpaceOK(t *testing.T) {
	cases := []struct {
		free, need int64
		ok         bool
	}{
		{-1, 1 << 30, true}, // unknown: do not block
		{0, 0, true},        // nothing to write
		{0, 1, false},       // full disk
		{100, 100, true},
		{100, 101, false},
		{1 << 30, 401 << 20, true},
	}
	for _, tc := range cases {
		got := spaceOK(tc.free, tc.need)
		if got != tc.ok {
			t.Errorf("spaceOK(%d, %d) = %v, want %v", tc.free, tc.need, got, tc.ok)
		}
	}
}
