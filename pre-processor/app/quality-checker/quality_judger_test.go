// ABOUTME: Comprehensive TDD tests for quality_judger.go
// ABOUTME: Tests LLM-based quality scoring, parsing logic, and retry mechanisms

package qualitychecker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"pre-processor/domain"
	"pre-processor/driver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test constants
const (
	testQualityCheckerURL = "http://quality-checker.test/api/generate"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func withMockTransport(t *testing.T, handler http.Handler) {
	t.Helper()
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if err := req.Context().Err(); err != nil {
			return nil, err
		}
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, req)
		return recorder.Result(), nil
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})
}

// TestParseScore tests the parseScore function with various response formats
func TestParseScore(t *testing.T) {
	tests := map[string]struct {
		input         string
		expectedScore int
		expectedError bool
		description   string
	}{
		"valid_xml_format": {
			input:         "<score>7</score>",
			expectedScore: 7,
			expectedError: false,
			description:   "Should parse valid XML-formatted score",
		},
		"valid_xml_with_whitespace": {
			input:         "  <score>8</score>  ",
			expectedScore: 8,
			expectedError: false,
			description:   "Should parse XML score with surrounding whitespace",
		},
		"valid_xml_with_surrounding_text": {
			input:         "The quality is <score>6</score> out of 10",
			expectedScore: 6,
			expectedError: false,
			description:   "Should extract XML score from surrounding text",
		},
		"valid_xml_without_closing_tag": {
			input:         "<score>9",
			expectedScore: 9,
			expectedError: false,
			description:   "Should parse score without closing tag (Ollama stop sequence truncates it)",
		},
		"tag_with_surrounding_whitespace": {
			input:         "<score> 8 </score>",
			expectedScore: 8,
			expectedError: false,
			description:   "Should parse a score padded with spaces inside the anchored tag",
		},
		"tag_with_newline_before_digits": {
			input:         "<score>\n8",
			expectedScore: 8,
			expectedError: false,
			description:   "Should parse a score on its own line inside the anchored tag",
		},
		"tag_then_prose_starting_with_a_digit": {
			input:         "<score>\n3 つの事実が欠けている。",
			expectedScore: 0,
			expectedError: true,
			description:   "Must not adopt a prose integer that merely follows the opening tag — that is the delete → re-summarize loop this parser exists to prevent",
		},
		"tag_then_prose_after_a_space": {
			input:         "<score> 4 つの問題がある",
			expectedScore: 0,
			expectedError: true,
			description:   "Whitespace tolerance must not extend to digits that begin a sentence",
		},
		"score_at_minimum_boundary": {
			input:         "<score>1</score>",
			expectedScore: 1,
			expectedError: false,
			description:   "Should handle score at minimum boundary (1)",
		},
		"score_at_maximum_boundary": {
			input:         "<score>10</score>",
			expectedScore: 10,
			expectedError: false,
			description:   "Should handle score at maximum boundary (10)",
		},
		"error_score_above_scale": {
			input:         "<score>50</score>",
			expectedScore: 0,
			expectedError: true,
			description:   "Should reject a score above the 1-10 scale instead of clamping it",
		},
		"error_score_below_scale": {
			input:         "<score>0</score>",
			expectedScore: 0,
			expectedError: true,
			description:   "Should reject a score below the 1-10 scale instead of clamping it",
		},
		"error_negative_score": {
			input:         "<score>-5</score>",
			expectedScore: 0,
			expectedError: true,
			description:   "Should reject a negative score instead of reading its absolute value",
		},
		"error_plain_number_without_tag": {
			input:         "The score is 8",
			expectedScore: 0,
			expectedError: true,
			description:   "Should reject a bare integer: only the anchored <score> tag counts",
		},
		"error_integer_in_prose": {
			input:         "この要約には 3 つの事実が欠けている。",
			expectedScore: 0,
			expectedError: true,
			description:   "Should reject an integer that is part of the model's prose",
		},
		"error_no_score_found": {
			input:         "No score available",
			expectedScore: 0,
			expectedError: true,
			description:   "Should error when no score can be extracted",
		},
		"error_empty_string": {
			input:         "",
			expectedScore: 0,
			expectedError: true,
			description:   "Should error on empty string",
		},
		"error_only_whitespace": {
			input:         "   \n\t  ",
			expectedScore: 0,
			expectedError: true,
			description:   "Should error on whitespace-only string",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			score, err := parseScore(tc.input)

			if tc.expectedError {
				require.Error(t, err, tc.description)
			} else {
				require.NoError(t, err, tc.description)
				assert.Equal(t, tc.expectedScore, score.Overall, tc.description)
			}
		})
	}
}

