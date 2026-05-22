package closeutil

import (
	"errors"
	"testing"
)

func TestMultiCloserCloseOrderAndFirstError(t *testing.T) {
	firstErr := errors.New("first")
	var order []string
	mc := MultiCloser(
		closeRecorder{name: "first", order: &order, err: firstErr},
		nil,
		closeRecorder{name: "second", order: &order},
	)
	if err := mc.Close(); err != firstErr {
		t.Fatalf("Close() error = %v, want first closer error", err)
	}
	if got := order; len(got) != 2 || got[0] != "second" || got[1] != "first" {
		t.Fatalf("close order = %v, want [second first]", got)
	}
}

func TestMultiCloserEmpty(t *testing.T) {
	if got := MultiCloser(nil); got != nil {
		t.Fatalf("MultiCloser(nil) = %v, want nil", got)
	}
}

type closeRecorder struct {
	name  string
	order *[]string
	err   error
}

func (c closeRecorder) Close() error {
	*c.order = append(*c.order, c.name)
	return c.err
}
