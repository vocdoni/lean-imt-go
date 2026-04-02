package circuit

import (
	"fmt"
	"math/big"

	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/std/math/bits"
	"github.com/consensys/gnark/std/math/cmp"
	"github.com/vocdoni/gnark-crypto-primitives/hash/native/bn254/poseidon"
	"github.com/vocdoni/lean-imt-go/census"
)

const MaxCensusDepth = 24

type MerkleProof struct {
	Leaf       frontend.Variable                 // The leaf value to verify
	PathBits   frontend.Variable                 // Packed per-level path bits
	LeafIndex  frontend.Variable                 // Absolute leaf position in the level-0 leaves
	TreeSize   frontend.Variable                 // Number of leaves when the proof was generated
	LevelsMask frontend.Variable                 // Packed per-level mask of levels that have siblings
	Siblings   [MaxCensusDepth]frontend.Variable // Array of sibling nodes for the proof path
}

func alignProof(pathBits, leafIndex, treeSize uint64, proofSiblings []*big.Int) (uint64, uint64, [MaxCensusDepth]frontend.Variable) {
	siblings := [MaxCensusDepth]frontend.Variable{}
	var alignedPathBits uint64
	var levelMask uint64
	siblingIdx := 0
	index := leafIndex
	size := treeSize

	for level := range MaxCensusDepth {
		if size <= 1 {
			siblings[level] = big.NewInt(0)
			continue
		}

		isRight := (index & 1) == 1
		haveSibling := isRight || index+1 < size
		if haveSibling {
			levelMask |= 1 << uint(level)
			if ((pathBits >> uint(siblingIdx)) & 1) == 1 {
				alignedPathBits |= 1 << uint(level)
			}
			if siblingIdx < len(proofSiblings) {
				siblings[level] = proofSiblings[siblingIdx]
			} else {
				siblings[level] = big.NewInt(0)
			}
			siblingIdx++
		} else {
			siblings[level] = big.NewInt(0)
		}

		index >>= 1
		size = (size + 1) >> 1
	}

	return alignedPathBits, levelMask, siblings
}

// CensusProofToMerkleProof converts a census.CensusProof to a MerkleProof
// suitable for in-circuit verification. It packs the address and weight into
// a single leaf value and pads the siblings array to MaxCensusDepth.
func CensusProofToMerkleProof(proof *census.CensusProof) MerkleProof {
	pathBits, levelMask, siblings := alignProof(proof.PathBits, proof.AddressIndex, proof.TreeSize, proof.Siblings)

	return MerkleProof{
		Leaf:       census.PackAddressWeight(proof.Address.Big(), proof.Weight),
		PathBits:   new(big.Int).SetUint64(pathBits),
		LeafIndex:  new(big.Int).SetUint64(proof.AddressIndex),
		TreeSize:   new(big.Int).SetUint64(proof.TreeSize),
		LevelsMask: new(big.Int).SetUint64(levelMask),
		Siblings:   siblings,
	}
}

// NewMerkleProof creates a new MerkleProof instance by packing the address and
// weight into a single leaf value. This functions should be used in-circuit.
func NewMerkleProof(
	api frontend.API,
	address, weight, pathBits, leafIndex, treeSize, levelsMask frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) MerkleProof {
	return MerkleProof{
		Leaf:       PackLeaf(api, address, weight),
		PathBits:   pathBits,
		LeafIndex:  leafIndex,
		TreeSize:   treeSize,
		LevelsMask: levelsMask,
		Siblings:   siblings,
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
	api.AssertIsLessOrEqual(1, p.TreeSize)
	api.AssertIsLessOrEqual(api.Add(p.LeafIndex, 1), p.TreeSize)

	pathBits := api.ToBinary(p.PathBits, MaxCensusDepth)
	levelMaskBits := api.ToBinary(p.LevelsMask, MaxCensusDepth)

	currentNode := p.Leaf
	currentIndex := p.LeafIndex
	currentSize := p.TreeSize
	expectedMask := frontend.Variable(0)

	for i, sibling := range p.Siblings {
		idxBits := api.ToBinary(currentIndex, MaxCensusDepth)
		sizeBits := api.ToBinary(currentSize, MaxCensusDepth+1)

		isRight := idxBits[0]
		nextIndex := bits.FromBinary(api, idxBits[1:])
		sizeHalfFloor := bits.FromBinary(api, sizeBits[1:])
		nextSize := api.Add(sizeHalfFloor, sizeBits[0]) // ceil(currentSize / 2)

		currentSizeGtOne := cmp.IsLess(api, 1, currentSize)
		hasRightSibling := cmp.IsLess(api, api.Add(currentIndex, 1), currentSize)
		hasSibling := api.And(currentSizeGtOne, api.Or(isRight, hasRightSibling))

		expectedMask = api.Add(expectedMask, api.Mul(hasSibling, new(big.Int).Lsh(big.NewInt(1), uint(i))))
		api.AssertIsEqual(levelMaskBits[i], hasSibling)
		api.AssertIsEqual(api.Mul(pathBits[i], api.Sub(1, hasSibling)), 0)

		bit := pathBits[i]
		leftInput := api.Select(bit, sibling, currentNode)
		rightInput := api.Select(bit, currentNode, sibling)
		hashedValue, err := poseidon.Hash(api, leftInput, rightInput)
		if err != nil {
			return frontend.Variable(0), fmt.Errorf("failed to hash nodes: %w", err)
		}
		currentNode = api.Select(hasSibling, hashedValue, currentNode)
		currentIndex = nextIndex
		currentSize = nextSize
	}

	api.AssertIsEqual(expectedMask, p.LevelsMask)
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
	treeSize frontend.Variable,
	levelsMask frontend.Variable,
	siblings [MaxCensusDepth]frontend.Variable,
) (frontend.Variable, error) {
	proof := NewMerkleProof(api, address, weight, pathBits, leafIndex, treeSize, levelsMask, siblings)
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
