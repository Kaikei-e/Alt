package ledgers

import (
	"context"
	"fmt"
	"io"
)

func prepare(ctx context.Context, stdout, stderr io.Writer) (context.Context, io.Writer, io.Writer, int) {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if ctx == nil {
		fmt.Fprintln(stderr, "error: nil context")
		return nil, stdout, stderr, 2
	}
	return ctx, stdout, stderr, 0
}
