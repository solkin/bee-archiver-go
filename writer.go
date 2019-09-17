// Writer interface definition and implementation.
package main

import (
	"bufio"
	"io"
)

// Writer is the bit writer interface.
// Must be closed in order to flush cached data.
// If you can't or don't want to close it, flushing data can also be forced
// by calling Align().
type Writer interface {
	// Writer is an io.Writer and io.Closer.
	// Close closes the bit writer, writes out cached bits.
	// It does not close the underlying io.Writer.
	io.WriteCloser

	// Writer is also an io.ByteWriter.
	// WriteByte writes 8 bits.
	io.ByteWriter

	// WriteBool writes one bit: 1 if param is true, 0 otherwise.
	WriteBool(b bool) (err error)

	// Align aligns the bit stream to a byte boundary,
	// so next write will start/go into a new byte.
	// If there are cached bits, they are first written to the output.
	// Returns the number of skipped (unset but still written) bits.
	Align() (skipped byte, err error)
}

// An io.Writer and io.ByteWriter at the same time.
type writerAndByteWriter interface {
	io.Writer
	io.ByteWriter
}

// writer is the bit writer implementation.
type writer struct {
	out       writerAndByteWriter
	wrapperbw *bufio.Writer // wrapper bufio.Writer if the target does not implement io.ByteWriter
	cache     byte          // unwritten bits are stored here
	bits      byte          // number of unwritten bits in cache
}

// NewWriter returns a new Writer using the specified io.Writer as the output.
func NewWriter(out io.Writer) Writer {
	w := &writer{}
	var ok bool
	w.out, ok = out.(writerAndByteWriter)
	if !ok {
		w.wrapperbw = bufio.NewWriter(out)
		w.out = w.wrapperbw
	}
	return w
}

// Write implements io.Writer.
func (w *writer) Write(p []byte) (n int, err error) {
	// w.bits will be the same after writing 8 bits, so we don't need to update that.
	if w.bits == 0 {
		return w.out.Write(p)
	}

	for i, b := range p {
		if err = w.writeUnalignedByte(b); err != nil {
			return i, err
		}
	}

	return len(p), nil
}

// WriteByte implements io.ByteWriter.
func (w *writer) WriteByte(b byte) (err error) {
	// w.bits will be the same after writing 8 bits, so we don't need to update that.
	if w.bits == 0 {
		return w.out.WriteByte(b)
	}
	return w.writeUnalignedByte(b)
}

// writeUnalignedByte writes 8 bits which are (may be) unaligned.
func (w *writer) writeUnalignedByte(b byte) (err error) {
	// w.bits will be the same after writing 8 bits, so we don't need to update that.
	bits := w.bits
	err = w.out.WriteByte(w.cache | b>>bits)
	if err != nil {
		return
	}
	w.cache = (b & (1<<bits - 1)) << bits
	return
}

func (w *writer) WriteBool(b bool) (err error) {
	if b {
		w.cache |= 1 << (w.bits)
	}
	w.bits++

	if w.bits == 8 {
		err = w.out.WriteByte(w.cache)
		if err != nil {
			return
		}
		w.cache, w.bits = 0, 0
		return nil
	}
	return nil
}

func (w *writer) Align() (skipped byte, err error) {
	if w.bits > 0 {
		if err = w.out.WriteByte(w.cache); err != nil {
			return
		}

		skipped = w.bits
		w.cache, w.bits = 0, 0
	}
	if w.wrapperbw != nil {
		err = w.wrapperbw.Flush()
	}
	return
}

// Close implements io.Closer.
func (w *writer) Close() (err error) {
	// Make sure cached bits are flushed:
	if _, err = w.Align(); err != nil {
		return
	}

	return nil
}
