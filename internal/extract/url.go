package extract

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/mackee/go-readability"
	"golang.org/x/net/html"
)

const (
	minExtractedContentSize = 600
	httpTimeout             = 25 * time.Second
)

// URLContent represents extracted content from a URL.
type URLContent struct {
	Title        string
	Content      string
	CanonicalURL string
	ContentType  string
	FetchedAt    time.Time
	ContentHash  string
}

// ExtractURL fetches and extracts content from a URL.
func ExtractURL(userURL string, userAgent string) (*URLContent, error) {
	// Validate URL scheme
	parsedURL, err := url.Parse(userURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("unsupported URL scheme: %s (only http and https are allowed)", parsedURL.Scheme)
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: httpTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Follow redirects (default behavior)
			return nil
		},
	}

	// Create request
	req, err := http.NewRequest("GET", userURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	// Fetch URL
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch URL: %w", err)
	}
	defer resp.Body.Close()

	// Get final URL after redirects
	canonicalURL := resp.Request.URL.String()
	contentType := resp.Header.Get("Content-Type")

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	fetchedAt := time.Now().UTC()

	// Handle different content types
	var title, markdownContent string
	if strings.Contains(contentType, "text/html") {
		title, markdownContent, err = extractHTML(body, canonicalURL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract HTML content: %w", err)
		}
	} else if strings.Contains(contentType, "text/plain") {
		// For plain text, use as-is (ChunkMarkdown will handle it, just without headings)
		markdownContent = string(body)
		title = extractTitleFromPlainText(markdownContent)
	} else {
		return nil, fmt.Errorf("unsupported content-type for url ingest: %s", contentType)
	}

	// Normalize line endings for consistent hashing
	markdownContent = strings.ReplaceAll(markdownContent, "\r\n", "\n")
	markdownContent = strings.TrimSpace(markdownContent)

	// Compute content hash
	contentHash := sha256Hex([]byte(markdownContent))

	return &URLContent{
		Title:        title,
		Content:      markdownContent, // Now contains markdown, not plain text
		CanonicalURL: canonicalURL,
		ContentType:  contentType,
		FetchedAt:    fetchedAt,
		ContentHash:  contentHash,
	}, nil
}

// extractHTML extracts title and markdown content from HTML using readability.
func extractHTML(body []byte, sourceURL string) (string, string, error) {
	options := readability.DefaultOptions()
	article, err := readability.Extract(string(body), options)
	if err != nil {
		return "", "", fmt.Errorf("readability extraction failed: %w", err)
	}
	if article.Root == nil {
		return "", "", fmt.Errorf("readability extracted no content")
	}

	title := article.Title

	// Convert article.Root directly to markdown
	markdown := readability.ToMarkdown(article.Root)
	markdown = strings.TrimSpace(markdown)

	// Check minimum content size
	if len(markdown) < minExtractedContentSize {
		return "", "", fmt.Errorf("readability extracted too little content (%d chars, minimum %d)", len(markdown), minExtractedContentSize)
	}

	return title, markdown, nil
}

// extractTitleFromHTML extracts the title from HTML.
func extractTitleFromHTML(body []byte) string {
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return ""
	}

	var findTitle func(*html.Node) string
	findTitle = func(n *html.Node) string {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil {
				return strings.TrimSpace(n.FirstChild.Data)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if title := findTitle(c); title != "" {
				return title
			}
		}
		return ""
	}

	return findTitle(doc)
}

// extractTitleFromPlainText extracts a title from plain text (first line or first sentence).
func extractTitleFromPlainText(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 && len(line) < 200 {
			return line
		}
	}
	// Fallback: first 100 chars
	if len(text) > 100 {
		return text[:100] + "..."
	}
	return text
}

// sha256Hex computes the SHA256 hash and returns it as a hex string.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
