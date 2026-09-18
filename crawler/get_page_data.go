package crawler

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

func GetPageData(url string) (string, string, int, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to fetch URL: %w", err)
	}

	defer resp.Body.Close()
	if resp.StatusCode > 399 {
		return "", "", resp.StatusCode, fmt.Errorf("HTTP error: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}

	contentType := resp.Header.Get("content-type")
	if !strings.HasPrefix(contentType, "text/html") {
		return "", contentType, resp.StatusCode, fmt.Errorf("Invalid content type: %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", contentType, resp.StatusCode, fmt.Errorf("Failed to read response body: %w", err)
	}
	return string(body), contentType, resp.StatusCode, nil

}
