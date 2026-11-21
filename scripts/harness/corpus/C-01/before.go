package middleware

type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string {
	return e.Message
}

func recordServerError(err error, recordFailure func()) {
	if err == nil {
		return
	}
	// Only 5xx server errors should trip the circuit breaker; 4xx must be ignored.
	if he, ok := err.(*HTTPError); ok && he.Code >= 500 {
		recordFailure()
	}
}
