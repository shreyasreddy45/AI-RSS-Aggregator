package rss

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"
)

type RSSFeed struct {
	Channel struct {
		Title       string    `xml:"title"`
		Link        string    `xml:"link"`
		Description string    `xml:"description"`
		Language    string    `xml:"language"`
		Item        []RSSItem `xml:"item"`
	} `xml:"channel"`
}

type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

func fetchWithRetry(client *http.Client, url string) (*http.Response, error) {

	backoff := 2 * time.Second

	for attempt := 1; attempt <= 3; attempt++ {

		resp, err := client.Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}

		if resp != nil {
			resp.Body.Close()
		}

		if attempt == 3 {
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("request failed with status %d", resp.StatusCode)
		}

		time.Sleep(backoff)
		backoff *= 2
	}

	return nil, fmt.Errorf("failed to fetch feed")
}

// UrlToFeed fetches and parses the RSS feed at the given URL.
func UrlToFeed(url string) (RSSFeed, error) {

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	resp, err := fetchWithRetry(httpClient, url)
	if err != nil {
		return RSSFeed{}, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return RSSFeed{}, err
	}

	var rssFeed RSSFeed

	err = xml.Unmarshal(data, &rssFeed)
	if err != nil {
		return RSSFeed{}, err
	}

	return rssFeed, nil
}
