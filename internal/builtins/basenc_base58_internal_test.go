package builtins

import (
	"bytes"
	"math/big"
	"math/rand/v2"
	"testing"
)

// naiveBase58Encode is the digit-at-a-time reference conversion.
func naiveBase58Encode(data []byte) string {
	zeros := 0
	for zeros < len(data) && data[zeros] == 0 {
		zeros++
	}
	value := new(big.Int).SetBytes(data[zeros:])
	base, mod := big.NewInt(58), new(big.Int)
	var out []byte
	for value.Sign() > 0 {
		value.DivMod(value, base, mod)
		out = append([]byte{basencBase58Alphabet[mod.Int64()]}, out...)
	}
	return string(bytes.Repeat([]byte("1"), zeros)) + string(out)
}

func TestBase58MatchesReferenceAndRoundTrips(t *testing.T) {
	t.Parallel()

	rng := rand.New(rand.NewPCG(1, 2))
	for _, size := range []int{1, 2, 9, 10, 11, 100, 511, 512, 513, 1000, 4096, 6000} {
		data := make([]byte, size)
		for i := range data {
			data[i] = byte(rng.UintN(256))
		}
		data[0] = 0
		encoded := encodeBase58(data)
		if want := naiveBase58Encode(data); encoded != want {
			t.Fatalf("size %d: encodeBase58 differs from reference", size)
		}
		decoded, invalid := decodeBase58([]byte(encoded), false)
		if invalid || !bytes.Equal(decoded, data) {
			t.Fatalf("size %d: round trip failed (invalid=%v)", size, invalid)
		}
	}
}