// TestScoreSummary tests the scoreSummary function with mocked HTTP server
func TestScoreSummary(t *testing.T) {
	tests := map[string]struct {
		serverResponse func(w http.ResponseWriter, r *http.Request)
		expectedScore  *int
		expectedError  bool
		description    string
	}{
		"successful_score_response": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "<score>9</score>",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(9),
			expectedError: false,
			description:   "Should successfully parse valid Ollama response",
		},
		"response_with_text_and_score": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "The article quality is <score>8</score> because it's well-written.",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(8),
			expectedError: false,
			description:   "Should extract score from response with surrounding text",
		},
		"response_incomplete": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "<score>8</score>",
					Done:     false, // Not completed
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should error when Ollama response not completed",
		},
		"response_without_score_tag_returns_error": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "Score is 8 points",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should error when the anchored <score> tag is missing, rather than adopting a bare integer",
		},
		"response_unparseable_returns_error": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "Cannot determine quality",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				if err := json.NewEncoder(w).Encode(response); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should return an error (not a fabricated low score) when all parsing strategies fail",
		},
		"http_server_error": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte("Server error"))
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should error on HTTP server error",
		},
		"invalid_json_response": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte("Not valid JSON"))
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should error on invalid JSON response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			withMockTransport(t, http.HandlerFunc(tc.serverResponse))

			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = testQualityCheckerURL
			t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

			// Execute test
			ctx := context.Background()
			prompt := "Test prompt"
			score, err := scoreSummary(ctx, prompt)

			// Verify results
			if tc.expectedError {
				require.Error(t, err, tc.description)
			} else {
				require.NoError(t, err, tc.description)
				if tc.expectedScore != nil {
					require.NotNil(t, score, tc.description)
					assert.Equal(t, *tc.expectedScore, score.Overall, tc.description)
				}
			}
		})
	}
}

// TestScoreSummaryWithRetry tests the retry logic
func TestScoreSummaryWithRetry(t *testing.T) {
	tests := map[string]struct {
		serverBehavior []func(w http.ResponseWriter, r *http.Request)
		maxRetries     int
		expectedScore  *int
		expectedError  bool
		description    string
	}{
		"success_on_first_attempt": {
			serverBehavior: []func(w http.ResponseWriter, r *http.Request){
				func(w http.ResponseWriter, r *http.Request) {
					response := ollamaResponse{Response: "<score>9</score>", Done: true}
					w.WriteHeader(http.StatusOK)
					if err := json.NewEncoder(w).Encode(response); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}
				},
			},
			maxRetries:    3,
			expectedScore: intPtr(9),
			expectedError: false,
			description:   "Should succeed on first attempt",
		},
		"success_on_second_attempt": {
			serverBehavior: []func(w http.ResponseWriter, r *http.Request){
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				func(w http.ResponseWriter, r *http.Request) {
					response := ollamaResponse{Response: "<score>8</score>", Done: true}
					w.WriteHeader(http.StatusOK)
					if err := json.NewEncoder(w).Encode(response); err != nil {
						http.Error(w, err.Error(), http.StatusInternalServerError)
					}
				},
			},
			maxRetries:    3,
			expectedScore: intPtr(8),
			expectedError: false,
			description:   "Should succeed on second attempt after first failure",
		},
		"fail_all_retries": {
			serverBehavior: []func(w http.ResponseWriter, r *http.Request){
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
			},
			maxRetries:    3,
			expectedScore: nil,
			expectedError: true,
			description:   "Should fail after exhausting all retries",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			callCount := 0
			withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if callCount < len(tc.serverBehavior) {
					tc.serverBehavior[callCount](w, r)
					callCount++
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))

			// Override API URL
			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = testQualityCheckerURL
			t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

			// Execute test
			ctx := context.Background()
			score, err := scoreSummaryWithRetry(ctx, "test prompt", tc.maxRetries)

			// Verify results
			if tc.expectedError {
				require.Error(t, err, tc.description)
			} else {
				require.NoError(t, err, tc.description)
				if tc.expectedScore != nil {
					require.NotNil(t, score, tc.description)
					assert.Equal(t, *tc.expectedScore, score.Overall, tc.description)
				}
			}
		})
	}
}

