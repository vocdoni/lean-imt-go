package circuit

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/profile"
	"github.com/consensys/gnark/test"
	leanimt "github.com/vocdoni/lean-imt-go"
)

// leanIMTProofCircuit is a circuit for testing the Lean IMT Merkle proof verifier.
// This circuit verifies that a given leaf is included in a Merkle tree with a specific root.
type leanIMTProofCircuit struct {
	// Public inputs
	Root  frontend.Variable `gnark:"merkle_root,public"`
	Proof MerkleProof       `gnark:"merkle_proof,public"`
}

// newLeanIMTProofCircuit creates a new circuit instance with the specified maximum depth.
// The maxDepth parameter determines the maximum number of siblings that can be processed.
func newLeanIMTProofCircuit() *leanIMTProofCircuit {
	return &leanIMTProofCircuit{}
}

// Define implements the circuit logic for testing purposes only.
func (circuit *leanIMTProofCircuit) Define(api frontend.API) error {
	if len(circuit.Proof.Siblings) == 0 {
		return fmt.Errorf("siblings array cannot be empty for circuit compilation")
	}

	isValid, err := circuit.Proof.Verify(api, circuit.Root)
	if err != nil {
		return fmt.Errorf("proof verification failed: %w", err)
	}

	// Assert that the proof is valid (isValid should be 1)
	api.AssertIsEqual(isValid, 1)

	return nil
}

func TestLeanIMTProofCircuit(t *testing.T) {
	// Create a Lean IMT tree with some test data
	tree, err := leanimt.New(leanimt.PoseidonHasher, leanimt.BigIntEqual, nil, nil, nil)
	if err != nil {
		t.Fatalf("Failed to create tree: %v", err)
	}

	// Insert test leaves
	leaves := []*big.Int{
		big.NewInt(1),
		big.NewInt(2),
		big.NewInt(3),
		big.NewInt(4),
		big.NewInt(5),
	}

	for _, leaf := range leaves {
		if err := tree.Insert(leaf); err != nil {
			t.Fatalf("Failed to insert leaf: %v", err)
		}
	}

	// Generate proof for leaf at index 2 (value 3)
	proofIndex := 2
	proof, err := tree.GenerateProof(proofIndex)
	if err != nil {
		t.Fatalf("Failed to generate proof: %v", err)
	}

	// Verify proof using the tree's built-in verification
	if !tree.VerifyProof(proof) {
		t.Fatal("Generated proof should be valid")
	}

	// Create circuit with appropriate depth
	circuit := newLeanIMTProofCircuit()

	// Create witness assignment
	witness := &leanIMTProofCircuit{
		Root: proof.Root,
		Proof: MerkleProof{
			Leaf:     proof.Leaf,
			Index:    proof.Index,
			Siblings: [MaxCensusDepth]frontend.Variable{},
		},
	}

	// Fill siblings array
	for i, sibling := range proof.Siblings {
		witness.Proof.Siblings[i] = sibling
	}

	// Pad remaining siblings with zeros if needed
	for i := len(proof.Siblings); i < MaxCensusDepth; i++ {
		witness.Proof.Siblings[i] = big.NewInt(0)
	}

	// Test circuit satisfaction
	assert := test.NewAssert(t)
	assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

	t.Logf("Circuit test passed for proof of leaf %v at index %d", proof.Leaf, proofIndex)
	t.Logf(" Root: %v", proof.Root)
	t.Logf(" Siblings: %d", len(proof.Siblings))
}

