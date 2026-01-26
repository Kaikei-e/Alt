package html_parser

import (
	"strings"
	"testing"
)

// Helper to create content that exceeds MinArticleLength (100 chars)
const longContent = "This is a long paragraph of content that exceeds the minimum article length requirement of 100 characters. It contains enough text to be considered valid article content by the extraction logic."

func TestExtractArticleHTML_PreservesHeaders(t *testing.T) {
	raw := `<html><body><article>
		<h1>Main Title</h1>
		<h2>Section Header</h2>
		<h3>Subsection</h3>
		<p>` + longContent + `</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	// go-readability may convert h1 to h2, so check for header tags existence
	if !strings.Contains(got, "<h") {
		t.Errorf("expected header tags to be preserved, got %q", got)
	}
	if !strings.Contains(got, "Main Title") {
		t.Errorf("expected header content 'Main Title' to be preserved, got %q", got)
	}
	if !strings.Contains(got, "Section Header") {
		t.Errorf("expected header content 'Section Header' to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesLists(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<ul>
			<li>First item in the unordered list</li>
			<li>Second item in the unordered list</li>
		</ul>
		<ol>
			<li>First numbered item</li>
			<li>Second numbered item</li>
		</ol>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<ul>") {
		t.Errorf("expected ul tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<ol>") {
		t.Errorf("expected ol tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<li>") {
		t.Errorf("expected li tags to be preserved, got %q", got)
	}
	if !strings.Contains(got, "First item") {
		t.Errorf("expected list content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesCodeBlocks(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>Here is some example code that demonstrates the feature:</p>
		<pre><code>func main() {
    fmt.Println("Hello, World!")
}</code></pre>
		<p>And this is inline <code>code</code> that also works.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<pre>") {
		t.Errorf("expected pre tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<code>") {
		t.Errorf("expected code tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "fmt.Println") {
		t.Errorf("expected code content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesLinksWithHref(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>For more information, check out <a href="https://example.com">this external link</a> for details.</p>
		<p>Also see <a href="/relative/path">this relative link</a> here in the documentation.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<a") {
		t.Errorf("expected a tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, `href="https://example.com"`) {
		t.Errorf("expected href attribute to be preserved, got %q", got)
	}
	if !strings.Contains(got, "this external link") {
		t.Errorf("expected link text to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesImgTags(t *testing.T) {
	// img tags are intentionally removed as they are:
	// 1. Not fetched/displayed by Alt anyway
	// 2. A major XSS vector (onerror, onload events)
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>Here is an illustrative image showing the concept:</p>
		<img src="https://example.com/image.jpg" alt="Example image showing concept" onerror="alert('XSS')"/>
		<p>The image above demonstrates the key concept discussed in this article.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<img") {
		t.Errorf("expected img tag to be removed, got %q", got)
	}
	// Content around the image should still be preserved
	if !strings.Contains(got, "illustrative image") {
		t.Errorf("expected surrounding content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesBlockquotes(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>As the famous author once said:</p>
		<blockquote>This is a quoted text that should be preserved in the output for proper attribution and citation.</blockquote>
		<p>This quote perfectly summarizes the main point of the article.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<blockquote>") {
		t.Errorf("expected blockquote tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "quoted text") {
		t.Errorf("expected blockquote content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesTables(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>The following table shows the comparison data:</p>
		<table>
			<thead><tr><th>Feature Name</th><th>Description</th></tr></thead>
			<tbody><tr><td>Feature One</td><td>Description of feature one</td></tr></tbody>
		</table>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<table>") {
		t.Errorf("expected table tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<th>") {
		t.Errorf("expected th tags to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<td>") {
		t.Errorf("expected td tags to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesScriptTags(t *testing.T) {
	raw := `<html><body>
		<script>alert('XSS attack attempt')</script>
		<article>
			<p>` + longContent + `</p>
			<p>This is the safe content that should be preserved in the output.</p>
		</article>
		<script type="text/javascript">maliciousFunction()</script>
	</body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<script") {
		t.Errorf("expected script tags to be removed, got %q", got)
	}
	if strings.Contains(got, "alert") || strings.Contains(got, "malicious") {
		t.Errorf("expected script content to be removed, got %q", got)
	}
	if !strings.Contains(got, "safe content") {
		t.Errorf("expected article content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesOnEventHandlers(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p onclick="alert('click')">Paragraph with event handler that should be removed.</p>
		<a href="https://example.com" onmouseover="alert('hover')">Safe link text</a>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	// Check that event handler attributes are removed (not present as attributes)
	if strings.Contains(got, `onclick="`) {
		t.Errorf("expected onclick attribute to be removed, got %q", got)
	}
	if strings.Contains(got, `onmouseover="`) {
		t.Errorf("expected onmouseover attribute to be removed, got %q", got)
	}
	// Content should still be preserved
	if !strings.Contains(got, "Paragraph with") {
		t.Errorf("expected paragraph content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesStyleTags(t *testing.T) {
	raw := `<html><body>
		<style>.malicious { display: none; }</style>
		<article>
			<p>` + longContent + `</p>
			<p>This is the safe content here.</p>
		</article>
	</body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<style") {
		t.Errorf("expected style tags to be removed, got %q", got)
	}
	if strings.Contains(got, ".malicious") {
		t.Errorf("expected style content to be removed, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesIframeTags(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>Content before the iframe element.</p>
		<iframe src="https://malicious.com/embed"></iframe>
		<p>Content after the iframe element.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<iframe") {
		t.Errorf("expected iframe tags to be removed, got %q", got)
	}
	if strings.Contains(got, "malicious.com") {
		t.Errorf("expected iframe src to be removed, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesJavascriptURLs(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<a href="javascript:alert('XSS')">Click me for XSS</a>
		<a href="https://safe.com/page">Safe link to external site</a>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "javascript:") {
		t.Errorf("expected javascript: URL to be removed, got %q", got)
	}
	// Safe link should be preserved
	if !strings.Contains(got, "https://safe.com") {
		t.Errorf("expected safe href to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesAllImageTags(t *testing.T) {
	// All img tags should be removed - both with data: URLs and regular URLs
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<img src="data:image/svg+xml,<svg onload='alert(1)'>" alt="XSS attempt"/>
		<img src="https://example.com/safe.jpg" alt="Safe image"/>
		<p>Some more text content to ensure proper length.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	// All img tags should be removed for security
	if strings.Contains(got, "<img") {
		t.Errorf("expected all img tags to be removed, got %q", got)
	}
	// But text content should remain
	if !strings.Contains(got, "Some more text") {
		t.Errorf("expected text content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesTextFormatting(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>This text has <strong>bold text</strong> and <em>italic text</em> formatting.</p>
		<p>Also <b>b tag bold</b> and <i>i tag italic</i> should work.</p>
		<p>And <u>underlined text</u> should be preserved too.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "<strong>") || !strings.Contains(got, "bold text") {
		t.Errorf("expected strong tag to be preserved, got %q", got)
	}
	if !strings.Contains(got, "<em>") || !strings.Contains(got, "italic text") {
		t.Errorf("expected em tag to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_HandlesEmptyInput(t *testing.T) {
	got := ExtractArticleHTML("")
	if got != "" {
		t.Errorf("expected empty string for empty input, got %q", got)
	}
}

func TestExtractArticleHTML_HandlesPlainText(t *testing.T) {
	raw := longContent + " Additional text without any HTML tags at all to test plain text handling."

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "plain text") {
		t.Errorf("expected plain text to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_RemovesNavHeaderFooter(t *testing.T) {
	raw := `<html><body>
		<nav><a href="/">Home</a><a href="/about">About</a></nav>
		<header><h1>Site Header</h1></header>
		<article>
			<p>` + longContent + `</p>
			<p>Main article content that should be extracted and preserved.</p>
		</article>
		<footer><p>Copyright 2024</p></footer>
	</body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<nav>") {
		t.Errorf("expected nav to be removed, got %q", got)
	}
	// Note: bluemonday may strip header/footer tags but content could remain
	// The key is that main article content is preserved
	if !strings.Contains(got, "Main article content") {
		t.Errorf("expected article content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_PreservesStructuralDivs(t *testing.T) {
	raw := `<html><body><article>
		<div class="content">
			<p>` + longContent + `</p>
			<p>Content inside a div should be preserved properly.</p>
			<section>
				<p>Content inside section element too should be preserved.</p>
			</section>
		</div>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if !strings.Contains(got, "Content inside a div") {
		t.Errorf("expected div content to be preserved, got %q", got)
	}
	if !strings.Contains(got, "Content inside section") {
		t.Errorf("expected section content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_SanitizesComplexXSSVectors(t *testing.T) {
	// Various XSS vectors that should be sanitized
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>Safe content that should definitely be preserved.</p>
		<svg onload="alert('XSS')"><circle r="50"/></svg>
		<math><mi>x</mi></math>
		<object data="data:text/html,<script>alert(1)</script>"></object>
		<embed src="data:text/html,<script>alert(1)</script>"/>
		<form action="https://evil.com"><input type="submit"/></form>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	if strings.Contains(got, "<svg") {
		t.Errorf("expected svg to be removed, got %q", got)
	}
	if strings.Contains(got, "<object") {
		t.Errorf("expected object to be removed, got %q", got)
	}
	if strings.Contains(got, "<embed") {
		t.Errorf("expected embed to be removed, got %q", got)
	}
	if strings.Contains(got, "<form") {
		t.Errorf("expected form to be removed, got %q", got)
	}
	if !strings.Contains(got, "Safe content") {
		t.Errorf("expected safe content to be preserved, got %q", got)
	}
}

func TestExtractArticleHTML_ReturnsHTMLNotPlainText(t *testing.T) {
	raw := `<html><body><article>
		<p>` + longContent + `</p>
		<p>Second paragraph with <strong>bold</strong> formatting.</p>
	</article></body></html>`

	got := ExtractArticleHTML(raw)

	// Should return HTML, not plain text
	if !strings.Contains(got, "<p>") {
		t.Errorf("expected HTML with p tags, got %q", got)
	}
}