// TestJudgeArticleQuality tests the JudgeArticleQuality function
func TestJudgeArticleQuality(t *testing.T) {
	tests := map[string]struct {
		article       *driver.ArticleWithSummary
		expectedError bool
		description   string
	}{
		"nil_article": {
			article:       nil,
			expectedError: true,
			description:   "Should error on nil article",
		},
		"empty_article_id": {
			article: &driver.ArticleWithSummary{
				ArticleID:       "",
				Content:         "Some content",
				SummaryJapanese: "要約",
			},
			expectedError: true,
			description:   "Should error on empty article ID",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := JudgeArticleQuality(context.Background(), nil, nil, tc.article)
			if tc.expectedError {
				require.Error(t, err, tc.description)
			} else {
				require.NoError(t, err, tc.description)
			}
		})
	}
}

// TestJudgeArticleQualityScoring tests the scoring logic without database
func TestJudgeArticleQualityScoring(t *testing.T) {
	// Test that scoring logic works correctly by mocking HTTP server
	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>9</score>", // High score
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// High score should not attempt database operation, so nil repos are OK
	err := JudgeArticleQuality(context.Background(), nil, nil, article)
	require.NoError(t, err, "High quality score should not require database operation")
}

// TestRemoveLowScoreSummary tests the summary removal logic
func TestRemoveLowScoreSummary(t *testing.T) {
	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article",
		Content:         "Good content",
		SummaryJapanese: "良い要約",
	}

	t.Run("nil score returns error", func(t *testing.T) {
		err := RemoveLowScoreSummary(context.Background(), nil, nil, article, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "received nil score")
	})

	t.Run("nil summaryRepo returns error", func(t *testing.T) {
		score := &Score{Overall: 10} // Low score
		err := RemoveLowScoreSummary(context.Background(), nil, nil, article, score)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "summary repository is nil")
	})
}

// TestJudgeTemplate verifies the prompt template is valid
func TestJudgeTemplate(t *testing.T) {
	assert.NotEmpty(t, JudgeTemplate, "JudgeTemplate should not be empty")
	assert.Contains(t, JudgeTemplate, "%s", "JudgeTemplate should contain placeholders")

	// Verify template can be formatted
	formatted := fmt.Sprintf(JudgeTemplate, "test content", "test summary")
	assert.NotEmpty(t, formatted, "Formatted template should not be empty")
	assert.Contains(t, formatted, "test content", "Formatted template should contain content")
	assert.Contains(t, formatted, "test summary", "Formatted template should contain summary")
}

// TestConstants verifies critical constants
func TestConstants(t *testing.T) {
	assert.Greater(t, lowScoreThreshold, 0, "lowScoreThreshold should be positive")
	assert.LessOrEqual(t, lowScoreThreshold, 30, "lowScoreThreshold should be <= 30")
	assert.NotEmpty(t, modelName, "modelName should not be empty")
	assert.NotEmpty(t, qualityCheckerAPIURL, "qualityCheckerAPIURL should not be empty")
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// TestScoreSummaryContextCancellation tests context cancellation handling
func TestScoreSummaryContextCancellation(t *testing.T) {
	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not be called for canceled context")
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scoreSummary(ctx, "test prompt")
	require.Error(t, err, "Should error when context is cancelled")
}

// TestScoreBoundaryConditions pins the edges of the 1-10 scale JudgeTemplate
// asks for. Values outside it are rejected rather than clamped: a clamped-down
// value deletes a good summary and a clamped-up one passes a bad summary, both
// without any signal that the model broke the output contract.
func TestScoreBoundaryConditions(t *testing.T) {
	tests := []struct {
		input       string
		expected    int
		expectError bool
		desc        string
	}{
		{"<score>1</score>", 1, false, "Min score should remain 1"},
		{"<score>10</score>", 10, false, "Max score should remain 10"},
		{"<score>0</score>", 0, true, "Zero is below the scale and must be rejected"},
		{"<score>11</score>", 0, true, "Above max must be rejected"},
		{"<score>1000</score>", 0, true, "Large value must be rejected"},
		{"<score>-1</score>", 0, true, "Negative value must be rejected, not read as 1"},
		{"<score>-1000</score>", 0, true, "Negative value must be rejected, not read as 1000"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			score, err := parseScore(tc.input)
			if tc.expectError {
				require.Error(t, err, tc.desc)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, score.Overall)
		})
	}
}

