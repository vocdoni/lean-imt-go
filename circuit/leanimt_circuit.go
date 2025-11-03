package circuit

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/vocdoni/gnark-crypto-primitives/hash/bn254/poseidon"
	"github.com/vocdoni/lean-imt-go/census"
)

const MaxCensusDepth = 24

type MerkleProof struct {
	Leaf     frontend.Variable                 // The leaf value to verify
	Index    frontend.Variable                 // Packed path bits indicating the position of the leaf
	Siblings [MaxCensusDepth]frontend.Variable // Array of sibling nodes for the proof path
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
		Leaf:     census.PackAddressWeight(proof.Address.Big(), proof.Weight),
		Index:    new(big.Int).SetUint64(proof.Index),
		Siblings: siblings,
	}
}

// NewMerkleProof creates a new MerkleProof instance by packing the address and
// weight into a single leaf value. This functions should be used in-circuit.
func NewMerkleProof(
	api frontend.API,
	address, weight, index frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) MerkleProof {
	return MerkleProof{
		Leaf:     PackLeaf(api, address, weight),
		Index:    index,
		Siblings: siblings,
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
	indexBits := api.ToBinary(p.Index, len(p.Siblings))
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
	index frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) (frontend.Variable, error) {
	proof := NewMerkleProof(api, address, weight, index, siblings)
	return proof.Verify(api, root)
}

// PackLeaf packs the address and weight into a single leaf value for
// in-circuit use.
func PackLeaf(api frontend.API, address, weight frontend.Variable) frontend.Variable {
	// packed = (address << 88) | weight
	shift88 := new(big.Int).Lsh(big.NewInt(1), 88)
	addressShifted := api.Mul(address, shift88)
	return api.Add(addressShifted, weight)
}
