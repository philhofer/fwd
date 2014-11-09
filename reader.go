// The `fwd` package provides
// a buffered reader that can
// seek forward an arbitrary
// number of bytes.
package fwd

import (
	"io"
)

const (
	// DefaultReaderSize is the default size of the read buffer
	DefaultReaderSize = 2048

	// minimum read buffer; straight from bufio
	minReaderSize = 16
)

// NewReader returns a new *Reader that reads from 'r'
func NewReader(r io.Reader) *Reader {
	return NewReaderSize(r, DefaultReaderSize)
}

// NewReaderSize returns a new *Reader that
// reads from 'r' and has a buffer size 'n'
func NewReaderSize(r io.Reader, n int) *Reader {
	return &Reader{
		r:    r,
		data: make([]byte, 0, max(minReaderSize, n)),
	}
}

// Reader is a buffered look-ahead reader.
type Reader struct {
	r io.Reader // underlying reader

	// data[n:len(data)] is buffered data; data[len(data):cap(data)] is free buffer space
	data  []byte // data
	n     int    // read offset
	state error  // last read error
}

// Reset resets the state of the reader and
// sets the buffer size to 'size'. If size is
// less than 0, the buffer size remains unchanged.
func (r *Reader) Reset(rd io.Reader) {
	r.r = rd
	r.data = r.data[0:0]
	r.n = 0
	r.state = nil
}

// more() does one read on the underlying reader
func (r *Reader) more() {
	// move data backwards so that
	// the read offset is 0
	if r.n != 0 {
		r.data = r.data[:copy(r.data[0:], r.data[r.n:])]
		r.n = 0
	}
	var a int
	a, r.state = r.r.Read(r.data[len(r.data):cap(r.data)])
	r.data = r.data[:len(r.data)+a]
}

// buffered bytes
func (r *Reader) buffered() int { return len(r.data) - r.n }

// Buffered returns the number of bytes currently in the buffer
func (r *Reader) Buffered() int { return len(r.data) - r.n }

// BufferSize returns the total size of the buffer
func (r *Reader) BufferSize() int { return cap(r.data) }

// Peek returns the next 'n' buffered bytes,
// reading from the underlying reader if necessary.
// It will only return a slice shorter than 'n' bytes
// if it also returns an error. Peek does not advance
// the reader.
func (r *Reader) Peek(n int) ([]byte, error) {
	// in the degenerate case,
	// we may need to realloc
	// (the caller asked for more
	// bytes than the size of the buffer)
	if cap(r.data) < n {
		old := r.data[r.n:]
		r.data = make([]byte, n+r.buffered())
		r.data = r.data[:copy(r.data, old)]
	}

	// keep filling until
	// we hit an error or
	// read enough bytes
	for r.buffered() < n && r.state == nil {
		r.more()
	}

	// we must have hit an error
	if r.buffered() < n {
		e := r.state
		r.state = nil
		return r.data[r.n:], e
	}

	return r.data[r.n : r.n+n], nil
}

// Forward moves the reader forward 'n' bytes.
// Returns the number of bytes skipped and any
// errors encountered
func (r *Reader) Skip(n int) (int, error) {

	// fast path
	if r.buffered() >= n {
		r.n += n
		return n, nil
	}

	// EOF or other error
	if r.state != nil {
		s := r.buffered()
		r.n += s
		e := r.state
		r.state = nil
		return s, e
	}

	// loop on filling
	// and then erasing
	var skipped int
	for r.buffered() < n && r.state == nil {
		r.more()
		step := min(r.buffered(), n)
		skipped += step
		r.n += step
		n -= step
	}
	return skipped, r.state
}

// Read implements io.Reader
func (r *Reader) Read(b []byte) (int, error) {
	if len(b) <= r.buffered() {
		x := copy(b, r.data[r.n:])
		r.n += x
		return x, nil
	}
	r.more()
	if r.buffered() > 0 {
		x := copy(b, r.data[r.n:])
		r.n += x
		return x, nil
	}

	// io.Reader is supposed to return
	// 0 read bytes on error
	e := r.state
	r.state = nil
	return 0, e
}

// ReadByte implements io.ByteReader
func (r *Reader) ReadByte() (byte, error) {
	if r.buffered() < 1 && r.state == nil {
		r.more()
	}
	if r.buffered() < 1 {
		e := r.state
		r.state = nil
		return 0, e
	}
	b := r.data[r.n]
	r.n++
	return b, nil
}

// WriteTo imlements io.WriterTo
func (r *Reader) WriteTo(w io.Writer) (int64, error) {
	var (
		i   int64
		ii  int
		err error
	)
	// first, clear buffer
	if r.buffered() > 0 {
		ii, err = w.Write(r.data[r.n:])
		i += int64(ii)
		if err != nil {
			return i, err
		}
		r.data = r.data[0:0]
		r.n = 0
	}
	for r.state == nil {
		// here we just do
		// 1:1 reads and writes
		r.more()
		if r.buffered() > 0 {
			ii, err = w.Write(r.data)
			i += int64(ii)
			if err != nil {
				return i, err
			}
			r.data = r.data[0:0]
			r.n = 0
		}
	}
	if r.state != io.EOF {
		return i, err
	}
	return i, nil
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a < b {
		return b
	}
	return a
}
