package nodes

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"time"

	"orchkit"
)

// RSS fetches and parses an RSS or Atom feed.
// Returns items as a list of maps with title, link, description, pubDate.
//
// Example:
//
//	nodes.NewRSS("https://hnrss.org/frontpage")
type RSS struct {
	URL   string
	Limit int // max items to return, 0 = all
}

func NewRSS(url string) *RSS {
	return &RSS{URL: url}
}

func (r *RSS) WithLimit(n int) *RSS {
	r.Limit = n
	return r
}

func (r *RSS) Name() string { return "rss" }

func (r *RSS) Schema() orchkit.Schema {
	return orchkit.Schema{
		Description: "Fetches and parses an RSS or Atom feed. Returns items with title, link, description, and date.",
		Params: map[string]any{
			"url":   map[string]any{"type": "string", "desc": "RSS or Atom feed URL."},
			"limit": map[string]any{"type": "integer", "desc": "Max items to return. 0 = all."},
		},
	}
}

func (r *RSS) Execute(ctx context.Context, in orchkit.Input) (orchkit.Output, error) {
	url := r.URL
	if v, ok := in["url"].(string); ok && v != "" {
		url = v
	}
	if url == "" {
		return nil, fmt.Errorf("rss: url is required")
	}

	limit := r.Limit
	if v, ok := in["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("rss: %w", err)
	}
	req.Header.Set("user-agent", "orchkit-rss/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rss: fetch: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("rss: read: %w", err)
	}

	// Try RSS first, then Atom.
	var feed rssFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("rss: parse: %w", err)
	}

	items := make([]any, 0, len(feed.Channel.Items))
	for i, item := range feed.Channel.Items {
		if limit > 0 && i >= limit {
			break
		}
		items = append(items, map[string]any{
			"title":       item.Title,
			"link":        item.Link,
			"description": item.Description,
			"pub_date":    item.PubDate,
			"guid":        item.GUID,
		})
	}

	return orchkit.Output{
		"items":       items,
		"count":       len(items),
		"feed_title":  feed.Channel.Title,
		"feed_link":   feed.Channel.Link,
	}, nil
}

type rssFeed struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Items       []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}
