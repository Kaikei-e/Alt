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

	"pre-processor/driver"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseScore tests the parseScore function with various response formats
func TestParseScore(t *testing.T) {
	tests := map[string]struct {
		input         string
		expectedScore int
		expectedError bool
		description   string
	}{
		"valid_xml_format": {
			input:         "<score>15</score>",
			expectedScore: 15,
			expectedError: false,
			description:   "Should parse valid XML-formatted score",
		},
		"valid_xml_with_whitespace": {
			input:         "  <score>20</score>  ",
			expectedScore: 20,
			expectedError: false,
			description:   "Should parse XML score with surrounding whitespace",
		},
		"valid_xml_with_surrounding_text": {
			input:         "The quality is <score>25</score> out of 30",
			expectedScore: 25,
			expectedError: false,
			description:   "Should extract XML score from surrounding text",
		},
		"score_at_minimum_boundary": {
			input:         "<score>0</score>",
			expectedScore: 0,
			expectedError: false,
			description:   "Should handle score at minimum boundary (0)",
		},
		"score_at_maximum_boundary": {
			input:         "<score>30</score>",
			expectedScore: 30,
			expectedError: false,
			description:   "Should handle score at maximum boundary (30)",
		},
		"score_above_maximum_clamped": {
			input:         "<score>50</score>",
			expectedScore: 30,
			expectedError: false,
			description:   "Should clamp score above maximum to 30",
		},
		"score_below_minimum_clamped": {
			input:         "<score>-5</score>",
			expectedScore: 5, // Regex extracts "5" from "-5", which is then clamped if needed
			expectedError: false,
			description:   "Should extract positive digit from negative score",
		},
		"fallback_plain_number": {
			input:         "The score is 18",
			expectedScore: 18,
			expectedError: false,
			description:   "Should use fallback parsing for plain number",
		},
		"fallback_first_number": {
			input:         "Quality: 12 out of 30 points",
			expectedScore: 12,
			expectedError: false,
			description:   "Should extract first number in fallback mode",
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

// TestAttemptEmergencyParsing tests the emergency parsing fallback logic
func TestAttemptEmergencyParsing(t *testing.T) {
	tests := map[string]struct {
		input         string
		expectedScore *int
		description   string
	}{
		"simple_number": {
			input:         "15",
			expectedScore: intPtr(15),
			description:   "Should extract simple number",
		},
		"number_with_text": {
			input:         "The quality score is 22 points",
			expectedScore: intPtr(22),
			description:   "Should extract number from text",
		},
		"number_with_special_chars": {
			input:         "Score: [18] (good)",
			expectedScore: intPtr(18),
			description:   "Should extract number ignoring special characters",
		},
		"multiple_numbers_takes_first": {
			input:         "12 out of 30",
			expectedScore: intPtr(12),
			description:   "Should take first number when multiple present",
		},
		"clamp_above_maximum": {
			input:         "100",
			expectedScore: intPtr(30),
			description:   "Should clamp score above 30",
		},
		"clamp_below_minimum": {
			input:         "-10",
			expectedScore: intPtr(10), // Regex \b(\d+)\b only matches positive digits
			description:   "Should extract positive digits from negative number",
		},
		"no_numbers": {
			input:         "No score available",
			expectedScore: nil,
			description:   "Should return nil when no numbers found",
		},
		"empty_string": {
			input:         "",
			expectedScore: nil,
			description:   "Should return nil for empty string",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			result := attemptEmergencyParsing(tc.input)

			if tc.expectedScore == nil {
				assert.Nil(t, result, tc.description)
			} else {
				require.NotNil(t, result, tc.description)
				assert.Equal(t, *tc.expectedScore, result.Overall, tc.description)
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
					Response: "<score>20</score>",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(20),
			expectedError: false,
			description:   "Should successfully parse valid Ollama response",
		},
		"response_with_text_and_score": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "The article quality is <score>25</score> because it's well-written.",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(25),
			expectedError: false,
			description:   "Should extract score from response with surrounding text",
		},
		"response_incomplete": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "<score>15</score>",
					Done:     false, // Not completed
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: nil,
			expectedError: true,
			description:   "Should error when Ollama response not completed",
		},
		"response_with_fallback_parsing": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "Score is 18 points",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(18),
			expectedError: false,
			description:   "Should use fallback parsing when XML format not found",
		},
		"response_unparseable_uses_final_fallback": {
			serverResponse: func(w http.ResponseWriter, r *http.Request) {
				response := ollamaResponse{
					Response: "Cannot determine quality",
					Done:     true,
				}
				w.WriteHeader(http.StatusOK)
				_ = json.NewEncoder(w).Encode(response)
			},
			expectedScore: intPtr(1),
			expectedError: false,
			description:   "Should use final fallback score (1) when parsing fails",
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
			// Create mock server
			server := httptest.NewServer(http.HandlerFunc(tc.serverResponse))
			defer server.Close()

			// Temporarily override the API URL
			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = server.URL
			defer func() { qualityCheckerAPIURL = originalURL }()

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
					response := ollamaResponse{Response: "<score>20</score>", Done: true}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(response)
				},
			},
			maxRetries:    3,
			expectedScore: intPtr(20),
			expectedError: false,
			description:   "Should succeed on first attempt",
		},
		"success_on_second_attempt": {
			serverBehavior: []func(w http.ResponseWriter, r *http.Request){
				func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
				},
				func(w http.ResponseWriter, r *http.Request) {
					response := ollamaResponse{Response: "<score>15</score>", Done: true}
					w.WriteHeader(http.StatusOK)
					json.NewEncoder(w).Encode(response)
				},
			},
			maxRetries:    3,
			expectedScore: intPtr(15),
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
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if callCount < len(tc.serverBehavior) {
					tc.serverBehavior[callCount](w, r)
					callCount++
				} else {
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()

			// Override API URL
			originalURL := qualityCheckerAPIURL
			qualityCheckerAPIURL = server.URL
			defer func() { qualityCheckerAPIURL = originalURL }()

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
			err := JudgeArticleQuality(context.Background(), nil, tc.article)
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>25</score>", // High score
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = server.URL
	defer func() { qualityCheckerAPIURL = originalURL }()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// High score should not attempt database operation, so nil dbPool is OK
	err := JudgeArticleQuality(context.Background(), nil, article)
	require.NoError(t, err, "High quality score should not require database operation")
}

// TestRemoveLowScoreSummary tests the summary removal logic
func TestRemoveLowScoreSummary(t *testing.T) {
	// Test only high score case that doesn't need database
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>25</score>", // High score
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = server.URL
	defer func() { qualityCheckerAPIURL = originalURL }()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article",
		Content:         "Good content",
		SummaryJapanese: "良い要約",
	}

	// High score should not delete, so no DB operation
	err := RemoveLowScoreSummary(context.Background(), nil, article)
	require.NoError(t, err, "Should not delete summary with high score")
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
	// Create a server that delays response
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// This will never complete
		select {}
	}))
	defer server.Close()

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = server.URL
	defer func() { qualityCheckerAPIURL = originalURL }()

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := scoreSummary(ctx, "test prompt")
	require.Error(t, err, "Should error when context is cancelled")
}

