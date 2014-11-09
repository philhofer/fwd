package fwd

import (
	"bytes"
	"io"
	"io/ioutil"
	"math/rand"
	"testing"
	"unsafe"
)

// partialReader reads into only
// part of the supplied byte slice
//
type partialReader struct {
	r io.Reader
}

func (p partialReader) Read(b []byte) (int, error) {
	return p.r.Read(b[:rand.Intn(len(b))])
}

func randomBts(sz int) []byte {
	o := make([]byte, sz)
	for i := 0; i < len(o); i += 8 {
		j := (*int64)(unsafe.Pointer(&o[i]))
		*j = rand.Int63()
	}
	return o
}

func TestRead(t *testing.T) {
	bts := randomBts(512)

	// make the buffer much
	// smaller than the underlying
	// bytes to incur multiple fills
	rd := NewReaderSize(bytes.NewReader(bts), 128)

	if rd.BufferSize() != cap(rd.data) {
		t.Errorf("BufferSize() returned %d; should return %d", rd.BufferSize(), cap(rd.data))
	}

	// starting Buffered() should be 0
	if rd.Buffered() != 0 {
		t.Errorf("Buffered() should return 0 at initialization; got %d", rd.Buffered())
	}

	out, err := ioutil.ReadAll(rd)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(bts, out) {
		t.Errorf("bytes not equal; %d bytes in and %d bytes out", len(bts), len(out))
	}
}

func TestReadByte(t *testing.T) {
	bts := randomBts(512)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 98)

	var (
		err error
		i   int
		b   byte
	)
	for err != io.EOF {
		b, err = rd.ReadByte()
		if err == nil {
			if b != bts[i] {
				t.Fatalf("offset %d: %d in; %d out", b, bts[i])
			}
		}
		i++
	}
	if err != io.EOF {
		t.Fatal(err)
	}
}

func TestSkip(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	n, err := rd.Skip(512)
	if err != nil {
		t.Fatal(err)
	}
	if n != 512 {
		t.Fatalf("Skip() returned a nil error, but skipped %d bytes instead of %d", n, 512)
	}

	var b byte
	b, err = rd.ReadByte()
	if err != nil {
		t.Fatal(err)
	}

	if b != bts[512] {
		t.Fatalf("at index %d: %d in; %d out", 512, bts[512], b)
	}

	// now try to skip past the end
	rd = NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	n, err = rd.Skip(2000)
	if err != io.EOF {
		t.Fatalf("expected error %q; got %q", io.EOF, err)
	}
	if n != 1024 {
		t.Fatalf("expected to skip only 1024 bytes; skipped %d", n)
	}
}

func TestPeek(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	// first, a peek < buffer size
	var (
		peek []byte
		err  error
	)
	peek, err = rd.Peek(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(peek) != 100 {
		t.Fatalf("asked for %d bytes; got %d", 100, len(peek))
	}
	if !bytes.Equal(peek, bts[:100]) {
		t.Fatal("peeked bytes not equal")
	}

	// now, a peek > buffer size
	peek, err = rd.Peek(256)
	if err != nil {
		t.Fatal(err)
	}
	if len(peek) != 256 {
		t.Fatalf("asked for %d bytes; got %d", 100, len(peek))
	}
	if !bytes.Equal(peek, bts[:256]) {
		t.Fatal("peeked bytes not equal")
	}
}

func TestWriteTo(t *testing.T) {
	bts := randomBts(2048)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 128)

	var out bytes.Buffer
	n, err := rd.WriteTo(&out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2048 {
		t.Fatalf("should have written %d bytes; wrote %d", 2048, n)
	}
	if !bytes.Equal(out.Bytes(), bts) {
		t.Fatal("bytes not equal")
	}
}
