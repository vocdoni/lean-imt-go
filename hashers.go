package leanimt

import (
	"crypto/sha256"
	"math/big"

	fr_bls12377 "github.com/consensys/gnark-crypto/ecc/bls12-377/fr"
	mimc_bls12_377 "github.com/consensys/gnark-crypto/ecc/bls12-377/fr/mimc"
	mimc_bn254 "github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
	iden3mimc7 "github.com/iden3/go-iden3-crypto/mimc7"
	iden3poseidon "github.com/iden3/go-iden3-crypto/poseidon"
	multiposeidon "github.com/vocdoni/davinci-node/crypto/hash/poseidon"
	"golang.org/x/crypto/blake2b"
)

// bigIntHasher is a simple hash function for *big.Int values.
// This is a deterministic, non-cryptographic hash suitable for testing.
// It uses two prime numbers to combine the inputs in a way that minimizes collisions.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
func bigIntHasher(a, b *big.Int) *big.Int {
	P1 := big.NewInt(1315423911)
	P2 := big.NewInt(2654435761)
	out := new(big.Int).Mul(a, P1)
	out.Add(out, new(big.Int).Mul(b, P2))
	return out
}

// PoseidonHasher performs Poseidon hash on two big.Int values using the iden3 implementation.
// Poseidon is a ZK-friendly cryptographic hash function optimized for use in zero-knowledge
// proof systems, particularly over the BN254 curve. It's significantly more efficient in
// circuits compared to traditional hash functions like SHA-256.
//
// This hasher is suitable for:
//   - Merkle tree constructions in ZK circuits
//   - Privacy-preserving applications
//   - Blockchain applications requiring ZK proofs
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the hash operation fails (should not happen with valid inputs)
func PoseidonHasher(a, b *big.Int) *big.Int {
	out, err := iden3poseidon.Hash([]*big.Int{a, b})
	if err != nil {
		panic(err) // Should not happen with valid inputs
	}
	return out
}

// SHA256Hasher performs SHA-256 hash on two big.Int values.
// SHA-256 is a widely-used cryptographic hash function from the SHA-2 family.
// While not optimized for zero-knowledge circuits, it provides strong security
// guarantees and is well-tested in production systems.
//
// This hasher is suitable for:
//   - General-purpose cryptographic hashing
//   - Systems requiring NIST-approved algorithms
//   - Compatibility with existing SHA-256 based systems
//
// The function converts both inputs to bytes (big-endian), concatenates them,
// and computes the SHA-256 hash. The result is interpreted as a big.Int.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
func SHA256Hasher(a, b *big.Int) *big.Int {
	// Convert inputs to bytes and concatenate
	aBytes := a.Bytes()
	bBytes := b.Bytes()
	toHash := append(aBytes, bBytes...)

	// Compute SHA-256 hash
	hash := sha256.Sum256(toHash)

	// Convert hash to big.Int
	return new(big.Int).SetBytes(hash[:])
}

// Blake2bHasher performs BLAKE2b-256 hash on two big.Int values.
// BLAKE2b is a cryptographic hash function that is faster than SHA-256 while
// providing similar security guarantees. It's optimized for 64-bit platforms
// and is widely used in modern cryptographic applications.
//
// This hasher is suitable for:
//   - High-performance hashing requirements
//   - Modern cryptographic systems
//   - Applications requiring fast, secure hashing
//
// The function converts both inputs to bytes, writes them to a BLAKE2b hasher,
// and returns the 256-bit hash result.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the BLAKE2b initialization fails
func Blake2bHasher(a, b *big.Int) *big.Int {
	// Initialize BLAKE2b-256 hasher
	hasher, err := blake2b.New256(nil)
	if err != nil {
		panic(err) // Should not happen with nil key
	}

	// Write both inputs
	aBytes := a.Bytes()
	bBytes := b.Bytes()

	if _, err := hasher.Write(aBytes); err != nil {
		panic(err)
	}
	if _, err := hasher.Write(bBytes); err != nil {
		panic(err)
	}

	// Get hash result
	hash := hasher.Sum(nil)
	return new(big.Int).SetBytes(hash)
}

// MiMCBLS12377Hasher performs MiMC hash on two big.Int values over the BLS12-377 curve.
// MiMC (Minimal Multiplicative Complexity) is a family of block ciphers and hash functions
// designed to be efficient in zero-knowledge proof systems. This variant operates over the
// scalar field of the BLS12-377 elliptic curve.
//
// This hasher is suitable for:
//   - ZK circuits using the BLS12-377 curve
//   - Applications requiring BLS12-377 compatibility
//   - Systems built with gnark using BLS12-377
//
// The function ensures inputs are reduced modulo the BLS12-377 field order before hashing.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the hash operation fails
func MiMCBLS12377Hasher(a, b *big.Int) *big.Int {
	h := mimc_bls12_377.NewMiMC()
	q := fr_bls12377.Modulus()

	// Reduce inputs modulo the field order
	aReduced := new(big.Int).Mod(a, q)
	bReduced := new(big.Int).Mod(b, q)

	// Convert to bytes (32 bytes each for BLS12-377)
	aBytes := make([]byte, 32)
	bBytes := make([]byte, 32)
	aReduced.FillBytes(aBytes)
	bReduced.FillBytes(bBytes)

	// Hash both inputs
	if _, err := h.Write(aBytes); err != nil {
		panic(err)
	}
	if _, err := h.Write(bBytes); err != nil {
		panic(err)
	}

	// Get hash result
	hash := h.Sum(nil)
	return new(big.Int).SetBytes(hash)
}

