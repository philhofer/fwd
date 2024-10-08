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
// to the underlying reader
type partialReader struct {
	r io.Reader
}

func (p partialReader) Read(b []byte) (int, error) {
	n := max(1, rand.Intn(len(b)))
	return p.r.Read(b[:n])
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
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 128)

	if rd.BufferSize() != cap(rd.data) {
		t.Errorf("BufferSize() returned %d; should return %d", rd.BufferSize(), cap(rd.data))
	}

	// starting Buffered() should be 0
	if rd.Buffered() != 0 {
		t.Errorf("Buffered() should return 0 at initialization; got %d", rd.Buffered())
	}

	some := make([]byte, 32)
	n, err := rd.Read(some)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Fatal("read 0 bytes w/ a non-nil error!")
	}
	some = some[:n]

	more := make([]byte, 64)
	j, err := rd.Read(more)
	if err != nil {
		t.Fatal(err)
	}
	if j == 0 {
		t.Fatal("read 0 bytes w/ a non-nil error")
	}
	more = more[:j]

	out, err := ioutil.ReadAll(rd)
	if err != nil {
		t.Fatal(err)
	}

	all := append(some, more...)
	all = append(all, out...)

	if !bytes.Equal(bts, all) {
		t.Errorf("bytes not equal; %d bytes in and %d bytes out", len(bts), len(out))
	}

	// test filling out of the underlying reader
	big := randomBts(1 << 21)
	rd = NewReaderSize(partialReader{bytes.NewReader(big)}, 2048)
	buf := make([]byte, 3100)

	n, err = rd.ReadFull(buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3100 {
		t.Errorf("expected 3100 bytes read by ReadFull; got %d", n)
	}
	if !bytes.Equal(buf[:n], big[:n]) {
		t.Error("data parity")
	}
	rest := make([]byte, (1<<21)-3100)
	n, err = io.ReadFull(rd, rest)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(rest) {
		t.Errorf("expected %d bytes read by io.ReadFull; got %d", len(rest), n)
	}
	if !bytes.Equal(append(buf, rest...), big) {
		t.Fatal("data parity")
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

	// scan through the whole
	// array byte-by-byte
	for err != io.EOF {
		b, err = rd.ReadByte()
		if err == nil {
			if b != bts[i] {
				t.Fatalf("offset %d: %d in; %d out", i, b, bts[i])
			}
		}
		i++
	}
	if err != io.EOF {
		t.Fatal(err)
	}
}

func remaining(r *Reader) int {
	return r.Buffered() + r.r.(partialReader).r.(*bytes.Reader).Len()
}

func TestSkipNoSeek(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	n, err := rd.Skip(512)
	if err != nil {
		t.Fatal(err)
	}
	if n != 512 {
		t.Fatalf("Skip() returned a nil error, but skipped %d bytes instead of %d", n, 512)
	}

	if remaining(rd) != 512 {
		t.Errorf("expected 512 remaining; got %d", remaining(rd))
	}

	var b byte
	b, err = rd.ReadByte()
	if err != nil {
		t.Fatal(err)
	}

	if b != bts[512] {
		t.Errorf("at index %d: %d in; %d out", 512, bts[512], b)
	}

	n, err = rd.Skip(10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Fatalf("Skip() returned a nil error, but skipped %d bytes instead of %d", n, 10)
	}
	// the number of bytes remaining in the buffer needs
	// to comport with the number of bytes we expect to have skipped
	if want := 1024 - 512 - 10 - 1; remaining(rd) != want {
		t.Errorf("only %d bytes remaining (want %d)?", remaining(rd), want)
	}
	n, err = rd.Skip(10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Fatalf("Skip(10) a second time returned %d", n)
	}
	if want := 1024 - 512 - 10 - 10 - 1; remaining(rd) != want {
		t.Errorf("only %d bytes remaining (want %d)?", remaining(rd), want)
	}
	b, err = rd.ReadByte()
	if err != nil {
		t.Fatalf("second ReadByte(): %s", err)
	}
	if b != bts[512+10+10+1] {
		t.Errorf("expected %d but got %d", bts[512+10+10], b)
	}

	// now try to skip past the end; we expect
	// only to skip the number of bytes remaining
	want := remaining(rd)
	n, err = rd.Skip(2000)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected error %q; got %q", io.EOF, err)
	}
	if n != want {
		t.Fatalf("expected to skip only %d bytes; skipped %d", want, n)
	}
}