func TestLeanIMTProofCircuitConstraints(t *testing.T) {
	// Test with different depths to show constraint scaling
	depths := []int{3, 5, 8, 10}

	for _, depth := range depths {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			// Create circuit with specified depth
			circuit := newLeanIMTProofCircuit()

			// Profile the compilation
			p := profile.Start()
			ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit)
			p.Stop()

			if err != nil {
				t.Fatalf("Failed to compile circuit: %v", err)
			}

			// Print constraint information
			fmt.Printf("\n=== Lean IMT Proof Circuit Analysis (Max Depth: %d) ===\n", MaxCensusDepth)
			fmt.Printf("Constraints: %d\n", ccs.GetNbConstraints())
			internal, secret, public := ccs.GetNbVariables()
			fmt.Printf("Variables: %d (internal: %d, secret: %d, public: %d)\n", internal+secret+public, internal, secret, public)
			fmt.Printf("Compilation Profile:\n")
			fmt.Printf("  Total Constraints: %d\n", p.NbConstraints())
			fmt.Printf("=== End Analysis ===\n\n")
		})
	}
}

// TestLeanIMTProofCircuitEdgeCases tests various edge cases
func TestLeanIMTProofCircuitEdgeCases(t *testing.T) {
	t.Run("single_leaf_tree", func(t *testing.T) {
		// Test with a single leaf tree (no siblings)
		tree, err := leanimt.New(leanimt.PoseidonHasher, leanimt.BigIntEqual, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create tree: %v", err)
		}

		// Insert single leaf
		leaf := big.NewInt(42)
		if err := tree.Insert(leaf); err != nil {
			t.Fatalf("Failed to insert leaf: %v", err)
		}

		// Generate proof
		proof, err := tree.GenerateProof(0)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		// For single leaf, root should equal leaf and no siblings
		if len(proof.Siblings) != 0 {
			t.Fatalf("Expected no siblings for single leaf tree, got %d", len(proof.Siblings))
		}

		siblings := [MaxCensusDepth]frontend.Variable{}
		for i := range MaxCensusDepth {
			siblings[i] = big.NewInt(0) // Padded
		}

		// Create circuit with minimal depth
		circuit := newLeanIMTProofCircuit()
		witness := &leanIMTProofCircuit{
			Root: proof.Root,
			Proof: MerkleProof{
				Leaf:     proof.Leaf,
				Index:    proof.Index,
				Siblings: siblings,
			},
		}

		assert := test.NewAssert(t)
		assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

		t.Log("Single leaf tree test passed")
	})

	t.Run("large_tree", func(t *testing.T) {
		// Test with a larger tree
		tree, err := leanimt.New(leanimt.PoseidonHasher, leanimt.BigIntEqual, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to create tree: %v", err)
		}

		// Insert many leaves
		numLeaves := 16
		for i := range numLeaves {
			if err := tree.Insert(big.NewInt(int64(i * 10))); err != nil {
				t.Fatalf("Failed to insert leaf %d: %v", i, err)
			}
		}

		// Test proof for various positions
		testIndices := []int{0, 7, 15} // First, middle, last
		for _, idx := range testIndices {
			proof, err := tree.GenerateProof(idx)
			if err != nil {
				t.Fatalf("Failed to generate proof for index %d: %v", idx, err)
			}

			// Create circuit with sufficient depth
			circuit := newLeanIMTProofCircuit()
			witness := &leanIMTProofCircuit{
				Root: proof.Root,
				Proof: MerkleProof{
					Leaf:     proof.Leaf,
					Index:    proof.Index,
					Siblings: [MaxCensusDepth]frontend.Variable{},
				},
			}

			// Fill siblings
			for i, sibling := range proof.Siblings {
				witness.Proof.Siblings[i] = sibling
			}
			// Pad remaining
			for i := len(proof.Siblings); i < MaxCensusDepth; i++ {
				witness.Proof.Siblings[i] = big.NewInt(0)
			}

			assert := test.NewAssert(t)
			assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

			t.Logf("Large tree test passed for index %d", idx)
		}
	})
}

// BenchmarkLeanIMTProofCircuit benchmarks circuit compilation and proving
func BenchmarkLeanIMTProofCircuit(b *testing.B) {
	depths := []int{5, 8, 10}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			circuit := newLeanIMTProofCircuit()

			b.ResetTimer()
			for b.Loop() {
				_, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit)
				if err != nil {
					b.Fatalf("Failed to compile circuit: %v", err)
				}
			}
		})
	}
}
