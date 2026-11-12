package rss

import (
	"strings"
	"time"
)

// feedTimeLayouts lists the date formats seen in real-world RSS/Atom feeds.
// RFC 822 / RFC 1123 dates come with either a numeric zone ("+0000") or a zone
// name ("GMT"); BBC, for example, uses "GMT", which time.RFC1123Z rejects.
var feedTimeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	time.RFC3339Nano,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04 -0700",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// ParseFeedTime parses a feed item's publication date. The second return value
// is false when the text matches none of the known layouts.
func ParseFeedTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range feedTimeLayouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