func TestSkipSeek(t *testing.T) {
	bts := randomBts(1024)

	// bytes.Reader implements io.Seeker
	rd := NewReaderSize(bytes.NewReader(bts), 200)

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

	n, err = rd.Skip(10)
	if err != nil {
		t.Fatal(err)
	}
	if n != 10 {
		t.Fatalf("Skip() returned a nil error, but skipped %d bytes instead of %d", n, 10)
	}

	// now try to skip past the end
	rd.Reset(bytes.NewReader(bts))

	// because of how bytes.Reader
	// implements Seek, this should
	// return (2000, nil)
	n, err = rd.Skip(2000)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2000 {
		t.Fatalf("should have returned %d bytes; returned %d", 2000, n)
	}

	// the next call to Read()
	// should return io.EOF
	n, err = rd.Read([]byte{0, 0, 0})
	if err != io.EOF {
		t.Errorf("expected %q; got %q", io.EOF, err)
	}
	if n != 0 {
		t.Errorf("expected 0 bytes read; got %d", n)
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

	// now try to peek past EOF
	peek, err = rd.Peek(2048)
	if err != io.EOF {
		t.Fatalf("expected error %q; got %q", io.EOF, err)
	}
	if len(peek) != 1024 {
		t.Fatalf("expected %d bytes peek-able; got %d", 1024, len(peek))
	}
}

func TestPeekByte(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	// first, a peek < buffer size
	var (
		peek byte
		err  error
	)
	rd.Skip(100)
	peek, err = rd.PeekByte()
	if err != nil {
		t.Fatal(err)
	}
	if peek != bts[100] {
		t.Fatalf("peeked byte not equal: want %d got %d", bts[100], peek)
	}

	// now, a peek > buffer size
	rd.Skip(156)
	peek, err = rd.PeekByte()
	if err != nil {
		t.Fatal(err)
	}
	if peek != bts[256] {
		t.Fatalf("peeked byte not equal: want %d got %d", bts[256], peek)
	}
}

func TestNext(t *testing.T) {
	size := 1024
	bts := randomBts(size)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	chunksize := 256
	chunks := size / chunksize

	for i := 0; i < chunks; i++ {
		out, err := rd.Next(chunksize)
		if err != nil {
			t.Fatal(err)
		}
		start := chunksize * i
		if !bytes.Equal(bts[start:start+chunksize], out) {
			t.Fatalf("chunk %d: chunks not equal", i+1)
		}
	}
}

func TestWriteTo(t *testing.T) {
	bts := randomBts(2048)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 200)

	// cause the buffer
	// to fill a little, just
	// to complicate things
	rd.Peek(25)

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

func TestReadFull(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 256)

	// try to ReadFull() the whole thing
	out := make([]byte, 1024)
	n, err := rd.ReadFull(out)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1024 {
		t.Fatalf("expected to read %d bytes; read %d", 1024, n)
	}
	if !bytes.Equal(bts, out) {
		t.Fatal("bytes not equal")
	}

	// we've read everything; this should EOF
	n, err = rd.Read(out)
	if err != io.EOF {
		t.Fatalf("expected %q; got %q", io.EOF, err)
	}

	rd.Reset(partialReader{bytes.NewReader(bts)})

	// now try to read *past* EOF
	out = make([]byte, 1500)
	n, err = rd.ReadFull(out)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("expected error %q; got %q", io.EOF, err)
	}
	if n != 1024 {
		t.Fatalf("expected to read %d bytes; read %d", 1024, n)
	}
}

