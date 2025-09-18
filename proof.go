package leanimt

import (
	"errors"
	"reflect"
)

// MerkleProof contains the fields needed to verify membership:
// - Root: root at the time of proof
// - Leaf: the leaf value
// - Index: packed path bits (LSB is first sibling combined)
// - Siblings: the sibling nodes included (missing siblings are omitted)
type MerkleProof[N any] struct {
	Root     N
	Leaf     N
	Index    uint64
	Siblings []N
}

// GenerateProof builds a LeanIMT proof for the leaf at index.
func (t *LeanIMT[N]) GenerateProof(index int) (MerkleProof[N], error) {
	var empty MerkleProof[N]

	if index < 0 || index >= t.Size() {
		return empty, errLeafOutOfRange(index)
	}

	leaf := t.nodes[0][index]
	siblings := make([]N, 0, t.Depth())
	// collect path bits, but we only need to know which levels were included
	// we'll compute Index from the decisions in reverse later
	pathBits := make([]uint8, 0, t.Depth())

	for level := 0; level < t.Depth(); level++ {
		isRight := (index & 1) == 1
		var haveSibling bool
		var sibling N

		if isRight {
			// left sibling must exist (since current node exists)
			sibling = t.nodes[level][index-1]
			haveSibling = true
		} else {
			ri := index + 1
			if ri < len(t.nodes[level]) {
				sibling = t.nodes[level][ri]
				haveSibling = true
			}
		}

		if haveSibling {
			if isRight {
				pathBits = append(pathBits, 1)
			} else {
				pathBits = append(pathBits, 0)
			}
			siblings = append(siblings, sibling)
		}
		index >>= 1
	}

	// Compute Index by packing the path bits (LSB first).
	var packed uint64
	for i := 0; i < len(pathBits); i++ {
		if pathBits[i] == 1 {
			packed |= 1 << uint(i)
		}
	}

	root, _ := t.Root()
	return MerkleProof[N]{
		Root:     root,
		Leaf:     leaf,
		Index:    packed,
		Siblings: siblings,
	}, nil
}

// VerifyProof verifies a proof against the current tree hash function.
func (t *LeanIMT[N]) VerifyProof(proof MerkleProof[N]) bool {
	return VerifyProofWith(proof, t.hash, t.equal)
}

// VerifyProofWith verifies a proof using the provided hash and equality functions.
func VerifyProofWith[N any](proof MerkleProof[N], hash Hasher[N], eq Equal[N]) bool {
	if hash == nil {
		return false
	}
	node := proof.Leaf
	for i := 0; i < len(proof.Siblings); i++ {
		if ((proof.Index >> uint(i)) & 1) == 1 {
			node = hash(proof.Siblings[i], node)
		} else {
			node = hash(node, proof.Siblings[i])
		}
	}
	if eq != nil {
		return eq(node, proof.Root)
	}
	// fallback
	return reflect.DeepEqual(node, proof.Root)
}

// errLeafOutOfRange returns an error for out-of-range leaf index.
func errLeafOutOfRange(index int) error {
	return errors.New("leaf index " + intToString(index) + " is out of range")
}
