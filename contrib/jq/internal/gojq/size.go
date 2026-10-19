package gojq

import (
	"strconv"
	"strings"
)

// MaxStringBytes bounds the length of any string a jq program builds by
// repetition, concatenation, joining or encoding. Without it a short program
// such as "x" * 1e9 allocates gigabytes before the host can intervene.
const MaxStringBytes = 64 << 20

// MaxArrayLength bounds the length of any array a jq program builds by
// collection, concatenation or flattening, so collecting a huge range cannot
// exhaust host memory.
const MaxArrayLength = 1 << 22

// SizeLimitError reports a value that would exceed [MaxStringBytes] or
// [MaxArrayLength].
//
// Like [DepthLimitError], it is raised by panicking and becomes the final
// value of the iterator returned by [Code.Run], so it cannot be caught with
// try.
type SizeLimitError struct {
	Op    string
	Limit int
	Unit  string
}

func (err *SizeLimitError) Error() string {
	return err.Op + ": result exceeds size limit of " + strconv.Itoa(err.Limit) + " " + err.Unit
}

func checkStringSize(op string, n int) {
	if n > MaxStringBytes || n < 0 {
		panic(&SizeLimitError{Op: op, Limit: MaxStringBytes, Unit: "bytes"})
	}
}

func checkArrayLength(op string, n int) {
	if n > MaxArrayLength || n < 0 {
		panic(&SizeLimitError{Op: op, Limit: MaxArrayLength, Unit: "elements"})
	}
}

// sizeLimitedBuilder is a strings.Builder that refuses to grow past
// MaxStringBytes.
type sizeLimitedBuilder struct {
	sb strings.Builder
	op string
}

func (b *sizeLimitedBuilder) Write(p []byte) (int, error) {
	checkStringSize(b.op, b.sb.Len()+len(p))
	return b.sb.Write(p)
}

func (b *sizeLimitedBuilder) WriteByte(c byte) error {
	checkStringSize(b.op, b.sb.Len()+1)
	return b.sb.WriteByte(c)
}

func (b *sizeLimitedBuilder) WriteString(s string) (int, error) {
	checkStringSize(b.op, b.sb.Len()+len(s))
	return b.sb.WriteString(s)
}

func (b *sizeLimitedBuilder) String() string {
	return b.sb.String()
}