func TestInputOffset(t *testing.T) {
	bts := randomBts(1024)
	rd := NewReaderSize(partialReader{bytes.NewReader(bts)}, 128)

	if rd.InputOffset() != 0 {
		t.Errorf("expected offset 0; got %d", rd.InputOffset())
	}

	// read a few bytes
	rd.ReadFull(make([]byte, 10))

	if rd.InputOffset() != 10 {
		t.Errorf("expected offset 10; got %d", rd.InputOffset())
	}

	rd.Peek(384)

	// peeking doesn't advance the offset
	if rd.InputOffset() != 10 {
		t.Errorf("expected offset 10; got %d", rd.InputOffset())
	}

	rd.Next(246)

	if rd.InputOffset() != 256 {
		t.Errorf("expected offset 256; got %d", rd.InputOffset())
	}

	rd.Skip(128)

	if rd.InputOffset() != 384 {
		t.Errorf("expected offset 384; got %d", rd.InputOffset())
	}

	rd.ReadByte()

	if rd.InputOffset() != 385 {
		t.Errorf("expected offset 385; got %d", rd.InputOffset())
	}

	n, _ := rd.Read(make([]byte, 128))

	if rd.InputOffset() != int64(385+n) {
		t.Errorf("expected offset %d; got %d", 385+n, rd.InputOffset())
	}

	rd.WriteTo(ioutil.Discard)

	if rd.InputOffset() != 1024 {
		t.Errorf("expected offset 1024; got %d", rd.InputOffset())
	}

	// try to read more
	_, err := rd.Read(make([]byte, 32))
	if err != io.EOF {
		t.Fatalf("expected error %q; got %q", io.EOF, err)
	}

	if rd.InputOffset() != 1024 {
		t.Errorf("expected offset 1024; got %d", rd.InputOffset())
	}

	// reset the reader
	rd.Reset(bytes.NewReader(bts))

	if rd.InputOffset() != 0 {
		t.Errorf("expected offset 0; got %d", rd.InputOffset())
	}

	rd.Skip(768 + 32)

	if rd.InputOffset() != 800 {
		t.Errorf("expected offset 800; got %d", rd.InputOffset())
	}

	rd.WriteTo(ioutil.Discard)

	if rd.InputOffset() != 1024 {
		t.Errorf("expected offset 1024; got %d", rd.InputOffset())
	}
}

type readCounter struct {
	r     io.Reader
	count int
}

func (r *readCounter) Read(p []byte) (int, error) {
	r.count++
	return r.r.Read(p)
}

func TestReadFullPerf(t *testing.T) {
	const size = 1 << 22
	data := randomBts(size)

	c := readCounter{
		r: &partialReader{
			r: bytes.NewReader(data),
		},
	}

	r := NewReader(&c)

	const segments = 4
	out := make([]byte, size/segments)

	for i := 0; i < segments; i++ {
		// force an unaligned read
		_, err := r.Peek(5)
		if err != nil {
			t.Fatal(err)
		}

		n, err := r.ReadFull(out)
		if err != nil {
			t.Fatal(err)
		}
		if n != size/segments {
			t.Fatalf("read %d bytes, not %d", n, size/segments)
		}
	}

	t.Logf("called Read() on the underlying reader %d times to fill %d buffers", c.count, size/r.BufferSize())
}

func TestReaderBufCreation(t *testing.T) {
	tests := []struct {
		name   string
		buffer []byte
		size   int
	}{
		{name: "nil", buffer: nil, size: minReaderSize},
		{name: "empty", buffer: []byte{}, size: minReaderSize},
		{name: "allocated", buffer: make([]byte, 0, 200), size: 200},
		{name: "filled", buffer: make([]byte, 200), size: 200},
	}

	for _, test := range tests {
		var b bytes.Buffer
		r := NewReaderBuf(&b, test.buffer)

		if r.BufferSize() != test.size {
			t.Errorf("%s: unequal buffer size (got: %d, expected: %d)", test.name, r.BufferSize(), test.size)
		}
		if r.Buffered() != 0 {
			t.Errorf("%s: unequal buffered bytes (got: %d, expected: 0)", test.name, r.Buffered())
		}
	}
}
