package fs

import (
	"context"
	"errors"
	"io"
	stdfs "io/fs"
	"os"
	"testing"
	"time"
)

func TestMemoryFIFOReaderOpenedWhileWriterActiveSeesEOFAfterWriterCloses(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	fifo := newMemoryFIFO()
	writer := newMemoryFIFOFile(context.Background(), mem, "/pipe", fifo, os.O_WRONLY)
	reader := newMemoryFIFOFile(context.Background(), mem, "/pipe", fifo, os.O_RDONLY)
	defer func() { _ = reader.Close() }()

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	buf := make([]byte, 1)
	n, err := reader.Read(buf)
	if n != 0 {
		t.Fatalf("reader.Read() bytes = %d, want 0", n)
	}
	if !errors.Is(err, io.EOF) {
		t.Fatalf("reader.Read() error = %v, want EOF", err)
	}
}

func TestMemoryFIFOReadUnblocksOnClose(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	reader := newMemoryFIFOFile(context.Background(), mem, "/pipe", newMemoryFIFO(), os.O_RDONLY)

	done := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := reader.Read(buf)
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	if err := reader.Close(); err != nil {
		t.Fatalf("reader.Close() error = %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, stdfs.ErrClosed) {
			t.Fatalf("reader.Read() error = %v, want closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("reader.Read() remained blocked after Close()")
	}
}

func TestMemoryFIFOWriteUnblocksOnClose(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	writer := newMemoryFIFOFile(context.Background(), mem, "/pipe", newMemoryFIFO(), os.O_WRONLY)

	done := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("x"))
		done <- err
	}()

	time.Sleep(10 * time.Millisecond)
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close() error = %v", err)
	}

	select {
	case err := <-done:
		if !errors.Is(err, stdfs.ErrClosed) {
			t.Fatalf("writer.Write() error = %v, want closed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("writer.Write() remained blocked after Close()")
	}
}

func TestMemoryFIFOWriteWithoutReaderStopsAtContextDeadline(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	if err := mem.Mkfifo(context.Background(), "/p", 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	writer, err := mem.OpenFile(ctx, "/p", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(writer) error = %v", err)
	}
	defer func() { _ = writer.Close() }()

	done := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("hi\n"))
		done <- err
	}()
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Write() error = %v, want deadline exceeded", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Write() on a FIFO without a reader ignored the context deadline")
	}
}

func TestMemoryFIFOReadWithoutWriterStopsAtContextCancel(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	if err := mem.Mkfifo(context.Background(), "/p", 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader, err := mem.OpenFile(ctx, "/p", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(reader) error = %v", err)
	}
	defer func() { _ = reader.Close() }()

	done := make(chan error, 1)
	go func() {
		_, err := reader.Read(make([]byte, 8))
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("Read() returned before cancel: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Read() error = %v, want canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Read() on a FIFO without a writer ignored context cancellation")
	}
}

func TestMemoryFIFOBufferedDataStaysReadableAfterContextEnds(t *testing.T) {
	t.Parallel()

	mem := NewMemory()
	if err := mem.Mkfifo(context.Background(), "/p", 0o600); err != nil {
		t.Fatalf("Mkfifo() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader, err := mem.OpenFile(ctx, "/p", os.O_RDONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(reader) error = %v", err)
	}
	defer func() { _ = reader.Close() }()
	writer, err := mem.OpenFile(context.Background(), "/p", os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(writer) error = %v", err)
	}
	if _, err := writer.Write([]byte("hi")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	_ = writer.Close()
	cancel()

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(data) != "hi" {
		t.Fatalf("ReadAll() = %q, want %q", data, "hi")
	}
}
