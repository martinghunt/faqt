package closeutil

import "io"

type multiCloser struct {
	closers []io.Closer
}

func CloseWithError(errp *error, closer io.Closer) {
	if closer == nil {
		return
	}
	if err := closer.Close(); err != nil && *errp == nil {
		*errp = err
	}
}

func MultiCloser(closers ...io.Closer) io.Closer {
	filtered := make([]io.Closer, 0, len(closers))
	for _, closer := range closers {
		if closer != nil {
			filtered = append(filtered, closer)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &multiCloser{closers: filtered}
}

func (m *multiCloser) Close() error {
	var first error
	for i := len(m.closers) - 1; i >= 0; i-- {
		if err := m.closers[i].Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