// TestIsConnectionError tests the isConnectionError function
func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		expected    bool
		description string
	}{
		{
			name:        "context_deadline_exceeded",
			err:         context.DeadlineExceeded,
			expected:    true,
			description: "Should detect context deadline exceeded as connection error",
		},
		{
			name:        "context_canceled",
			err:         context.Canceled,
			expected:    true,
			description: "Should detect context canceled as connection error",
		},
		{
			name:        "connection_refused",
			err:         errors.New("dial tcp: connection refused"),
			expected:    true,
			description: "Should detect connection refused error",
		},
		{
			name:        "no_such_host",
			err:         errors.New("no such host"),
			expected:    true,
			description: "Should detect DNS error (no such host)",
		},
		{
			name:        "connection_reset",
			err:         errors.New("connection reset by peer"),
			expected:    true,
			description: "Should detect connection reset error",
		},
		{
			name:        "io_timeout",
			err:         errors.New("i/o timeout"),
			expected:    true,
			description: "Should detect I/O timeout error",
		},
		{
			name:        "net_error",
			err:         &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			expected:    true,
			description: "Should detect net.Error as connection error",
		},
		{
			name:        "dns_error",
			err:         &net.DNSError{Err: "no such host", Name: "example.com"},
			expected:    true,
			description: "Should detect DNS error",
		},
		{
			name:        "parsing_error",
			err:         errors.New("failed to parse score"),
			expected:    false,
			description: "Should not detect parsing error as connection error",
		},
		{
			name:        "nil_error",
			err:         nil,
			expected:    false,
			description: "Should return false for nil error",
		},
		{
			name:        "generic_error",
			err:         errors.New("some other error"),
			expected:    false,
			description: "Should not detect generic error as connection error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isConnectionError(tc.err)
			assert.Equal(t, tc.expected, result, tc.description)
		})
	}
}

// TestJudgeArticleQualityConnectionError tests that JudgeArticleQuality does not delete data on connection errors
func TestJudgeArticleQualityConnectionError(t *testing.T) {
	// Test with connection refused error (simulating news-creator being down)
	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp: connection refused")
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-connection-error",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use a short timeout context to trigger connection error faster
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// JudgeArticleQuality should return an error without deleting data
	err := JudgeArticleQuality(ctx, nil, nil, article)
	require.Error(t, err, "Should return error on connection failure")
	assert.Contains(t, err.Error(), "failed to connect to news-creator service", "Error should indicate connection failure")
}

// TestJudgeArticleQualityTimeoutError tests that JudgeArticleQuality handles timeout errors correctly
func TestJudgeArticleQualityTimeoutError(t *testing.T) {
	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		select {
		case <-time.After(2 * time.Second):
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
		recorder := httptest.NewRecorder()
		recorder.WriteHeader(http.StatusOK)
		return recorder.Result(), nil
	})
	t.Cleanup(func() { http.DefaultTransport = originalTransport })

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-timeout",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use a short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// JudgeArticleQuality should return an error without deleting data
	err := JudgeArticleQuality(ctx, nil, nil, article)
	require.Error(t, err, "Should return error on timeout")
	assert.Contains(t, err.Error(), "failed to connect to news-creator service", "Error should indicate connection failure")
}

// TestJudgeArticleQualityContentTooLong tests that quality check is skipped for oversized content
func TestJudgeArticleQualityContentTooLong(t *testing.T) {
	// Create content that exceeds the limit
	longContent := make([]byte, maxQualityCheckContentLength+1)
	for i := range longContent {
		longContent[i] = 'a'
	}

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-long-content",
		Content:         string(longContent),
		SummaryJapanese: "短い要約",
	}

	// Should skip quality check without error (content too long)
	err := JudgeArticleQuality(context.Background(), nil, nil, article)
	require.NoError(t, err, "Should skip quality check for long content without error")
}

// TestJudgeArticleQualityContentWithinLimit tests that quality check proceeds for normal content
func TestJudgeArticleQualityContentWithinLimit(t *testing.T) {
	// Create a server that returns a high score
	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>9</score>",
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	// Content within limit
	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-normal-content",
		Content:         "Normal sized content",
		SummaryJapanese: "通常サイズの要約",
	}

	// Should proceed with quality check
	err := JudgeArticleQuality(context.Background(), nil, nil, article)
	require.NoError(t, err, "Should proceed with quality check for normal content")
}

