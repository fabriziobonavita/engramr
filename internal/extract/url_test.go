package extract

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestExtractURL_Redirect(t *testing.T) {
	// Create a test server that redirects
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirectCount == 0 {
			redirectCount++
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusMovedPermanently)
			return
		}
		// Final destination with enough content for readability
		w.Header().Set("Content-Type", "text/html")
		htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Final Page</title>
</head>
<body>
	<nav>Navigation | Links | About</nav>
	<main>
		<article>
			<h1>Final Page Title</h1>
			<p>This is the final content after redirect. This paragraph contains important information that should be extracted by the readability algorithm.</p>
			<p>Here is another paragraph with more detailed information about the topic. This content provides additional context and details that are relevant to understanding the main subject matter.</p>
			<p>Yet another paragraph continues the discussion with more information. This helps ensure that there is enough content to meet the minimum requirements for extraction. The readability algorithm should be able to identify this as the main content.</p>
			<p>This paragraph adds even more content to ensure we have sufficient text for the extraction process. The goal is to provide a comprehensive example that demonstrates the functionality of the URL extraction feature.</p>
			<p>Finally, this last paragraph wraps up the content with concluding thoughts and information. Together, all these paragraphs should provide enough text for readability to successfully extract the main content from the page.</p>
		</article>
	</main>
	<footer>Footer content | Privacy | Terms</footer>
</body>
</html>`
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	// Test redirect following
	urlContent, err := ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	// Check that canonical URL is the final URL (after redirect)
	expectedCanonical := server.URL + "/final"
	if urlContent.CanonicalURL != expectedCanonical {
		t.Errorf("Expected canonical URL %s, got %s", expectedCanonical, urlContent.CanonicalURL)
	}

	// Check that content was extracted
	if !strings.Contains(urlContent.Content, "final content after redirect") {
		t.Errorf("Expected content to contain 'final content after redirect', got: %s", urlContent.Content)
	}
}

func TestExtractURL_ReadabilityExtraction(t *testing.T) {
	// HTML with article content and boilerplate (enough content for readability)
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Test Article</title>
</head>
<body>
	<nav>Navigation Menu | Subscribe | Privacy Policy</nav>
	<main>
		<article>
			<h1>Main Article Title</h1>
			<p>This is the main article content that should be extracted. It contains important information about the topic and provides context for understanding the subject matter.</p>
			<p>This is another paragraph with more detailed information that should also be included in the extraction. It expands on the main topic and provides additional insights and details.</p>
			<p>Here is a third paragraph that continues the discussion with more comprehensive information. This content helps to build a complete picture of the topic being discussed.</p>
			<p>Yet another paragraph adds depth to the article content. This ensures that there is sufficient text for the readability algorithm to successfully identify and extract the main content from the page.</p>
			<p>This paragraph provides even more context and information about the subject. The goal is to create a substantial amount of content that readability can reliably extract and process.</p>
			<p>Finally, this last paragraph concludes the article content with final thoughts and summary information. Together, all these paragraphs should provide enough text for successful extraction.</p>
		</article>
	</main>
	<footer>Footer content | Privacy Policy | Terms of Service | Subscribe to newsletter</footer>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	urlContent, err := ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	// Check that article content is included
	if !strings.Contains(urlContent.Content, "main article content that should be extracted") {
		t.Errorf("Expected content to include article text, got: %s", urlContent.Content)
	}

	if !strings.Contains(urlContent.Content, "another paragraph with more detailed information") {
		t.Errorf("Expected content to include second paragraph, got: %s", urlContent.Content)
	}

	// Check that boilerplate is excluded (or at least minimized)
	// Note: readability may not remove all boilerplate, but should prioritize main content
	contentLower := strings.ToLower(urlContent.Content)
	// The main content should be more prominent than footer text
	if strings.Count(contentLower, "privacy policy") > strings.Count(contentLower, "main article content") {
		t.Errorf("Boilerplate appears to be more prominent than main content")
	}

	// Check title
	if urlContent.Title != "Test Article" {
		t.Errorf("Expected title 'Test Article', got '%s'", urlContent.Title)
	}
}

func TestExtractURL_PlainText(t *testing.T) {
	plainText := `This is a plain text document.
It has multiple lines.
And should be ingested directly without HTML parsing.`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(plainText))
	}))
	defer server.Close()

	urlContent, err := ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	// Check that content matches
	if !strings.Contains(urlContent.Content, "plain text document") {
		t.Errorf("Expected content to contain 'plain text document', got: %s", urlContent.Content)
	}

	if urlContent.ContentType != "text/plain" {
		t.Errorf("Expected content type 'text/plain', got '%s'", urlContent.ContentType)
	}
}

func TestExtractURL_UnsupportedContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key": "value"}`))
	}))
	defer server.Close()

	_, err := ExtractURL(server.URL, "engramr/test")
	if err == nil {
		t.Fatal("Expected error for unsupported content type, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported content-type") {
		t.Errorf("Expected error about unsupported content-type, got: %v", err)
	}
}

func TestExtractURL_InvalidScheme(t *testing.T) {
	_, err := ExtractURL("ftp://example.com/file.txt", "engramr/test")
	if err == nil {
		t.Fatal("Expected error for invalid scheme, got nil")
	}

	if !strings.Contains(err.Error(), "unsupported URL scheme") {
		t.Errorf("Expected error about unsupported URL scheme, got: %v", err)
	}
}

func TestExtractURL_ContentHash(t *testing.T) {
	// HTML with enough content for readability extraction
	htmlContent := `<!DOCTYPE html>
<html>
<head>
	<title>Test Content</title>
</head>
<body>
	<main>
		<article>
			<h1>Test Content for Hashing</h1>
			<p>This is test content that will be used to verify content hashing functionality. The content needs to be substantial enough for readability to extract it successfully.</p>
			<p>Here is another paragraph that adds more content to ensure we meet the minimum requirements. This helps to create a realistic test case that demonstrates the hashing feature.</p>
			<p>This paragraph continues the content with additional information. The goal is to provide enough text so that readability can reliably extract the main content from the HTML document.</p>
			<p>Yet another paragraph adds depth and context to the test content. This ensures that the extraction process works correctly and produces consistent results for hashing purposes.</p>
			<p>Finally, this last paragraph wraps up the test content with concluding information. Together, all these paragraphs should provide sufficient text for successful extraction and hashing.</p>
		</article>
	</main>
</body>
</html>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(htmlContent))
	}))
	defer server.Close()

	urlContent1, err := ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	urlContent2, err := ExtractURL(server.URL, "engramr/test")
	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	// Content hash should be deterministic
	if urlContent1.ContentHash != urlContent2.ContentHash {
		t.Errorf("Content hash should be deterministic, got different hashes: %s vs %s", urlContent1.ContentHash, urlContent2.ContentHash)
	}

	// Content hash should not be empty
	if urlContent1.ContentHash == "" {
		t.Error("Content hash should not be empty")
	}
}

func TestExtractURL_FetchedAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("test"))
	}))
	defer server.Close()

	before := time.Now().UTC()
	urlContent, err := ExtractURL(server.URL, "engramr/test")
	after := time.Now().UTC()

	if err != nil {
		t.Fatalf("ExtractURL failed: %v", err)
	}

	// FetchedAt should be between before and after
	if urlContent.FetchedAt.Before(before) || urlContent.FetchedAt.After(after) {
		t.Errorf("FetchedAt %v should be between %v and %v", urlContent.FetchedAt, before, after)
	}
}
