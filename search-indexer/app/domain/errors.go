package domain

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
