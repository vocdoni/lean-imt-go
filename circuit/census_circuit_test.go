package circuit

import (
	"math/big"
	"testing"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/consensys/gnark/test"
	"github.com/ethereum/go-ethereum/common"
	"github.com/vocdoni/lean-imt-go/census"
)

// censusProofCircuit for testing census proof verification
type censusProofCircuit struct {
	Root     frontend.Variable   `gnark:"root,public"`
	Address  frontend.Variable   `gnark:"address,public"`
	Weight   frontend.Variable   `gnark:"weight"`
	Index    frontend.Variable   `gnark:"index"`
	Siblings []frontend.Variable `gnark:"siblings"`
}

func (circuit *censusProofCircuit) Define(api frontend.API) error {
	isValid, err := VerifyCensusProof(api, circuit.Root, circuit.Address,
		circuit.Weight, circuit.Index, circuit.Siblings)
	if err != nil {
		return err
	}

	// Assert the proof is valid
	api.AssertIsEqual(isValid, 1)
	return nil
}

func TestVerifyCensusProof(t *testing.T) {
	// Create a census with test data
	tempDir := t.TempDir()
	censusTree, err := census.NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer censusTree.Close()

	// Add test addresses
	addresses := []common.Address{
		common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7"),
		common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec"),
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
	}

	weights := []*big.Int{
		big.NewInt(100),
		big.NewInt(250),
		big.NewInt(75),
	}

	// Add addresses to census
	for i, addr := range addresses {
		if err := censusTree.Add(addr, weights[i]); err != nil {
			t.Fatalf("Failed to add address %d: %v", i, err)
		}
	}

	// Test proof verification for each address
	for i, addr := range addresses {
		t.Run("address_"+addr.Hex(), func(t *testing.T) {
			// Generate proof
			proof, err := censusTree.GenerateProof(addr)
			if err != nil {
				t.Fatalf("Failed to generate proof: %v", err)
			}

			// Create circuit with appropriate depth
			maxDepth := len(proof.Siblings)
			if maxDepth == 0 {
				maxDepth = 1 // Minimum for circuit compilation
			}

			circuit := &censusProofCircuit{
				Siblings: make([]frontend.Variable, maxDepth),
			}

			// Create witness
			witness := &censusProofCircuit{
				Root:     proof.Root,
				Address:  proof.Address.Big(),
				Weight:   proof.Weight,
				Index:    proof.Index,
				Siblings: make([]frontend.Variable, maxDepth),
			}

			// Fill siblings array
			for j, sibling := range proof.Siblings {
				witness.Siblings[j] = sibling
			}
			// Pad remaining siblings with zeros
			for j := len(proof.Siblings); j < maxDepth; j++ {
				witness.Siblings[j] = big.NewInt(0)
			}

			// Test circuit satisfaction
			assert := test.NewAssert(t)
			assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

			t.Logf("✅ Census proof verified for address %s with weight %s",
				addr.Hex(), weights[i].String())
		})
	}
}

func TestVerifyCensusProof_LargerCensus(t *testing.T) {
	// Create a larger census for more comprehensive testing
	tempDir := t.TempDir()
	censusTree, err := census.NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer censusTree.Close()

	// Add many addresses
	numAddresses := 16
	addresses := make([]common.Address, numAddresses)
	weights := make([]*big.Int, numAddresses)

	for i := 0; i < numAddresses; i++ {
		// Generate deterministic addresses for testing
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i + 1) // Ensure non-zero
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64((i + 1) * 100))

		if err := censusTree.Add(addresses[i], weights[i]); err != nil {
			t.Fatalf("Failed to add address %d: %v", i, err)
		}
	}

	// Test proofs for a few selected addresses
	testIndices := []int{0, 7, 15} // First, middle, last
	for _, idx := range testIndices {
		t.Run("large_census_index_"+string(rune('0'+idx)), func(t *testing.T) {
			addr := addresses[idx]
			proof, err := censusTree.GenerateProof(addr)
			if err != nil {
				t.Fatalf("Failed to generate proof for index %d: %v", idx, err)
			}

			// Create circuit with sufficient depth
			maxDepth := 8 // Should be enough for 16 addresses
			circuit := &censusProofCircuit{
				Siblings: make([]frontend.Variable, maxDepth),
			}

			witness := &censusProofCircuit{
				Root:     proof.Root,
				Address:  proof.Address.Big(),
				Weight:   proof.Weight,
				Index:    proof.Index,
				Siblings: make([]frontend.Variable, maxDepth),
			}

			// Fill siblings
			for i, sibling := range proof.Siblings {
				witness.Siblings[i] = sibling
			}
			// Pad remaining
			for i := len(proof.Siblings); i < maxDepth; i++ {
				witness.Siblings[i] = big.NewInt(0)
			}

			assert := test.NewAssert(t)
			assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

			t.Logf("✅ Large census proof verified for address %s at index %d",
				addr.Hex(), idx)
		})
	}
}

