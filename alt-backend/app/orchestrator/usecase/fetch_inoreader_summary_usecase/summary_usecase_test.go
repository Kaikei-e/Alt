package fetch_inoreader_summary_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/logger"
	"context"
	"errors"
	"net"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestFetchInoreaderSummaryUsecase_Execute_Success(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	tests := []struct {
		name       string
		urls       []string
		mockResult []*domain.InoreaderSummary
		want       []*domain.InoreaderSummary
		wantErr    bool
	}{
		{
			name: "successful fetch with multiple articles",
			urls: []string{"https://93.184.216.34/article1", "https://93.184.216.34/article2"},
			mockResult: []*domain.InoreaderSummary{
				{
					ArticleURL:  "https://93.184.216.34/article1",
					Title:       "Test Article 1",
					Author:      stringPtr("Test Author 1"),
					Content:     "This is test content 1",
					ContentType: "html",
					PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					FetchedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
					InoreaderID: "inoreader123",
				},
			},
			want: []*domain.InoreaderSummary{
				{
					ArticleURL:  "https://93.184.216.34/article1",
					Title:       "Test Article 1",
					Author:      stringPtr("Test Author 1"),
					Content:     "This is test content 1",
					ContentType: "html",
					PublishedAt: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC),
					FetchedAt:   time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC),
					InoreaderID: "inoreader123",
				},
			},
			wantErr: false,
		},
		{
			name:       "empty URLs should return empty result",
			urls:       []string{},
			mockResult: []*domain.InoreaderSummary{},
			want:       []*domain.InoreaderSummary{},
			wantErr:    false,
		},
		{
			name:       "too many URLs should return error",
			urls:       make([]string, 51), // More than 50
			mockResult: nil,
			want:       nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectations only for valid cases that actually call the port
			if !tt.wantErr && len(tt.urls) > 0 && len(tt.urls) <= 50 {
				mockPort.EXPECT().
					FetchSummariesByURLs(gomock.Any(), tt.urls).
					Return(tt.mockResult, nil).
					Times(1)
			}

			usecase := NewFetchInoreaderSummaryUsecase(mockPort)

			// Execute
			result, err := usecase.Execute(context.Background(), tt.urls)

			// Assert
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.want), len(result))

				for i, expected := range tt.want {
					if i < len(result) {
						assert.Equal(t, expected.ArticleURL, result[i].ArticleURL)
						assert.Equal(t, expected.Title, result[i].Title)
						assert.Equal(t, expected.Content, result[i].Content)
						assert.Equal(t, expected.InoreaderID, result[i].InoreaderID)
					}
				}
			}
		})
	}
}

