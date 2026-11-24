package native

import (
	"bytes"
	"context"
	"io"
)

type memoryBytes []byte

func (b memoryBytes) Size() int64 { return int64(len(b)) }

func (b memoryBytes) OpenRange(_ context.Context, offset int64) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(b[offset:])), nil
}
