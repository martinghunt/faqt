package seqio

import (
	"errors"
	"testing"
)

func TestCloseWithErrorUsesCloseErrorOnlyOnSuccess(t *testing.T) {
	closeErr := errors.New("close failed")
	var err error
	closeWithError(&err, closeErrorStub{err: closeErr})
	if err != closeErr {
		t.Fatalf("closeWithError() error = %v, want close error", err)
	}

	mainErr := errors.New("main failed")
	err = mainErr
	closeWithError(&err, closeErrorStub{err: closeErr})
	if err != mainErr {
		t.Fatalf("closeWithError() error = %v, want original error", err)
	}
}

type closeErrorStub struct {
	err error
}

func (c closeErrorStub) Close() error {
	return c.err
}
