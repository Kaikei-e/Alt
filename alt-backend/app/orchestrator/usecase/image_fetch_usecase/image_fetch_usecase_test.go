package image_fetch_usecase

import (
	"alt/domain"
	"alt/mocks"
	"alt/utils/errors"
	"context"
	stderrors "errors"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestImageFetchUsecase_Execute(t *testing.T) {
	tests := []struct {
		name      string
		rawURL    string
		options   *domain.ImageFetchOptions
		mockSetup func(mockPort *mocks.MockImageFetchPort)
		want      *domain.ImageFetchResult
		wantErr   bool
		errCode   string
	}{
		{
			name:    "successful image fetch with default options",
			rawURL:  "https://9to5mac.com/wp-content/uploads/sites/6/2025/07/ios-26-home-screen.jpg",
			options: nil, // Will use defaults
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				expectedURL, _ := url.Parse("https://9to5mac.com/wp-content/uploads/sites/6/2025/07/ios-26-home-screen.jpg")
				expectedOptions := domain.NewImageFetchOptions()

				mockPort.EXPECT().
					FetchImage(gomock.Any(), expectedURL, expectedOptions).
					Return(&domain.ImageFetchResult{
						URL:         "https://9to5mac.com/wp-content/uploads/sites/6/2025/07/ios-26-home-screen.jpg",
						ContentType: "image/jpeg",
						Data:        []byte("fake-image-data"),
						Size:        15,
						FetchedAt:   time.Now(),
					}, nil)
			},
			want: &domain.ImageFetchResult{
				URL:         "https://9to5mac.com/wp-content/uploads/sites/6/2025/07/ios-26-home-screen.jpg",
				ContentType: "image/jpeg",
				Data:        []byte("fake-image-data"),
				Size:        15,
			},
			wantErr: false,
		},
		{
			name:   "successful fetch with custom options",
			rawURL: "https://images.unsplash.com/photo-1506905925346-21bda4d32df4",
			options: &domain.ImageFetchOptions{
				MaxSize: 2 * 1024 * 1024, // 2MB
				Timeout: 15 * time.Second,
			},
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				expectedURL, _ := url.Parse("https://images.unsplash.com/photo-1506905925346-21bda4d32df4")
				expectedOptions := &domain.ImageFetchOptions{
					MaxSize: 2 * 1024 * 1024,
					Timeout: 15 * time.Second,
				}

				mockPort.EXPECT().
					FetchImage(gomock.Any(), expectedURL, expectedOptions).
					Return(&domain.ImageFetchResult{
						URL:         "https://images.unsplash.com/photo-1506905925346-21bda4d32df4",
						ContentType: "image/jpeg",
						Data:        []byte("unsplash-image-data"),
						Size:        25,
						FetchedAt:   time.Now(),
					}, nil)
			},
			want: &domain.ImageFetchResult{
				URL:         "https://images.unsplash.com/photo-1506905925346-21bda4d32df4",
				ContentType: "image/jpeg",
				Data:        []byte("unsplash-image-data"),
				Size:        25,
			},
			wantErr: false,
		},
		{
			name:    "validation error - empty URL",
			rawURL:  "",
			options: nil,
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				// No mock calls expected for validation errors
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:    "validation error - invalid URL format",
			rawURL:  "not-a-valid-url",
			options: nil,
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				// No mock calls expected
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:    "validation error - non-HTTPS URL",
			rawURL:  "http://9to5mac.com/image.jpg",
			options: nil,
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				// No mock calls expected
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:    "validation error - domain not allowed",
			rawURL:  "https://evil-site.com/image.jpg",
			options: nil,
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				// No mock calls expected
			},
			want:    nil,
			wantErr: true,
			errCode: "VALIDATION_ERROR",
		},
		{
			name:    "gateway error - network failure",
			rawURL:  "https://9to5mac.com/wp-content/uploads/image.jpg",
			options: nil,
			mockSetup: func(mockPort *mocks.MockImageFetchPort) {
				expectedURL, _ := url.Parse("https://9to5mac.com/wp-content/uploads/image.jpg")
				expectedOptions := domain.NewImageFetchOptions()

				mockPort.EXPECT().
					FetchImage(gomock.Any(), expectedURL, expectedOptions).
					Return(nil, errors.NewExternalAPIContextError(
						"network timeout",
						"gateway",
						"ImageFetchGateway",
						"fetch_external_image",
						stderrors.New("context deadline exceeded"),
						map[string]interface{}{
							"url": "https://9to5mac.com/wp-content/uploads/image.jpg",
						},
					))
			},
			want:    nil,
			wantErr: true,
			errCode: "EXTERNAL_API_ERROR",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPort := mocks.NewMockImageFetchPort(ctrl)
			tt.mockSetup(mockPort)

			usecase := NewImageFetchUsecase(mockPort)

			got, err := usecase.Execute(context.Background(), tt.rawURL, tt.options)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errCode != "" {
					if appErr, ok := err.(*errors.AppContextError); ok {
						assert.Equal(t, tt.errCode, appErr.Code)
					}
				}
				assert.Nil(t, got)
			} else {
				assert.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, tt.want.URL, got.URL)
				assert.Equal(t, tt.want.ContentType, got.ContentType)
				assert.Equal(t, tt.want.Data, got.Data)
				assert.Equal(t, tt.want.Size, got.Size)
				assert.NotZero(t, got.FetchedAt) // Should be set
			}
		})
	}
}

