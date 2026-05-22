package seqio

import "io"

func closeWithError(errp *error, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil && *errp == nil {
		*errp = err
	}
}
