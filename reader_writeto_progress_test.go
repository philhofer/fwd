package fwd_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/philhofer/fwd"
)

func TestWriteToRetainsProgressAfterPipeFailure(t *testing.T) {
	for _, consumed := range []int{0, 3} {
		t.Run(map[int]string{0: "fresh", 3: "partly consumed buffer"}[consumed], func(t *testing.T) {
			data := []byte("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
			source := fwd.NewReaderSize(bytes.NewReader(data), 16)
			if _, err := source.Peek(8); err != nil {
				t.Fatal(err)
			}
			if _, err := source.Next(consumed); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("destination stopped")
			receiver, sender := io.Pipe()
			prefix := make([]byte, 4)
			received := make(chan error, 1)
			go func() { _, err := io.ReadFull(receiver, prefix); receiver.CloseWithError(failure); received <- err }()
			n, err := source.WriteTo(sender)
			sender.Close()
			if readErr := <-received; readErr != nil {
				t.Fatal(readErr)
			}
			if n != 4 || !errors.Is(err, failure) {
				t.Fatalf("WriteTo = %d,%v; want4,original failure", n, err)
			}
			if source.InputOffset() != int64(consumed)+n {
				t.Errorf("InputOffset = %d, want%d", source.InputOffset(), int64(consumed)+n)
			}
			var rest bytes.Buffer
			count, err := source.WriteTo(&rest)
			if err != nil {
				t.Fatal(err)
			}
			got := append(prefix, rest.Bytes()...)
			if !bytes.Equal(got, data[consumed:]) {
				t.Errorf("retry returned duplicated or missing data: %q", got)
			}
			if count != int64(len(data)-consumed-4) || source.InputOffset() != int64(len(data)) {
				t.Errorf("retry count/offset=%d/%d", count, source.InputOffset())
			}
		})
	}
}

type shortDestination struct{ data bytes.Buffer }

func (w *shortDestination) Write(p []byte) (int, error) { return w.data.Write(p[:len(p)/2]) }
func TestWriteToReportsShortWrite(t *testing.T) {
	source := fwd.NewReaderSize(bytes.NewBufferString("0123456789"), 16)
	dst := new(shortDestination)
	n, err := source.WriteTo(dst)
	if n != 5 || !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("WriteTo=%d,%v; want5,ErrShortWrite", n, err)
	}
	var rest bytes.Buffer
	if _, err := source.WriteTo(&rest); err != nil {
		t.Fatal(err)
	}
	if dst.data.String()+rest.String() != "0123456789" {
		t.Fatal("short write discarded or duplicated buffered bytes")
	}
}

func TestWriteToSuccessfulProgress(t *testing.T) {
	source := fwd.NewReaderSize(bytes.NewBufferString("0123456789abcdefghijklmnopqrstuvwxyz"), 16)
	var dst bytes.Buffer
	n, err := source.WriteTo(&dst)
	if n != 36 || err != nil || source.InputOffset() != 36 || dst.String() != "0123456789abcdefghijklmnopqrstuvwxyz" {
		t.Fatalf("WriteTo=%d,%v offset%d data%q", n, err, source.InputOffset(), dst.String())
	}
}
