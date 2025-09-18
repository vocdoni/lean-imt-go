package poseidon2

import (
	"bytes"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	frposeidon2 "github.com/consensys/gnark-crypto/ecc/bn254/fr/poseidon2"
)

var (
	// TypeHashPoseidon2 identifies the Poseidon2-BN254 hash
	TypeHashPoseidon2 = []byte("poseidon2")
	// HashFunctionPoseidon2 is a ready-to-use native Go implementation
	HashFunctionPoseidon2 HashPoseidon2
	// BN254BaseField is the base field for the BN254 curve.
	BN254BaseField = fr.Modulus()
)

type HashPoseidon2 struct{}

func (HashPoseidon2) Type() []byte { return TypeHashPoseidon2 }
func (HashPoseidon2) Len() int     { return 32 }

var perm2 = frposeidon2.NewPermutation(2 /*t*/, 6 /*rF*/, 50 /*rP*/)

// HashPoseidon2 (native Go) – FINAL, fully compatible with the gnark gadget.
func (h HashPoseidon2) Hash(limbs ...[]byte) ([]byte, error) {
	if n := len(limbs); n != 2 && n != 3 {
		return nil, fmt.Errorf("poseidon2: need 2 or 3 limbs, got %d", n)
	}

	// 1. canonicalise each limb to 32-byte BE field elements
	safe := make([][]byte, len(limbs))
	for i, b := range limbs {
		safe[i] = h.SafeBigInt(new(big.Int).SetBytes(b))
	}

	// 2. internal node → order (min,max)
	if len(safe) == 2 && bytes.Compare(safe[0], safe[1]) > 0 {
		safe[0], safe[1] = safe[1], safe[0]
	}

	// 3. Merkle–Damgård chaining with width-2 Poseidon-2 (t=2,rF=6,rP=50)
	var cv fr.Element // CV₀ := 0

	for _, mB := range safe {
		var m fr.Element
		if err := m.SetBytesCanonical(mB); err != nil {
			return nil, err
		}

		st := [...]fr.Element{cv, m} // absorb one limb
		if err := perm2.Permutation(st[:]); err != nil {
			return nil, err
		}

		cv.Add(&st[1], &m) // CVᵢ₊₁ = S₁ + mᵢ
	}
	return cv.Marshal(), nil
}

func (HashPoseidon2) SafeValue(x []byte) []byte {
	return BigIntToFFwithPadding(new(big.Int).SetBytes(x), BN254BaseField)
}

func (HashPoseidon2) SafeBigInt(x *big.Int) []byte {
	return BigIntToFFwithPadding(x, BN254BaseField)
}

func BigIntToFFwithPadding(x, modulus *big.Int) []byte {
	b := BigToFF(modulus, x).Bytes()
	for len(b) < 32 {
		b = append([]byte{0}, b...)
	}
	return b
}

func BigToFF(baseField, iv *big.Int) *big.Int {
	return new(big.Int).Mod(iv, baseField)
}

