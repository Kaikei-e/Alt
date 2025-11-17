package image_fetch_usecase

import (
	"alt/domain"
	"alt/port/image_fetch_port"
	"alt/utils/errors"
	"context"
)

// ImageFetchUsecaseInterface defines the interface for image fetching usecase
type ImageFetchUsecaseInterface interface {
	Execute(ctx context.Context, rawURL string, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error)
}

// ImageFetchUsecase orchestrates image fetching business logic
type ImageFetchUsecase struct {
	imageFetchPort image_fetch_port.ImageFetchPort
}

// NewImageFetchUsecase creates a new ImageFetchUsecase
func NewImageFetchUsecase(imageFetchPort image_fetch_port.ImageFetchPort) *ImageFetchUsecase {
	return &ImageFetchUsecase{
		imageFetchPort: imageFetchPort,
	}
}

// Execute performs image fetching with validation and business rules
func (u *ImageFetchUsecase) Execute(ctx context.Context, rawURL string, options *domain.ImageFetchOptions) (*domain.ImageFetchResult, error) {
	// Check for context cancellation early
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	// Validate and parse URL
	imageURL, err := domain.ValidateImageURL(rawURL)
	if err != nil {
		return nil, errors.NewValidationContextError(
			err.Error(),
			"usecase",
			"ImageFetchUsecase",
			"validate_url",
			map[string]interface{}{
				"raw_url": rawURL,
			},
		)
	}

	// Check domain whitelist
	if !domain.IsAllowedImageDomain(imageURL.Hostname()) {
		return nil, errors.NewValidationContextError(
			"domain not allowed",
			"usecase",
			"ImageFetchUsecase",
			"validate_domain",
			map[string]interface{}{
				"domain": imageURL.Hostname(),
				"url":    rawURL,
			},
		)
	}

	// Check path validity (basic image path validation)
	if !domain.IsValidImagePath(imageURL.Path) {
		return nil, errors.NewValidationContextError(
			"path does not appear to be a valid image",
			"usecase",
			"ImageFetchUsecase",
			"validate_path",
			map[string]interface{}{
				"path": imageURL.Path,
				"url":  rawURL,
			},
		)
	}

	// Use default options if not provided
	if options == nil {
		options = domain.NewImageFetchOptions()
	}

	// Call the port to fetch the image
	result, err := u.imageFetchPort.FetchImage(ctx, imageURL, options)
	if err != nil {
		// The error from the port layer should already be properly wrapped
		return nil, err
	}

	return result, nil
}
