// Reader interface definition and implementation.
package main

import (
	"bufio"
	"io"
)

// Reader is the bit reader interface.
type Reader interface {
	// Reader is an io.Reader
	io.Reader

	// Reader is also an io.ByteReader.
	// ReadByte reads the next 8 bits and returns them as a byte.
	io.ByteReader

	// ReadBool reads the next bit, and returns true if it is 1.
	ReadBool() (b bool, err error)

	// Align aligns the bit stream to a byte boundary,
	// so next read will read/use data from the next byte.
	// Returns the number of unread / skipped bits.
	Align() (skipped byte)
}

// An io.Reader and io.ByteReader at the same time.
type readerAndByteReader interface {
	io.Reader
	io.ByteReader
}

// reader is the bit reader implementation.
type reader struct {
	in    readerAndByteReader
	cache byte // unread bits are stored here
	bits  byte // number of unread bits in cache
}

// NewReader returns a new Reader using the specified io.Reader as the input (source).
func NewReader(in io.Reader) Reader {
	var bin readerAndByteReader
	bin, ok := in.(readerAndByteReader)
	if !ok {
		bin = bufio.NewReader(in)
	}
	return &reader{in: bin}
}

// Read implements io.Reader.
func (r *reader) Read(p []byte) (n int, err error) {
	// r.bits will be the same after reading 8 bits, so we don't need to update that.
	if r.bits == 0 {
		return r.in.Read(p)
	}

	for ; n < len(p); n++ {
		if p[n], err = r.readUnalignedByte(); err != nil {
			return
		}
	}

	return
}

// ReadByte implements io.ByteReader.
func (r *reader) ReadByte() (b byte, err error) {
	// r.bits will be the same after reading 8 bits, so we don't need to update that.
	if r.bits == 0 {
		return r.in.ReadByte()
	}
	return r.readUnalignedByte()
}

// readUnalignedByte reads the next 8 bits which are (may be) unaligned and returns them as a byte.
func (r *reader) readUnalignedByte() (b byte, err error) {
	// r.bits will be the same after reading 8 bits, so we don't need to update that.
	bits := r.bits
	b = r.cache << (8 - bits)
	r.cache, err = r.in.ReadByte()
	if err != nil {
		return 0, err
	}
	b |= r.cache >> bits
	r.cache &= 1<<bits - 1
	return
}

func (r *reader) ReadBool() (b bool, err error) {
	if r.bits == 0 {
		r.cache, err = r.in.ReadByte()
		if err != nil {
			return
		}
		r.bits = 8
	}

	r.bits--
	b = (r.cache % 2) != 0
	r.cache /= 2
	return
}

func (r *reader) Align() (skipped byte) {
	skipped = r.bits
	r.bits = 0 // no need to clear cache, will be overwritten on next read
	return
}
