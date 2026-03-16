package circuit

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/vocdoni/gnark-crypto-primitives/hash/native/bn254/poseidon"
	"github.com/vocdoni/lean-imt-go/census"
)

const MaxCensusDepth = 24

type MerkleProof struct {
	Leaf      frontend.Variable                 // The leaf value to verify
	PathBits  frontend.Variable                 // Packed path bits indicating the position of the leaf
	LeafIndex frontend.Variable                 // Absolute leaf position in the level-0 leaves
	Siblings  [MaxCensusDepth]frontend.Variable // Array of sibling nodes for the proof path
}

// CensusProofToMerkleProof converts a census.CensusProof to a MerkleProof
// suitable for in-circuit verification. It packs the address and weight into
// a single leaf value and pads the siblings array to MaxCensusDepth.
func CensusProofToMerkleProof(proof *census.CensusProof) MerkleProof {
	siblings := [MaxCensusDepth]frontend.Variable{}
	for i := range MaxCensusDepth {
		if i < len(proof.Siblings) {
			siblings[i] = proof.Siblings[i]
		} else {
			siblings[i] = big.NewInt(0) // Padding with zeros
		}
	}
	return MerkleProof{
		Leaf:      census.PackAddressWeight(proof.Address.Big(), proof.Weight),
		PathBits:  new(big.Int).SetUint64(proof.PathBits),
		LeafIndex: new(big.Int).SetUint64(proof.AddressIndex),
		Siblings:  siblings,
	}
}

// NewMerkleProof creates a new MerkleProof instance by packing the address and
// weight into a single leaf value. This functions should be used in-circuit.
func NewMerkleProof(
	api frontend.API,
	address, weight, pathBits, leafIndex frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) MerkleProof {
	return MerkleProof{
		Leaf:      PackLeaf(api, address, weight),
		PathBits:  pathBits,
		LeafIndex: leafIndex,
		Siblings:  siblings,
	}
}

// Verify method verifies a Lean IMT Merkle proof. It uses the leaf, index and
// siblings included in the MerkleProof struct to compute the root and compares
// it with the provided root.
//
// Parameters:
//   - api: The frontend API for constraint operations
//   - root: The expected Merkle tree root
//
// Returns:
//   - frontend.Variable: A boolean variable (0 or 1) indicating proof validity.
//   - error: Any error that occurred during compilation.
func (p MerkleProof) Verify(api frontend.API, root frontend.Variable) (frontend.Variable, error) {
	// Initialize the current node with the leaf value
	currentNode := p.Leaf
	// If no siblings, the leaf should equal the root (single-node tree)
	if len(p.Siblings) == 0 {
		isEqual := api.IsZero(api.Sub(currentNode, root))
		return isEqual, nil
	}
	// Get all index bits at once
	indexBits := api.ToBinary(p.PathBits, len(p.Siblings))
	// Process each sibling in the proof path
	for i, sibling := range p.Siblings {
		// Check if this sibling is actually used (non-zero)
		// For padding zeros, we skip the hashing
		isNonZero := api.Sub(1, api.IsZero(sibling))
		// Extract the i-th bit from the index to determine position
		bit := indexBits[i]
		// Compute hash based on position
		leftInput := api.Select(bit, sibling, currentNode)
		rightInput := api.Select(bit, currentNode, sibling)
		// Hash the two inputs using Poseidon
		hashedValue, err := poseidon.Hash(api, leftInput, rightInput)
		if err != nil {
			return frontend.Variable(0), fmt.Errorf("failed to hash nodes: %w", err)
		}
		// Only update currentNode if sibling is non-zero (not padding)
		currentNode = api.Select(isNonZero, hashedValue, currentNode)
	}
	// Return 1 if roots match, 0 otherwise
	isEqual := api.IsZero(api.Sub(currentNode, root))
	return isEqual, nil
}

// VerifyCensusProof verifies a census membership proof in-circuit
// This function packs the address and weight, then verifies the merkle proof
//
// Parameters:
//   - api: The frontend API for constraint operations
//   - root: The merkle root
//   - address: The voter's address as big.Int
//   - weight: The voting weight
//   - index: The tree index
//   - siblings: The merkle siblings
//
// Returns:
//   - frontend.Variable: 1 if proof is valid, 0 otherwise
//   - error: Any error that occurred during compilation
func VerifyCensusProof(
	api frontend.API,
	root frontend.Variable,
	address frontend.Variable,
	weight frontend.Variable,
	pathBits frontend.Variable,
	leafIndex frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) (frontend.Variable, error) {
	proof := NewMerkleProof(api, address, weight, pathBits, leafIndex, siblings)
	return proof.Verify(api, root)
}

// PackLeaf packs a canonical (address, weight) pair into one field element.
// It enforces address < 2^160 and weight < 2^88 so the packed value has a
// unique representation.
//
//	packed = address * 2^88 + weight
func PackLeaf(api frontend.API, address, weight frontend.Variable) frontend.Variable {
	// api.ToBinary fails if value is bigger than n bits,
	// effectively range-constraining address and weight.
	// This is to prevent leaf spoofing by ensuring that the packed value
	// is a unique representation of the (address, weight) pair.
	api.ToBinary(address, 160) // 20 bytes for Ethereum address
	api.ToBinary(weight, 88)

	shift88 := new(big.Int).Lsh(big.NewInt(1), 88)
	addressShifted := api.Mul(address, shift88)
	return api.Add(addressShifted, weight)
}
