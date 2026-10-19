package crawler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxPageSize = 5 << 20 // 5 MiB

type Page struct {
	Body        string
	ContentType string
	StatusCode  int
}

type Fetcher interface {
	Fetch(ctx context.Context, url string) (*Page, error)
}

type HTTPFetcher struct {
	client *http.Client
}

func NewHTTPFetcher(client *http.Client) *HTTPFetcher {
	return &HTTPFetcher{client: client}
}

func (f *HTTPFetcher) Fetch(ctx context.Context, url string) (*Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(strings.ToLower(contentType), "text/html") {
		return nil, fmt.Errorf("invalid content type: %s", contentType)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPageSize))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	return &Page{Body: string(body), ContentType: contentType, StatusCode: resp.StatusCode}, nil
}