func TestFetchInoreaderSummaryUsecase_Execute_URLValidation(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	tests := []struct {
		name    string
		urls    []string
		wantErr bool
	}{
		{
			name:    "valid HTTPS URLs should pass",
			urls:    []string{"https://93.184.216.34/article1"},
			wantErr: false,
		},
		{
			name:    "valid HTTP URLs should pass",
			urls:    []string{"http://93.184.216.34/article1"},
			wantErr: false,
		},
		{
			name:    "localhost should be rejected for security",
			urls:    []string{"http://localhost/article1"},
			wantErr: true,
		},
		{
			name:    "private IP should be rejected for security",
			urls:    []string{"http://192.168.1.1/article1"},
			wantErr: true,
		},
		{
			name:    "invalid scheme should be rejected",
			urls:    []string{"ftp://93.184.216.34/article1"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock expectation only for valid cases
			if !tt.wantErr {
				mockPort.EXPECT().
					FetchSummariesByURLs(gomock.Any(), tt.urls).
					Return([]*domain.InoreaderSummary{}, nil).
					Times(1)
			}

			usecase := NewFetchInoreaderSummaryUsecase(mockPort)
			_, err := usecase.Execute(context.Background(), tt.urls)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestFetchInoreaderSummaryUsecase_Execute_PortError(t *testing.T) {
	logger.InitLogger()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockFetchInoreaderSummaryPort(ctrl)

	// Setup mock to return error
	urls := []string{"https://93.184.216.34/article1"}
	mockPort.EXPECT().
		FetchSummariesByURLs(gomock.Any(), urls).
		Return(nil, assert.AnError).
		Times(1)

	usecase := NewFetchInoreaderSummaryUsecase(mockPort)
	result, err := usecase.Execute(context.Background(), urls)

	assert.Error(t, err)
	assert.Nil(t, result)
}

func stringPtr(s string) *string {
	return &s
}

func TestRemoveDuplicateURLs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "empty input",
			input:    []string{},
			expected: []string{},
		},
		{
			name:     "no duplicates",
			input:    []string{"https://a.com", "https://b.com"},
			expected: []string{"https://a.com", "https://b.com"},
		},
		{
			name:     "with duplicates preserves first occurrence order",
			input:    []string{"https://a.com", "https://b.com", "https://a.com", "https://c.com", "https://b.com"},
			expected: []string{"https://a.com", "https://b.com", "https://c.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeDuplicateURLs(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestIsPrivateIPAddress(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		{name: "loopback v4", ip: "127.0.0.1", expected: true},
		{name: "loopback v6", ip: "::1", expected: true},
		{name: "10.0.0.1", ip: "10.0.0.1", expected: true},
		{name: "172.16.0.1", ip: "172.16.0.1", expected: true},
		{name: "172.31.255.255", ip: "172.31.255.255", expected: true},
		{name: "172.32.0.1 (public)", ip: "172.32.0.1", expected: false},
		{name: "192.168.1.100", ip: "192.168.1.100", expected: true},
		{name: "public 8.8.8.8", ip: "8.8.8.8", expected: false},
		{name: "ipv6 unique local fc00", ip: "fc00::1", expected: true},
		{name: "ipv6 unique local fd00", ip: "fd12:3456::1", expected: true},
		{name: "ipv6 public", ip: "2607:f8b0:4005:805::200e", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := net.ParseIP(tt.ip)
			assert.NotNil(t, parsed)
			assert.Equal(t, tt.expected, isPrivateIPAddress(parsed))
		})
	}
}

func TestIsPrivateHost(t *testing.T) {
	tests := []struct {
		name       string
		hostname   string
		lookupFunc func(host string) ([]net.IP, error)
		expected   bool
	}{
		{
			name:     "literal private IP",
			hostname: "10.0.0.1",
			lookupFunc: func(host string) ([]net.IP, error) {
				t.Fatal("lookup should not be called for IP literal")
				return nil, nil
			},
			expected: true,
		},
		{
			name:     "literal public IP",
			hostname: "93.184.216.34",
			lookupFunc: func(host string) ([]net.IP, error) {
				t.Fatal("lookup should not be called for IP literal")
				return nil, nil
			},
			expected: false,
		},
		{
			name:     "hostname resolving to private IP",
			hostname: "internal.service",
			lookupFunc: func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("192.168.1.1")}, nil
			},
			expected: true,
		},
		{
			name:     "hostname resolving to public IP",
			hostname: "example.com",
			lookupFunc: func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			},
			expected: false,
		},
		{
			name:     "hostname resolution error",
			hostname: "invalid.domain",
			lookupFunc: func(host string) ([]net.IP, error) {
				return nil, errors.New("no such host")
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isPrivateHost(tt.hostname, tt.lookupFunc)
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestFetchInoreaderSummaryUsecase_PinnedURLValidation(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		lookupFunc func(host string) ([]net.IP, error)
		wantMsg    string
	}{
		{
			name:   `"localhost" (lookup returns 127.0.0.1) -> access to private networks not allowed`,
			rawURL: "http://localhost/article1",
			lookupFunc: func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("127.0.0.1")}, nil
			},
			wantMsg: "access to private networks not allowed",
		},
		{
			name:   `"169.254.169.254" -> access to private networks not allowed`,
			rawURL: "http://169.254.169.254/article1",
			lookupFunc: func(host string) ([]net.IP, error) {
				return nil, errors.New("lookup should not be called for literal IP")
			},
			wantMsg: "access to private networks not allowed",
		},
		{
			name:   `"foo.internal" with lookup returning a public IP -> access to internal domains not allowed`,
			rawURL: "http://foo.internal/article1",
			lookupFunc: func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			},
			wantMsg: "access to internal domains not allowed",
		},
		{
			name:   `lookup error -> access to private networks not allowed`,
			rawURL: "http://unknown-host.example.com/article1",
			lookupFunc: func(host string) ([]net.IP, error) {
				return nil, errors.New("dns lookup failed")
			},
			wantMsg: "access to private networks not allowed",
		},
		{
			name:   `invalid scheme -> only HTTP and HTTPS schemes allowed`,
			rawURL: "ftp://example.com/article1",
			lookupFunc: func(host string) ([]net.IP, error) {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			},
			wantMsg: "only HTTP and HTTPS schemes allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := &fetchInoreaderSummaryUsecase{
				port:     nil,
				lookupIP: tt.lookupFunc,
			}

			parsed, err := url.Parse(tt.rawURL)
			assert.NoError(t, err)

			err = uc.isAllowedURL(parsed)
			assert.Error(t, err)
			assert.Equal(t, tt.wantMsg, err.Error())
		})
	}
}
