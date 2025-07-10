package usecase

import (
	"context"
	"search-indexer/domain"
	"search-indexer/port"
)

// LoadConfigUsecase handles loading configuration
type LoadConfigUsecase struct {
	configRepo port.ConfigRepository
}

// LoadConfigResult represents the result of loading configuration
type LoadConfigResult struct {
	Config *domain.SearchIndexerConfig
}

// NewLoadConfigUsecase creates a new LoadConfigUsecase
func NewLoadConfigUsecase(configRepo port.ConfigRepository) *LoadConfigUsecase {
	return &LoadConfigUsecase{
		configRepo: configRepo,
	}
}

// Execute loads the configuration
func (u *LoadConfigUsecase) Execute(ctx context.Context) (*LoadConfigResult, error) {
	config, err := u.configRepo.LoadSearchIndexerConfig()
	if err != nil {
		return nil, err
	}

	return &LoadConfigResult{
		Config: config,
	}, nil
}
