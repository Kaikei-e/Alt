package domain

import "errors"

type SummarizeOrigin string

const (
	SummarizeOriginUser   SummarizeOrigin = "user"
	SummarizeOriginSystem SummarizeOrigin = "system"
)

var ErrInvalidSummarizeOrigin = errors.New("summarize origin must be user or system")

func (o SummarizeOrigin) IsValid() bool {
	return o == SummarizeOriginUser || o == SummarizeOriginSystem
}

func (o SummarizeOrigin) Validate() error {
	if !o.IsValid() {
		return ErrInvalidSummarizeOrigin
	}
	return nil
}
