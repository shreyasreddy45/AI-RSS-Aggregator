package rss

import (
	"testing"
	"time"
)

func TestParseFeedTime(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
		want time.Time
	}{
		{"GMT zone name (BBC)", "Wed, 04 Nov 2026 09:00:00 GMT", true, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)},
		{"numeric zone", "Wed, 04 Nov 2026 09:00:00 +0000", true, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)},
		{"offset converted to UTC", "Tue, 3 Nov 2026 10:30:00 -0500", true, time.Date(2026, 11, 3, 15, 30, 0, 0, time.UTC)},
		{"RFC3339", "2026-11-04T09:00:00Z", true, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)},
		{"surrounding spaces", "  Wed, 04 Nov 2026 09:00:00 GMT \n", true, time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)},
		{"empty", "", false, time.Time{}},
		{"garbage", "yesterday-ish", false, time.Time{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseFeedTime(tc.in)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && !got.Equal(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