func TestImageFetchUsecase_Execute_ContextCancellation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockPort := mocks.NewMockImageFetchPort(ctrl)
	usecase := NewImageFetchUsecase(mockPort)

	// Create a context that's already canceled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := usecase.Execute(ctx, "https://9to5mac.com/image.jpg", nil)

	assert.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestImageFetchUsecase_Execute_ClampsOptions(t *testing.T) {
	tests := []struct {
		name        string
		in          *domain.ImageFetchOptions
		wantMaxSize int
		wantTimeout time.Duration
	}{
		{
			name:        "zero values default to 5MB and 30s",
			in:          &domain.ImageFetchOptions{MaxSize: 0, Timeout: 0},
			wantMaxSize: 5 * 1024 * 1024,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "negative values default to 5MB and 30s",
			in:          &domain.ImageFetchOptions{MaxSize: -100, Timeout: -5 * time.Second},
			wantMaxSize: 5 * 1024 * 1024,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "oversized MaxSize and Timeout clamped to 10MB and 30s",
			in:          &domain.ImageFetchOptions{MaxSize: 100 * 1024 * 1024, Timeout: 5 * time.Minute},
			wantMaxSize: 10 * 1024 * 1024,
			wantTimeout: 30 * time.Second,
		},
		{
			name:        "sub-second positive Timeout clamped to 1s",
			in:          &domain.ImageFetchOptions{MaxSize: 1024 * 1024, Timeout: 50 * time.Millisecond},
			wantMaxSize: 1024 * 1024,
			wantTimeout: 1 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPort := mocks.NewMockImageFetchPort(ctrl)
			mockPort.EXPECT().
				FetchImage(gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(&domain.ImageFetchOptions{})).
				DoAndReturn(func(_ context.Context, _ *url.URL, opts *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
					assert.Equal(t, tt.wantMaxSize, opts.MaxSize)
					assert.Equal(t, tt.wantTimeout, opts.Timeout)
					return &domain.ImageFetchResult{
						URL:         "https://9to5mac.com/image.jpg",
						ContentType: "image/jpeg",
						Data:        []byte("ok"),
						Size:        2,
						FetchedAt:   time.Now(),
					}, nil
				})

			uc := NewImageFetchUsecase(mockPort)
			_, err := uc.Execute(context.Background(), "https://9to5mac.com/image.jpg", tt.in)
			require.NoError(t, err)
		})
	}
}
