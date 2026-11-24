package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
)

func confirm(ctx context.Context, in io.Reader) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	// Arbitrary readers cannot be interrupted safely. A buffered result lets the
	// input goroutine finish once its owner closes the reader, even after cancel.
	result := make(chan bool, 1)
	go func() {
		var answer string
		if _, err := fmt.Fscanln(in, &answer); err != nil {
			result <- false
			return
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		result <- answer == "y" || answer == "yes"
	}()
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case confirmed := <-result:
		if err := ctx.Err(); err != nil {
			return false, err
		}
		return confirmed, nil
	}
}
