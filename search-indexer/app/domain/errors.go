package domain

import "errors"

// ErrArticleNotFound is returned by ArticleRepository.GetArticleByID when the
// requested article does not exist. Callers that treat a missing article as a
// skippable condition (e.g. batch indexing after an article was deleted)
// should check for it with errors.Is rather than relying on a nil article
// paired with a nil error.
var ErrArticleNotFound = errors.New("article not found")

// RepositoryError represents an error from the repository layer.
type RepositoryError struct {
	Op  string
	Err string
}

func (e *RepositoryError) Error() string {
	return e.Op + ": " + e.Err
}

// SearchEngineError represents an error from the search engine layer.
type SearchEngineError struct {
	Op  string
	Err string
}

func (e *SearchEngineError) Error() string {
	return e.Op + ": " + e.Err
}

// DriverError represents an error from the driver layer.
type DriverError struct {
	Op  string
	Err string
}

func (e *DriverError) Error() string {
	return e.Op + ": " + e.Err
}
