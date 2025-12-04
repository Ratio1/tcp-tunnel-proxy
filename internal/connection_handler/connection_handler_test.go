package connectionhandler

import (
	"bytes"
	"errors"
	"testing"
)

type partialWriter struct {
	limit     int
	wrote     []byte
	failAfter int
	calls     int
}

func (p *partialWriter) Write(b []byte) (int, error) {
	p.calls++
	if p.failAfter > 0 && p.calls > p.failAfter {
		return 0, errors.New("forced failure")
	}
	n := p.limit
	if n <= 0 || n > len(b) {
		n = len(b)
	}
	p.wrote = append(p.wrote, b[:n]...)
	return n, nil
}

func TestWriteAllHandlesPartialWrites(t *testing.T) {
	payload := bytes.Repeat([]byte("a"), 1024)
	writer := &partialWriter{limit: 128}

	if err := writeAll(writer, payload); err != nil {
		t.Fatalf("writeAll returned error: %v", err)
	}
	if !bytes.Equal(writer.wrote, payload) {
		t.Fatalf("writeAll wrote %d bytes, want %d", len(writer.wrote), len(payload))
	}
	if writer.calls <= 1 {
		t.Fatalf("expected multiple writes, got %d", writer.calls)
	}
}

func TestWriteAllPropagatesErrors(t *testing.T) {
	payload := []byte("hello world")
	writer := &partialWriter{limit: 2, failAfter: 1}

	err := writeAll(writer, payload)
	if err == nil {
		t.Fatalf("expected error on second write")
	}
}