// TestJudgeArticleQualityContentBoundary tests boundary conditions for content length
func TestJudgeArticleQualityContentBoundary(t *testing.T) {
	tests := []struct {
		name        string
		contentLen  int
		summaryLen  int
		shouldSkip  bool
		description string
	}{
		{
			name:        "exactly_at_limit",
			contentLen:  maxQualityCheckContentLength - 10,
			summaryLen:  10,
			shouldSkip:  false,
			description: "Content exactly at limit should proceed",
		},
		{
			name:        "one_byte_over_limit",
			contentLen:  maxQualityCheckContentLength,
			summaryLen:  1,
			shouldSkip:  true,
			description: "Content one byte over limit should skip",
		},
		{
			name:        "content_alone_exceeds",
			contentLen:  maxQualityCheckContentLength + 1,
			summaryLen:  0,
			shouldSkip:  true,
			description: "Content alone exceeding limit should skip",
		},
		{
			name:        "summary_alone_exceeds",
			contentLen:  0,
			summaryLen:  maxQualityCheckContentLength + 1,
			shouldSkip:  true,
			description: "Summary alone exceeding limit should skip",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shouldSkip {
				// For skip cases, we don't need a mock server
				content := make([]byte, tc.contentLen)
				summary := make([]byte, tc.summaryLen)
				for i := range content {
					content[i] = 'a'
				}
				for i := range summary {
					summary[i] = 'b'
				}

				article := &driver.ArticleWithSummary{
					ArticleID:       "test-article-boundary",
					Content:         string(content),
					SummaryJapanese: string(summary),
				}

				err := JudgeArticleQuality(context.Background(), nil, nil, article)
				require.NoError(t, err, tc.description)
			} else {
				// For proceed cases, we need a mock server
				withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					response := ollamaResponse{
						Response: "<score>9</score>",
						Done:     true,
					}
					w.WriteHeader(http.StatusOK)
					_ = json.NewEncoder(w).Encode(response)
				}))

				originalURL := qualityCheckerAPIURL
				qualityCheckerAPIURL = testQualityCheckerURL
				t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

				content := make([]byte, tc.contentLen)
				summary := make([]byte, tc.summaryLen)
				for i := range content {
					content[i] = 'a'
				}
				for i := range summary {
					summary[i] = 'b'
				}

				article := &driver.ArticleWithSummary{
					ArticleID:       "test-article-boundary",
					Content:         string(content),
					SummaryJapanese: string(summary),
				}

				err := JudgeArticleQuality(context.Background(), nil, nil, article)
				require.NoError(t, err, tc.description)
			}
		})
	}
}

