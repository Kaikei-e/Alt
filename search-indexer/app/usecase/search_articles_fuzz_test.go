package usecase

import (
	"context"
	"testing"
)

func FuzzSearchArticlesValidation(f *testing.F) {
	// Seed corpus with known attack vectors
	f.Add("<script>alert('xss')</script>")
	f.Add("'; DROP TABLE articles; --")
	f.Add("test' UNION SELECT * FROM users--")
	f.Add("test | rm -rf /")
	f.Add("test; cat /etc/passwd")
	f.Add("test`whoami`")
	f.Add("test$(whoami)")
	f.Add("test\x00")
	f.Add("test\r\n")
	f.Add("%3Cscript%3Ealert%28%27xss%27%29%3C%2Fscript%3E")
	f.Add("test\u200B\u200C\u200D")
	f.Add("javascript:alert('xss')")
	f.Add("<iframe src=javascript:alert('xss')></iframe>")
	f.Add("<svg onload=alert('xss')>")
	f.Add("test/* comment */")
	f.Add("normal search query")
	f.Add("プログラミング")
	f.Add("test-driven development")
	f.Add("golang 1.24")

	searchEngine := &mockSearchEngine{}
	usecase := NewSearchArticlesUsecase(searchEngine)

	f.Fuzz(func(t *testing.T, query string) {
		// The usecase must never panic, regardless of input. Policy: allow
		// arbitrary search text but reject structural hazards (control chars,
		// zero-width, over-length). Denylists for SQL/XSS/cmd were removed in
		// H-002 because Meilisearch is not a vulnerable sink for those
		// payloads and regex denylists produced false positives for legit
		// queries.
		_, err := usecase.Execute(context.Background(), query, 10)

		if query == "" && err == nil {
			t.Error("empty query should return error")
		}
		if len(query) > 1000 && err == nil {
			t.Error("very long query should return error")
		}
	})
}