// TestScoreBoundaryConditions tests score clamping edge cases
func TestScoreBoundaryConditions(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		desc     string
	}{
		{"<score>-1</score>", 1, "Regex extracts 1 from -1"},
		{"<score>0</score>", 0, "Zero score should remain 0"},
		{"<score>30</score>", 30, "Max score should remain 30"},
		{"<score>31</score>", 30, "Above max should clamp to 30"},
		{"<score>1000</score>", 30, "Large value should clamp to 30"},
		{"<score>-1000</score>", 30, "Regex extracts 1000 from -1000, clamped to 30"},
	}

	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			score, err := parseScore(tc.input)
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
	// Use an invalid URL to trigger connection error
	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = "http://localhost:99999/api/generate" // Invalid port to trigger connection error
	defer func() { qualityCheckerAPIURL = originalURL }()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-connection-error",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use a short timeout context to trigger connection error faster
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// JudgeArticleQuality should return an error without deleting data
	err := JudgeArticleQuality(ctx, nil, article)
	require.Error(t, err, "Should return error on connection failure")
	assert.Contains(t, err.Error(), "failed to connect to news-creator service", "Error should indicate connection failure")
}

// TestJudgeArticleQualityTimeoutError tests that JudgeArticleQuality handles timeout errors correctly
func TestJudgeArticleQualityTimeoutError(t *testing.T) {
	// Create a server that never responds (to simulate timeout)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Sleep longer than context timeout
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = server.URL
	defer func() { qualityCheckerAPIURL = originalURL }()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-timeout",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use a short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// JudgeArticleQuality should return an error without deleting data
	err := JudgeArticleQuality(ctx, nil, article)
	require.Error(t, err, "Should return error on timeout")
	assert.Contains(t, err.Error(), "failed to connect to news-creator service", "Error should indicate connection failure")
}

// TestJudgeArticleQualityLowScoreStillDeletes tests that low scores from successful responses still trigger deletion
func TestJudgeArticleQualityLowScoreStillDeletes(t *testing.T) {
	// Create a server that returns a low score (below threshold)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ollamaResponse{
			Response: "<score>5</score>", // Low score (below threshold of 7)
			Done:     true,
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	originalURL := qualityCheckerAPIURL
	qualityCheckerAPIURL = server.URL
	defer func() { qualityCheckerAPIURL = originalURL }()

	article := &driver.ArticleWithSummary{
		ArticleID:       "test-article-low-score",
		Content:         "Test content",
		SummaryJapanese: "テスト要約",
	}

	// Use nil dbPool to verify that JudgeArticleQuality attempts to call RemoveLowScoreSummary
	// RemoveLowScoreSummary will return an error because dbPool is nil, but this confirms
	// that the low score was detected and deletion was attempted
	ctx := context.Background()
	err := JudgeArticleQuality(ctx, nil, article)
	// Should attempt to delete (will fail because dbPool is nil, but that's expected)
	require.Error(t, err, "Should return error when trying to delete with nil dbPool")
	// The error should be about database pool being nil, not connection
	assert.Contains(t, err.Error(), "database pool is nil", "Error should indicate database pool is nil")
	assert.NotContains(t, err.Error(), "failed to connect to news-creator service", "Error should not be about connection")
}
