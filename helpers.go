package leanimt

import (
	"math/big"

	p2 "github.com/vocdoni/lean-imt-go/poseidon2"
)

// bigIntHasher is a simple hash function for *big.Int values.
// This is a deterministic, non-cryptographic hash suitable for testing.
func bigIntHasher(a, b *big.Int) *big.Int {
	P1 := big.NewInt(1315423911)
	P2 := big.NewInt(2654435761)
	out := new(big.Int).Mul(a, P1)
	out.Add(out, new(big.Int).Mul(b, P2))
	return out
}

// Poseidon2Hasher is a cryptographic hash function using Poseidon2.
func Poseidon2Hasher(a, b *big.Int) *big.Int {
	out, err := p2.HashFunctionPoseidon2.Hash(
		p2.HashFunctionPoseidon2.SafeBigInt(a),
		p2.HashFunctionPoseidon2.SafeBigInt(b),
	)
	if err != nil {
		panic(err) // Should not happen with valid inputs
	}
	return new(big.Int).SetBytes(out)
}

// BigIntEqual is an equality function for *big.Int values.
func BigIntEqual(a, b *big.Int) bool {
	return a.Cmp(b) == 0
}

// BigIntEncoder encodes a *big.Int to bytes using big-endian format.
func BigIntEncoder(n *big.Int) ([]byte, error) {
	if n == nil {
		return []byte{}, nil
	}
	// Use a simple encoding that preserves zero values
	bytes := n.Bytes()
	if len(bytes) == 0 && n.Sign() == 0 {
		return []byte{0}, nil // Explicitly encode zero
	}
	return bytes, nil
}

// BigIntDecoder decodes bytes to a *big.Int.
func BigIntDecoder(data []byte) (*big.Int, error) {
	if len(data) == 0 {
		return big.NewInt(0), nil
	}
	if len(data) == 1 && data[0] == 0 {
		return big.NewInt(0), nil // Explicitly decode zero
	}
	return new(big.Int).SetBytes(data), nil
}