// MiMCBN254Hasher performs MiMC hash on two big.Int values over the BN254 curve.
// MiMC (Minimal Multiplicative Complexity) is a family of block ciphers and hash functions
// designed to be efficient in zero-knowledge proof systems. This variant operates over the
// scalar field of the BN254 (also known as BN128 or alt_bn128) elliptic curve.
//
// This hasher is suitable for:
//   - ZK circuits using the BN254 curve (most common in Ethereum)
//   - Applications requiring BN254 compatibility
//   - Systems built with gnark using BN254
//   - Ethereum-compatible ZK applications
//
// The function ensures inputs are reduced modulo the BN254 field order before hashing.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the hash operation fails
func MiMCBN254Hasher(a, b *big.Int) *big.Int {
	h := mimc_bn254.NewMiMC()

	// Get the BN254 field modulus
	// BN254 scalar field order
	q, _ := new(big.Int).SetString("21888242871839275222246405745257275088548364400416034343698204186575808495617", 10)

	// Reduce inputs modulo the field order
	aReduced := new(big.Int).Mod(a, q)
	bReduced := new(big.Int).Mod(b, q)

	// Convert to bytes (32 bytes each for BN254)
	aBytes := make([]byte, 32)
	bBytes := make([]byte, 32)
	aReduced.FillBytes(aBytes)
	bReduced.FillBytes(bBytes)

	// Hash both inputs
	if _, err := h.Write(aBytes); err != nil {
		panic(err)
	}
	if _, err := h.Write(bBytes); err != nil {
		panic(err)
	}

	// Get hash result
	hash := h.Sum(nil)
	return new(big.Int).SetBytes(hash)
}

// MiMC7Hasher performs MiMC-7 hash on two big.Int values using the iden3 implementation.
// MiMC-7 is a variant of the MiMC hash function with 7 rounds per block, optimized for
// zero-knowledge proof systems. This implementation is compatible with iden3's circom
// circuits and other iden3 tooling.
//
// This hasher is suitable for:
//   - Compatibility with iden3 ecosystem (circom, snarkjs)
//   - ZK applications using iden3 libraries
//   - Systems requiring MiMC-7 specifically
//
// The function operates over the BN254 scalar field and is compatible with iden3's
// circom MiMC7 implementation.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the hash operation fails
func MiMC7Hasher(a, b *big.Int) *big.Int {
	out, err := iden3mimc7.Hash([]*big.Int{a, b}, nil)
	if err != nil {
		panic(err)
	}
	return out
}

// MultiPoseidonHasher performs MultiPoseidon hash on two big.Int values.
// MultiPoseidon is Vocdoni's implementation of the Poseidon hash function that can
// efficiently handle variable-length inputs by automatically chunking them into
// field elements. This makes it particularly useful for hashing arbitrary-length data
// in ZK circuits.
//
// This hasher is suitable for:
//   - Vocdoni ecosystem applications
//   - Variable-length input hashing in ZK circuits
//   - Applications requiring efficient multi-element Poseidon hashing
//
// The function operates over the BN254 scalar field and is optimized for use in
// gnark circuits.
//
// Parameters:
//   - a: First input value
//   - b: Second input value
//
// Returns: Hash result as *big.Int
// Panics if the hash operation fails
func MultiPoseidonHasher(a, b *big.Int) *big.Int {
	out, err := multiposeidon.MultiPoseidon(a, b)
	if err != nil {
		panic(err)
	}
	return out
}

// BigIntEqual is an equality function for *big.Int values.
// This function is used by the LeanIMT to compare values for equality.
//
// Parameters:
//   - a: First value to compare
//   - b: Second value to compare
//
// Returns: true if a equals b, false otherwise
func BigIntEqual(a, b *big.Int) bool {
	return a.Cmp(b) == 0
}

// BigIntEncoder encodes a *big.Int to bytes using big-endian format.
// This function is used by the LeanIMT for persistence operations.
// It explicitly handles zero values to ensure they are properly encoded.
//
// Parameters:
//   - n: The big.Int value to encode
//
// Returns: Byte slice representation of the value, or error if encoding fails
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
// This function is used by the LeanIMT for persistence operations.
// It explicitly handles zero values to ensure they are properly decoded.
//
// Parameters:
//   - data: Byte slice to decode
//
// Returns: Decoded big.Int value, or error if decoding fails
func BigIntDecoder(data []byte) (*big.Int, error) {
	if len(data) == 0 {
		return big.NewInt(0), nil
	}
	if len(data) == 1 && data[0] == 0 {
		return big.NewInt(0), nil // Explicitly decode zero
	}
	return new(big.Int).SetBytes(data), nil
}
