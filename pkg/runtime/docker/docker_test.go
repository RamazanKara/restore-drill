package docker

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestCopyAndCloseWriteCopiesAndClosesWriteSide(t *testing.T) {
	writer := &recordingCloseWriter{}

	if err := copyAndCloseWrite(writer, strings.NewReader("backup")); err != nil {
		t.Fatalf("copy and close write: %v", err)
	}
	if writer.body != "backup" {
		t.Fatalf("expected copied body, got %q", writer.body)
	}
	if !writer.closed {
		t.Fatal("expected write side to be closed")
	}
}

func TestCopyAndCloseWriteReturnsCopyError(t *testing.T) {
	writer := &recordingCloseWriter{}

	err := copyAndCloseWrite(writer, errReader{})
	if err == nil {
		t.Fatal("expected copy error")
	}
	if !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if !writer.closed {
		t.Fatal("expected write side to be closed after copy error")
	}
}

type recordingCloseWriter struct {
	body   string
	closed bool
}

func (w *recordingCloseWriter) Write(p []byte) (int, error) {
	w.body += string(p)
	return len(p), nil
}

func (w *recordingCloseWriter) CloseWrite() error {
	w.closed = true
	return nil
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("read failed")
}

var _ io.Reader = errReader{}
