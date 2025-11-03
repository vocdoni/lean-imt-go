package census

import (
	"math/big"
)

// PackAddressWeight packs address (160 bits) and weight (88 bits) into single big.Int
// Layout: [address (160 bits)] [weight (88 bits)] = 248 bits total (fits safely in BN254 field ~254 bits)
func PackAddressWeight(address, weight *big.Int) *big.Int {
	if address.BitLen() > 160 {
		panic("address exceeds 160 bits")
	}
	if weight.BitLen() > 88 {
		panic("weight exceeds 88 bits (11 bytes)")
	}

	// Shift address left by 88 bits and OR with weight
	packed := new(big.Int).Lsh(address, 88)
	return packed.Or(packed, weight)
}

// UnpackAddressWeight unpacks a composite value into address and weight
// This is used internally for verification and debugging
func UnpackAddressWeight(packed *big.Int) (address, weight *big.Int) {
	// Create mask for lower 88 bits
	weightMask := new(big.Int).Sub(
		new(big.Int).Lsh(big.NewInt(1), 88),
		big.NewInt(1),
	)

	// Extract weight (lower 88 bits)
	weight = new(big.Int).And(packed, weightMask)

	// Extract address (upper bits, shifted right by 88)
	address = new(big.Int).Rsh(packed, 88)

	return address, weight
}
