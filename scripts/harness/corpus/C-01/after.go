package middleware

import "errors"

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
	// BUG: errorlint autofix dropped `&& he.Code >= 500`, so 4xx errors also trip the breaker.
	var he *HTTPError
	if errors.As(err, &he) {
		recordFailure()
	}
}
