package circuit

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/vocdoni/gnark-crypto-primitives/hash/bn254/poseidon"
)

// VerifyLeanIMTProof verify a Lean IMT Merkle proof.
//
// Parameters:
//   - api: The frontend API for constraint operations
//   - root: The expected Merkle tree root
//   - leaf: The leaf value to verify
//   - index: Packed path bits indicating the position of the leaf
//   - siblings: Array of sibling nodes for the proof path
//
// Returns:
//   - frontend.Variable: A boolean variable (0 or 1) indicating proof validity
//   - error: Any error that occurred during compilation
func VerifyLeanIMTProof(
	api frontend.API,
	root frontend.Variable,
	leaf frontend.Variable,
	index frontend.Variable,
	siblings []frontend.Variable,
) (frontend.Variable, error) {
	// Initialize the current node with the leaf value
	currentNode := leaf

	// If no siblings, the leaf should equal the root (single-node tree)
	if len(siblings) == 0 {
		isEqual := api.IsZero(api.Sub(currentNode, root))
		return isEqual, nil
	}

	// Get all index bits at once
	indexBits := api.ToBinary(index, len(siblings))

	// Process each sibling in the proof path
	for i, sibling := range siblings {
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
	siblings []frontend.Variable,
) (frontend.Variable, error) {
	// Pack the leaf value in-circuit
	// packed = (address << 88) | weight
	shift88 := new(big.Int).Lsh(big.NewInt(1), 88)
	addressShifted := api.Mul(address, shift88)
	packedLeaf := api.Add(addressShifted, weight)

	// Verify the merkle proof with the packed leaf
	isValid, err := VerifyLeanIMTProof(api, root, packedLeaf, index, siblings)
	if err != nil {
		return frontend.Variable(0), err
	}
	return isValid, nil
}