func TestVerifyCensusProof_EdgeCases(t *testing.T) {
	t.Run("single_address_census", func(t *testing.T) {
		// Test with single address census
		tempDir := t.TempDir()
		censusTree, err := census.NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer censusTree.Close()

		addr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
		weight := big.NewInt(1000)

		if err := censusTree.Add(addr, weight); err != nil {
			t.Fatalf("Failed to add address: %v", err)
		}

		proof, err := censusTree.GenerateProof(addr)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		// Single address should have no siblings
		maxDepth := 1 // Minimum for circuit
		circuit := &censusProofCircuit{
			Siblings: make([]frontend.Variable, maxDepth),
		}

		witness := &censusProofCircuit{
			Root:     proof.Root,
			Address:  proof.Address.Big(),
			Weight:   proof.Weight,
			Index:    proof.Index,
			Siblings: []frontend.Variable{big.NewInt(0)}, // Padded
		}

		assert := test.NewAssert(t)
		assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

		t.Log("✅ Single address census proof verified")
	})

	t.Run("max_weight", func(t *testing.T) {
		// Test with maximum allowed weight (90 bits)
		tempDir := t.TempDir()
		censusTree, err := census.NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer censusTree.Close()

		addr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
		// Max weight: 2^88 - 1 (11 bytes)
		maxWeight := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 88), big.NewInt(1))

		if err := censusTree.Add(addr, maxWeight); err != nil {
			t.Fatalf("Failed to add address: %v", err)
		}

		proof, err := censusTree.GenerateProof(addr)
		if err != nil {
			t.Fatalf("Failed to generate proof: %v", err)
		}

		circuit := &censusProofCircuit{
			Siblings: make([]frontend.Variable, 1),
		}

		witness := &censusProofCircuit{
			Root:     proof.Root,
			Address:  proof.Address.Big(),
			Weight:   proof.Weight,
			Index:    proof.Index,
			Siblings: []frontend.Variable{big.NewInt(0)},
		}

		assert := test.NewAssert(t)
		assert.SolvingSucceeded(circuit, witness, test.WithCurves(ecc.BN254), test.WithBackends(backend.GROTH16))

		t.Log("✅ Maximum weight census proof verified")
	})
}

func TestCensusProofConstraints(t *testing.T) {
	// Test constraint counting for census proofs
	depths := []int{3, 5, 8}

	for _, depth := range depths {
		t.Run("depth_"+string(rune('0'+depth)), func(t *testing.T) {
			circuit := &censusProofCircuit{
				Siblings: make([]frontend.Variable, depth),
			}

			// Compile to count constraints
			ccs, err := frontend.Compile(ecc.BN254.ScalarField(), r1cs.NewBuilder, circuit)
			if err != nil {
				t.Fatalf("Failed to compile circuit: %v", err)
			}

			constraints := ccs.GetNbConstraints()
			internal, secret, public := ccs.GetNbVariables()
			totalVars := internal + secret + public

			t.Logf("Census Proof Circuit (depth %d): %d constraints, %d variables",
				depth, constraints, totalVars)

			// Expected constraints:
			// - Merkle proof: ~247 * depth
			// - Packing: ~2 constraints
			// - Range checks: 256 constraints (160 + 96)
			// Total: ~247*depth + 258

			expectedMin := 247*depth + 200 // Allow some variance
			expectedMax := 247*depth + 300

			if constraints < expectedMin || constraints > expectedMax {
				t.Logf("Warning: Constraint count %d outside expected range [%d, %d]",
					constraints, expectedMin, expectedMax)
			}
		})
	}
}