// TestScoreSummaryRequestIncludesRawTrue verifies that the Ollama request includes raw:true
// to prevent double-templating when JudgeTemplate already contains Gemma chat template tokens.
func TestScoreSummaryRequestIncludesRawTrue(t *testing.T) {
	var receivedPayload judgePrompt

	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the request body to inspect the payload
		err := json.NewDecoder(r.Body).Decode(&receivedPayload)
		require.NoError(t, err, "Should decode request body")

		response := ollamaResponse{
			Response: "<score>8</score>",
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	ctx := context.Background()
	_, err := scoreSummary(ctx, "Test prompt")
	require.NoError(t, err)

	assert.True(t, receivedPayload.Raw, "Request payload must include raw:true to prevent Ollama from double-applying chat template")
	assert.Equal(t, modelName, receivedPayload.Model, "Model name should match")
	assert.False(t, receivedPayload.Stream, "Stream should be false")
}

// TestMaxQualityCheckContentLengthConstant tests the constant value
func TestMaxQualityCheckContentLengthConstant(t *testing.T) {
	assert.Equal(t, 20_000, maxQualityCheckContentLength, "maxQualityCheckContentLength should be 20,000")
	assert.Greater(t, maxQualityCheckContentLength, 0, "maxQualityCheckContentLength should be positive")
}

// TestIsPlaceholderSummary tests the isPlaceholderSummary function
func TestIsPlaceholderSummary(t *testing.T) {
	tests := []struct {
		name     string
		summary  string
		expected bool
	}{
		{
			name:     "placeholder_too_short",
			summary:  "本文が短すぎるため要約できませんでした。",
			expected: true,
		},
		{
			name:     "placeholder_too_long",
			summary:  "本文が長すぎるため要約できませんでした。",
			expected: true,
		},
		{
			name:     "normal_summary",
			summary:  "この記事はAIの最新動向について述べている。",
			expected: false,
		},
		{
			name:     "empty_summary",
			summary:  "",
			expected: false,
		},
		{
			name:     "partial_match",
			summary:  "本文が短すぎるため",
			expected: false,
		},
		{
			name:     "placeholder_with_extra_text",
			summary:  "本文が短すぎるため要約できませんでした。追加テキスト",
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := isPlaceholderSummary(tc.summary)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestJudgeArticleQualitySkipsPlaceholder tests that placeholder summaries are skipped
func TestJudgeArticleQualitySkipsPlaceholder(t *testing.T) {
	tests := []struct {
		name    string
		summary string
	}{
		{
			name:    "too_short_placeholder",
			summary: "本文が短すぎるため要約できませんでした。",
		},
		{
			name:    "too_long_placeholder",
			summary: "本文が長すぎるため要約できませんでした。",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			article := &driver.ArticleWithSummary{
				ArticleID:       "test-placeholder-article",
				Content:         "Short",
				SummaryJapanese: tc.summary,
			}

			// No mock server needed - should return nil without making any HTTP calls
			err := JudgeArticleQuality(context.Background(), nil, nil, article)
			require.NoError(t, err, "Placeholder summary should be skipped without error")
		})
	}
}

// TestJudgeArticleQualityLowScoreStillDeletes tests that low scores from successful responses still trigger deletion
func TestJudgeArticleQualityLowScoreStillDeletes(t *testing.T) {
	// Create a server that returns a low score (below threshold)
	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>5</score>", // Low score (below threshold of 7)
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-low-score",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use nil summaryRepo to verify that JudgeArticleQuality attempts to call RemoveLowScoreSummary
	// RemoveLowScoreSummary will return an error because summaryRepo is nil, but this confirms
	// that the low score was detected and deletion was attempted
	ctx := context.Background()
	err := JudgeArticleQuality(ctx, nil, nil, article)
	// Should attempt to delete (will fail because summaryRepo is nil, but that's expected)
	require.Error(t, err, "Should return error when trying to delete with nil summaryRepo")
	// The error should be about summary repository being nil, not connection
	assert.Contains(t, err.Error(), "summary repository is nil", "Error should indicate summary repository is nil")
	assert.NotContains(t, err.Error(), "failed to connect to news-creator service", "Error should not be about connection")
}

// deleteTrackingSummaryRepo is a minimal repository.SummaryRepository stub
// that records whether Delete was invoked, without needing a real database.
type deleteTrackingSummaryRepo struct {
	deleteCalled bool
}

func (r *deleteTrackingSummaryRepo) Create(context.Context, *domain.ArticleSummary) error {
	return nil
}

func (r *deleteTrackingSummaryRepo) FindArticlesWithSummaries(context.Context, *domain.Cursor, int) ([]*domain.ArticleWithSummary, *domain.Cursor, error) {
	return nil, nil, nil
}

func (r *deleteTrackingSummaryRepo) Delete(context.Context, string) error {
	r.deleteCalled = true
	return nil
}

func (r *deleteTrackingSummaryRepo) Exists(context.Context, string) (bool, error) {
	return false, nil
}

// TestJudgeArticleQualityParseFailureDoesNotDelete reproduces the HIGH
// data-loss finding: when scoreSummary exhausts every parsing strategy, it
// previously fabricated Score{Overall: 1} (nil error), which is below
// lowScoreThreshold and caused JudgeArticleQuality to delete a summary purely
// because the model's output format broke — not because it was actually low
// quality. A broken output format must surface as an error and the summary
// must be left untouched.
func TestJudgeArticleQualityParseFailureDoesNotDelete(t *testing.T) {
	withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "I am unable to provide a quality assessment for this content.",
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = testQualityCheckerURL
	t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-unparseable-score",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	repo := &deleteTrackingSummaryRepo{}

	err := JudgeArticleQuality(context.Background(), repo, nil, article)
	require.Error(t, err, "a completely unparseable score response must surface as an error")
	assert.False(t, repo.deleteCalled, "a parse failure must not delete the summary")
}

// TestJudgeArticleQualityMalformedScoreDoesNotDelete pins the data-loss path
// behind the fallback parsers: any integer that is not an in-range
// <score>N</score> used to be adopted as the score. Prose such as
// 「3 つの事実が欠けている」 became Score{Overall: 3}, below lowScoreThreshold,
// so the summary was deleted and the article went back on the summarize queue —
// a delete → re-summarize loop for as long as the model's output format stayed
// broken. A malformed score must surface as an error and leave the summary alone.
func TestJudgeArticleQualityMalformedScoreDoesNotDelete(t *testing.T) {
	tests := map[string]struct {
		modelResponse string
		description   string
	}{
		"prose_with_stray_integer": {
			modelResponse: "この要約には 3 つの事実が欠けている。",
			description:   "An integer inside prose is not a score",
		},
		"negative_anchored_score": {
			modelResponse: "<score>-5</score>",
			description:   "A negative score must not be read as its absolute value",
		},
		"anchored_score_above_scale": {
			modelResponse: "<score>50</score>",
			description:   "A score outside the 1-10 scale must not be silently clamped",
		},
		"anchored_score_below_scale": {
			modelResponse: "<score>0</score>",
			description:   "A score outside the 1-10 scale must not be treated as low quality",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: tc.modelResponse,
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			}))

			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = testQualityCheckerURL
			t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

			article := &driver.ArticleWithSummary{
				ArticleID:       "test-article-malformed-score",
				Content:         "Test content",
				SummaryJapanese: "テスト要約",
			}

			repo := &deleteTrackingSummaryRepo{}

			err := JudgeArticleQuality(context.Background(), repo, nil, article)
			require.Error(t, err, tc.description)
			assert.False(t, repo.deleteCalled, "a malformed score must not delete the summary: %s", tc.description)
		})
	}
}

