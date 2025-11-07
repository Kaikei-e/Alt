package recap_articles_usecase

import (
	"alt/domain"
	"alt/port/recap_articles_port"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Config carries tunables for pagination and date window validation.
type Config struct {
	DefaultPageSize int
	MaxPageSize     int
	MaxRangeDays    int
}

// Input encapsulates the filters accepted by the usecase.
type Input struct {
	From     time.Time
	To       time.Time
	Page     int
	PageSize int
	LangHint *string
	Fields   []string
}

// RecapArticlesUsecase validates recap article queries before delegating to storage.
type RecapArticlesUsecase struct {
	repo recap_articles_port.RecapArticlesPort
	cfg  Config
}

// NewRecapArticlesUsecase builds a new usecase instance.
func NewRecapArticlesUsecase(repo recap_articles_port.RecapArticlesPort, cfg Config) *RecapArticlesUsecase {
	return &RecapArticlesUsecase{repo: repo, cfg: cfg}
}

// Execute returns paginated recap articles for the requested range.
func (u *RecapArticlesUsecase) Execute(ctx context.Context, input Input) (*domain.RecapArticlesPage, error) {
	if input.To.IsZero() || input.From.IsZero() {
		return nil, errors.New("from/to parameters are required")
	}

	if !input.From.Before(input.To) {
		return nil, fmt.Errorf("from must be before to")
	}

	if u.cfg.MaxRangeDays > 0 {
		maxRange := time.Duration(u.cfg.MaxRangeDays) * 24 * time.Hour
		if input.To.Sub(input.From) > maxRange {
			return nil, fmt.Errorf("date range exceeds %d days", u.cfg.MaxRangeDays)
		}
	}

	page := input.Page
	if page == 0 {
		page = 1
	}
	if page < 1 {
		return nil, fmt.Errorf("page must be >= 1")
	}

	pageSize := input.PageSize
	if pageSize == 0 {
		pageSize = u.cfg.DefaultPageSize
	}
	if pageSize < 1 {
		return nil, fmt.Errorf("page_size must be >= 1")
	}
	if u.cfg.MaxPageSize > 0 && pageSize > u.cfg.MaxPageSize {
		return nil, fmt.Errorf("page_size must be <= %d", u.cfg.MaxPageSize)
	}

	fields, err := sanitizeFields(input.Fields)
	if err != nil {
		return nil, err
	}

	langHint, err := sanitizeLang(input.LangHint)
	if err != nil {
		return nil, err
	}

	query := domain.RecapArticlesQuery{
		From:     input.From,
		To:       input.To,
		Page:     page,
		PageSize: pageSize,
		LangHint: langHint,
		Fields:   fields,
	}

	return u.repo.FetchRecapArticles(ctx, query)
}

var allowedFields = map[string]struct{}{
	"title":    {},
	"fulltext": {},
}

func sanitizeFields(requested []string) ([]string, error) {
	if len(requested) == 0 {
		return requested, nil
	}

	seen := make(map[string]struct{}, len(requested))
	result := make([]string, 0, len(requested))
	for _, field := range requested {
		candidate := strings.ToLower(strings.TrimSpace(field))
		if candidate == "" {
			continue
		}
		if _, ok := allowedFields[candidate]; !ok {
			return nil, fmt.Errorf("unsupported field %q", field)
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}

	return result, nil
}

var allowedLangs = map[string]struct{}{
	"ja": {},
	"en": {},
}

func sanitizeLang(lang *string) (*string, error) {
	if lang == nil {
		return nil, nil
	}
	candidate := strings.ToLower(strings.TrimSpace(*lang))
	if candidate == "" || candidate == "*" {
		return nil, nil
	}
	if _, ok := allowedLangs[candidate]; !ok {
		return nil, fmt.Errorf("unsupported lang %q", *lang)
	}
	return &candidate, nil
}
