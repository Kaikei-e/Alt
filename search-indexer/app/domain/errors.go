package domain

import "errors"

// ErrArticleNotFound is returned by ArticleRepository.GetArticleByID when the
// requested article does not exist. Callers that treat a missing article as a
// skippable condition (e.g. batch indexing after an article was deleted)
// should check for it with errors.Is rather than relying on a nil article
// paired with a nil error.
var ErrArticleNotFound = errors.New("article not found")

// RepositoryError represents an error from the repository layer. Err is a
// real error (not a stringified copy) so callers can use errors.Is/As to
// classify the cause instead of matching on Error() text.
type RepositoryError struct {
	Op  string
	Err error
}

func (e *RepositoryError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *RepositoryError) Unwrap() error {
	return e.Err
}

// SearchEngineError represents an error from the search engine layer.
type SearchEngineError struct {
	Op  string
	Err error
}

func (e *SearchEngineError) Error() string {
	return e.Op + ": " + e.Err.Error()
}

func (e *SearchEngineError) Unwrap() error {
	return e.Err
}