// TestJudgeArticleQualityAcceptsWhitespaceInScoreTag pins the other side of the
// anchored-parser contract: dropping the fallback parsers also dropped the only
// tolerance for whitespace inside the tag, so a reply such as `<score> 8 </score>`
// failed to parse, burned all three scoreSummaryWithRetry attempts and made
// JudgeArticleQuality error on every pass for that article. Whitespace padding is
// still an anchored score, not prose — it must be accepted.
func TestJudgeArticleQualityAcceptsWhitespaceInScoreTag(t *testing.T) {
	tests := map[string]struct {
		modelResponse string
		description   string
	}{
		"spaces_inside_tag": {
			modelResponse: "<score> 8 </score>",
			description:   "Spaces padding the digits inside the tag",
		},
		"newline_before_digits": {
			modelResponse: "<score>\n8",
			description:   "Digits on the line after the opening tag (closing tag cut by the stop sequence)",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			withMockTransport(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: tc.modelResponse,
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			}))

			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = testQualityCheckerURL
			t.Cleanup(func() { qualityCheckerAPIURL = originalURL })

			article := &driver.ArticleWithSummary{
				ArticleID:       "test-article-whitespace-score",
				Content:         "Test content",
				SummaryJapanese: "テスト要約",
			}

			repo := &deleteTrackingSummaryRepo{}

			err := JudgeArticleQuality(context.Background(), repo, nil, article)
			require.NoError(t, err, "a whitespace-padded score tag must parse: %s", tc.description)
			assert.False(t, repo.deleteCalled, "score 8 is above lowScoreThreshold, the summary must be kept")
		})
	}
}

// --- Gemma 4 chat template token tests ---

func TestJudgeTemplateUsesGemma4TurnTokens(t *testing.T) {
	t.Run("contains Gemma 4 turn tokens", func(t *testing.T) {
		assert.Contains(t, JudgeTemplate, "<|turn>user", "JudgeTemplate must use Gemma 4 <|turn>user token")
		assert.Contains(t, JudgeTemplate, "<|turn>model", "JudgeTemplate must use Gemma 4 <|turn>model token")
		assert.Contains(t, JudgeTemplate, "<turn|>", "JudgeTemplate must use Gemma 4 <turn|> token")
	})

	t.Run("does not contain Gemma 3 turn tokens", func(t *testing.T) {
		assert.NotContains(t, JudgeTemplate, "<start_of_turn>", "JudgeTemplate must not contain Gemma 3 <start_of_turn>")
		assert.NotContains(t, JudgeTemplate, "<end_of_turn>", "JudgeTemplate must not contain Gemma 3 <end_of_turn>")
	})
}
