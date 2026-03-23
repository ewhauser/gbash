package builtins

import (
	"bufio"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"math/bits"
	"math/rand"
)

var errShufRandomSourceEOF = errors.New("random source eof")

type shufRandomSource interface {
	generateAtMost(uint64) (uint64, error)
	Close() error
}

type shufDefaultRNG struct {
	rand *rand.Rand
}

type shufRNGHandle struct {
	inv          *Invocation
	randomSource string
	opener       func() (shufRandomSource, error)
	current      shufRandomSource
}

type shufFileRNG struct {
	file   io.Closer
	reader *bufio.Reader
	state  uint64
	limit  uint64
}

func (r *shufDefaultRNG) generateAtMost(atMost uint64) (uint64, error) {
	return shufGenerateAtMost(r.rand.Uint64, atMost), nil
}

func (r *shufDefaultRNG) Close() error {
	return nil
}

func (h *shufRNGHandle) generateAtMost(atMost uint64) (uint64, error) {
	if h.current == nil {
		rng, err := h.opener()
		if err != nil {
			return 0, err
		}
		h.current = rng
	}
	return h.current.generateAtMost(atMost)
}

func (h *shufRNGHandle) Close() error {
	if h.current == nil {
		return nil
	}
	return h.current.Close()
}

func (r *shufFileRNG) Close() error {
	if r.file == nil {
		return nil
	}
	return r.file.Close()
}

func (r *shufFileRNG) generateAtMost(atMost uint64) (uint64, error) {
	for r.limit < atMost {
		next, err := r.reader.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return 0, errShufRandomSourceEOF
			}
			return 0, err
		}
		r.state = r.state*256 + uint64(next)
		r.limit = r.limit*256 + 255
	}

	if atMost == math.MaxUint64 {
		value := r.state
		r.state = 0
		r.limit = 0
		return value, nil
	}

	count := atMost + 1
	nextLimit, carry := bits.Add64(r.limit, 1, 0)
	margin := nextLimit % count
	if carry != 0 {
		_, margin = bits.Div64(carry, nextLimit, count)
	}
	safeZone := r.limit - margin
	if r.state <= safeZone {
		value := r.state % count
		r.state /= count
		r.limit -= atMost
		r.limit /= count
		return value, nil
	}

	r.state %= count
	r.limit %= count
	return r.generateAtMost(atMost)
}

func shufGenerateAtMost(next func() uint64, atMost uint64) uint64 {
	if atMost == math.MaxUint64 {
		return next()
	}
	span := atMost + 1
	for {
		sample := next()
		high, low := bits.Mul64(sample, span)
		if low < span {
			threshold := (^span + 1) % span
			if low < threshold {
				continue
			}
		}
		return high
	}
}

func newShufDefaultRNG() (*shufDefaultRNG, error) {
	var seedBytes [8]byte
	if _, err := crand.Read(seedBytes[:]); err != nil {
		return nil, err
	}
	seed := int64(binary.LittleEndian.Uint64(seedBytes[:]))
	return &shufDefaultRNG{rand: rand.New(rand.NewSource(seed))}, nil
}
