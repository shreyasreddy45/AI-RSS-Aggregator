package handlers

import (
	"strings"
	"testing"
)

func TestValidateFeedInput(t *testing.T) {
	tests := []struct {
		name      string
		inputName string
		inputURL  string
		wantName  string
		wantURL   string
		wantErr   string
	}{
		{
			name:      "trims valid input",
			inputName: "  BBC News  ",
			inputURL:  "  https://feeds.bbci.co.uk/news/technology/rss.xml  ",
			wantName:  "BBC News",
			wantURL:   "https://feeds.bbci.co.uk/news/technology/rss.xml",
		},
		{
			name:     "requires name",
			inputURL: "https://example.com/rss",
			wantURL:  "https://example.com/rss",
			wantErr:  "name is required",
		},
		{
			name:      "limits name characters",
			inputName: strings.Repeat("a", 100) + "é",
			inputURL:  "https://example.com/rss",
			wantName:  strings.Repeat("a", 100) + "é",
			wantURL:   "https://example.com/rss",
			wantErr:   "name must be at most 100 characters",
		},
		{
			name:      "requires URL",
			inputName: "Example",
			wantName:  "Example",
			wantErr:   "url is required",
		},
		{
			name:      "rejects relative URL",
			inputName: "Example",
			inputURL:  "bbc",
			wantName:  "Example",
			wantURL:   "bbc",
			wantErr:   "url must be a valid http or https address, e.g. https://feeds.bbci.co.uk/news/technology/rss.xml",
		},
		{
			name:      "rejects unsupported scheme",
			inputName: "Example",
			inputURL:  "ftp://example.com/rss",
			wantName:  "Example",
			wantURL:   "ftp://example.com/rss",
			wantErr:   "url must be a valid http or https address, e.g. https://feeds.bbci.co.uk/news/technology/rss.xml",
		},
		{
			name:      "rejects missing host",
			inputName: "Example",
			inputURL:  "https:///rss",
			wantName:  "Example",
			wantURL:   "https:///rss",
			wantErr:   "url must be a valid http or https address, e.g. https://feeds.bbci.co.uk/news/technology/rss.xml",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotURL, err := validateFeedInput(tt.inputName, tt.inputURL)
			if gotName != tt.wantName {
				t.Errorf("validateFeedInput() name = %q, want %q", gotName, tt.wantName)
			}
			if gotURL != tt.wantURL {
				t.Errorf("validateFeedInput() URL = %q, want %q", gotURL, tt.wantURL)
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateFeedInput() error = %v, want nil", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("validateFeedInput() error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}
