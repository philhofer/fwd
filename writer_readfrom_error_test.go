package fwd

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

type readFromErrorReader struct {
	data  []byte
	err   error
	calls int
}

func (r *readFromErrorReader) Read(p []byte) (int, error) {
	r.calls++
	n := copy(p, r.data)
	r.data = r.data[n:]
	return n, r.err
}

type readFromErrorWriter struct {
	bytes.Buffer
	limit int
	err   error
}

func (w *readFromErrorWriter) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		p = p[:w.limit]
	}
	n, _ := w.Buffer.Write(p)
	return n, w.err
}

func TestWriterReadFromErrors(t *testing.T) {
	readErr, writeErr := errors.New("read failed"), errors.New("write failed")
	for _, tt := range []struct {
		name              string
		readErr, writeErr error
		written           int
		wantErr           error
	}{
		{name: "read error with data", readErr: readErr, written: 4, wantErr: readErr},
		{name: "EOF with data", readErr: io.EOF, written: 4},
		{name: "write error", writeErr: writeErr, written: 2, wantErr: writeErr},
		{name: "both errors", readErr: readErr, writeErr: writeErr, written: 2, wantErr: writeErr},
		{name: "short write", written: 2, wantErr: io.ErrShortWrite},
		{name: "zero write", written: 0, wantErr: io.ErrShortWrite},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := &readFromErrorReader{data: []byte("data"), err: tt.readErr}
			dst := &readFromErrorWriter{limit: tt.written, err: tt.writeErr}
			w := NewWriter(dst)
			n, err := w.ReadFrom(r)
			if n != int64(tt.written) || !errors.Is(err, tt.wantErr) {
				t.Fatalf("ReadFrom: got %d, %v; want %d, %v", n, err, tt.written, tt.wantErr)
			}
			if r.calls != 1 {
				t.Fatalf("read %d times after terminal result", r.calls)
			}
			if got, want := w.Buffered(), 4-tt.written; got != want {
				t.Fatalf("buffered %d bytes, want %d", got, want)
			}
			// Recovering the destination and flushing must write only the suffix,
			// never replay the prefix or expose unused buffer capacity.
			dst.limit, dst.err = 100, nil
			if err := w.Flush(); err != nil {
				t.Fatal(err)
			}
			if got := dst.String(); got != "data" {
				t.Fatalf("recovered output %q, want data", got)
			}
		})
	}
}
