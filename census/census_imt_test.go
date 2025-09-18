package census

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestCensusIMT_Basic(t *testing.T) {
	// Create census with temporary database
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer census.Close()

	// Test empty census
	if census.Size() != 0 {
		t.Errorf("Expected empty census, got size %d", census.Size())
	}

	// Add some addresses
	addr1 := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
	addr2 := common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec")
	addr3 := common.HexToAddress("0x1234567890123456789012345678901234567890")

	weight1 := big.NewInt(100)
	weight2 := big.NewInt(250)
	weight3 := big.NewInt(75)

	// Add addresses
	if err := census.Add(addr1, weight1); err != nil {
		t.Fatalf("Failed to add addr1: %v", err)
	}

	if err := census.Add(addr2, weight2); err != nil {
		t.Fatalf("Failed to add addr2: %v", err)
	}

	if err := census.Add(addr3, weight3); err != nil {
		t.Fatalf("Failed to add addr3: %v", err)
	}

	// Check size
	if census.Size() != 3 {
		t.Errorf("Expected census size 3, got %d", census.Size())
	}

	// Check Has
	if !census.Has(addr1) {
		t.Error("addr1 should exist in census")
	}
	if !census.Has(addr2) {
		t.Error("addr2 should exist in census")
	}
	if !census.Has(addr3) {
		t.Error("addr3 should exist in census")
	}

	// Check non-existent address
	nonExistent := common.HexToAddress("0x0000000000000000000000000000000000000000")
	if census.Has(nonExistent) {
		t.Error("non-existent address should not be in census")
	}

	// Check weights
	if weight, exists := census.GetWeight(addr1); !exists || weight.Cmp(weight1) != 0 {
		t.Errorf("Expected weight %s for addr1, got %s (exists: %t)", weight1, weight, exists)
	}

	if weight, exists := census.GetWeight(addr2); !exists || weight.Cmp(weight2) != 0 {
		t.Errorf("Expected weight %s for addr2, got %s (exists: %t)", weight2, weight, exists)
	}

	// Test duplicate address
	if err := census.Add(addr1, big.NewInt(999)); err != ErrAddressAlreadyExists {
		t.Errorf("Expected ErrAddressAlreadyExists, got %v", err)
	}

	// Check root exists
	if root, exists := census.Root(); !exists {
		t.Error("Census should have a root")
	} else if root == nil {
		t.Error("Root should not be nil")
	}
}

func TestCensusIMT_Proofs(t *testing.T) {
	// Create census with temporary database
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer census.Close()

	// Add test addresses
	addresses := []common.Address{
		common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7"),
		common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec"),
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		common.HexToAddress("0x9876543210987654321098765432109876543210"),
	}

	weights := []*big.Int{
		big.NewInt(100),
		big.NewInt(250),
		big.NewInt(75),
		big.NewInt(500),
		big.NewInt(33),
	}

	// Add all addresses
	for i, addr := range addresses {
		if err := census.Add(addr, weights[i]); err != nil {
			t.Fatalf("Failed to add address %d: %v", i, err)
		}
	}

	// Generate proofs for all addresses
	for i, addr := range addresses {
		proof, err := census.GenerateProof(addr)
		if err != nil {
			t.Fatalf("Failed to generate proof for address %d: %v", i, err)
		}

		// Verify proof structure
		if proof.Address != addr {
			t.Errorf("Proof address mismatch for address %d", i)
		}

		if proof.Weight.Cmp(weights[i]) != 0 {
			t.Errorf("Proof weight mismatch for address %d: expected %s, got %s",
				i, weights[i], proof.Weight)
		}

		if proof.Root == nil {
			t.Errorf("Proof root is nil for address %d", i)
		}

		if len(proof.Siblings) == 0 && census.Size() > 1 {
			t.Errorf("Expected siblings for address %d in multi-member census", i)
		}

		t.Logf("✅ Proof generated for address %s with weight %s",
			addr.Hex(), weights[i].String())
	}

	// Test proof for non-existent address
	nonExistent := common.HexToAddress("0x0000000000000000000000000000000000000000")
	if _, err := census.GenerateProof(nonExistent); err != ErrAddressNotFound {
		t.Errorf("Expected ErrAddressNotFound for non-existent address, got %v", err)
	}
}

func TestCensusIMT_Persistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create census with persistence
	census1, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create persistent census: %v", err)
	}

	// Add test data
	addr1 := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
	addr2 := common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec")
	weight1 := big.NewInt(100)
	weight2 := big.NewInt(250)

	if err := census1.Add(addr1, weight1); err != nil {
		t.Fatalf("Failed to add addr1: %v", err)
	}

	if err := census1.Add(addr2, weight2); err != nil {
		t.Fatalf("Failed to add addr2: %v", err)
	}

	// Get root before closing
	root1, exists1 := census1.Root()
	if !exists1 {
		t.Fatal("Census should have a root")
	}

	// Close the census
	if err := census1.Close(); err != nil {
		t.Fatalf("Failed to close census: %v", err)
	}

	// Reopen the census
	census2, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to reopen persistent census: %v", err)
	}
	defer census2.Close()

	// Verify data was persisted
	if census2.Size() != 2 {
		t.Errorf("Expected size 2 after reopening, got %d", census2.Size())
	}

	if !census2.Has(addr1) {
		t.Error("addr1 should exist after reopening")
	}

	if !census2.Has(addr2) {
		t.Error("addr2 should exist after reopening")
	}

	// Check weights
	if weight, exists := census2.GetWeight(addr1); !exists || weight.Cmp(weight1) != 0 {
		t.Errorf("Weight mismatch for addr1 after reopening")
	}

	if weight, exists := census2.GetWeight(addr2); !exists || weight.Cmp(weight2) != 0 {
		t.Errorf("Weight mismatch for addr2 after reopening")
	}

	// Check root is the same
	root2, exists2 := census2.Root()
	if !exists2 {
		t.Fatal("Reopened census should have a root")
	}

	if root1.Cmp(root2) != 0 {
		t.Errorf("Root mismatch after reopening: %s vs %s", root1, root2)
	}

	// Generate proofs to ensure everything works
	proof1, err := census2.GenerateProof(addr1)
	if err != nil {
		t.Fatalf("Failed to generate proof after reopening: %v", err)
	}

	if proof1.Weight.Cmp(weight1) != 0 {
		t.Errorf("Proof weight mismatch after reopening")
	}

	t.Log("✅ Persistence test passed")
}

func TestPackUnpackAddressWeight(t *testing.T) {
	// Test cases
	testCases := []struct {
		address *big.Int
		weight  *big.Int
		name    string
	}{
		{
			name:    "small values",
			address: big.NewInt(0x1234),
			weight:  big.NewInt(100),
		},
		{
			name:    "max address",
			address: new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 160), big.NewInt(1)), // 2^160 - 1
			weight:  big.NewInt(1),
		},
		{
			name:    "max weight",
			address: big.NewInt(1),
			weight:  new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 88), big.NewInt(1)), // 2^88 - 1
		},
		{
			name:    "ethereum address",
			address: common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7").Big(),
			weight:  big.NewInt(1000000),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Pack
			packed := packAddressWeight(tc.address, tc.weight)

			// Unpack
			unpackedAddr, unpackedWeight := unpackAddressWeight(packed)

			// Verify
			if tc.address.Cmp(unpackedAddr) != 0 {
				t.Errorf("Address mismatch: expected %s, got %s", tc.address, unpackedAddr)
			}

			if tc.weight.Cmp(unpackedWeight) != 0 {
				t.Errorf("Weight mismatch: expected %s, got %s", tc.weight, unpackedWeight)
			}
		})
	}
}

func TestPackAddressWeight_Panics(t *testing.T) {
	// Test address too large
	t.Run("address_too_large", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for address too large")
			}
		}()

		largeAddr := new(big.Int).Lsh(big.NewInt(1), 161) // 2^161
		packAddressWeight(largeAddr, big.NewInt(1))
	})

	// Test weight too large
	t.Run("weight_too_large", func(t *testing.T) {
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic for weight too large")
			}
		}()

		largeWeight := new(big.Int).Lsh(big.NewInt(1), 97) // 2^97
		packAddressWeight(big.NewInt(1), largeWeight)
	})
}

func TestCensusIMT_AddBulk(t *testing.T) {
	// Create census with temporary database
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer census.Close()

	// Prepare test data
	addresses := []common.Address{
		common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7"),
		common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec"),
		common.HexToAddress("0x1234567890123456789012345678901234567890"),
		common.HexToAddress("0xabcdefabcdefabcdefabcdefabcdefabcdefabcd"),
		common.HexToAddress("0x9876543210987654321098765432109876543210"),
	}

	weights := []*big.Int{
		big.NewInt(100),
		big.NewInt(250),
		big.NewInt(75),
		big.NewInt(500),
		big.NewInt(33),
	}

	// Test bulk add
	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	// Verify all addresses were added
	if census.Size() != len(addresses) {
		t.Errorf("Expected census size %d, got %d", len(addresses), census.Size())
	}

	// Verify each address exists and has correct weight
	for i, addr := range addresses {
		if !census.Has(addr) {
			t.Errorf("Address %d should exist in census", i)
		}

		weight, exists := census.GetWeight(addr)
		if !exists {
			t.Errorf("Weight should exist for address %d", i)
		}

		if weight.Cmp(weights[i]) != 0 {
			t.Errorf("Weight mismatch for address %d: expected %s, got %s",
				i, weights[i], weight)
		}
	}

	// Verify proofs can be generated for all addresses
	for i, addr := range addresses {
		proof, err := census.GenerateProof(addr)
		if err != nil {
			t.Fatalf("Failed to generate proof for bulk-added address %d: %v", i, err)
		}

		if proof.Weight.Cmp(weights[i]) != 0 {
			t.Errorf("Proof weight mismatch for address %d", i)
		}

		t.Logf("✅ Bulk-added address %s verified with weight %s",
			addr.Hex(), weights[i].String())
	}
}

func TestCensusIMT_AddBulk_EdgeCases(t *testing.T) {
	t.Run("empty_bulk_add", func(t *testing.T) {
		tempDir := t.TempDir()
		census, err := NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer census.Close()

		// Empty bulk add should succeed
		if err := census.AddBulk([]common.Address{}, []*big.Int{}); err != nil {
			t.Errorf("Empty bulk add should succeed: %v", err)
		}

		if census.Size() != 0 {
			t.Errorf("Census should remain empty after empty bulk add")
		}
	})

	t.Run("mismatched_lengths", func(t *testing.T) {
		tempDir := t.TempDir()
		census, err := NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer census.Close()

		addresses := []common.Address{
			common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7"),
		}
		weights := []*big.Int{
			big.NewInt(100),
			big.NewInt(200), // Extra weight
		}

		err = census.AddBulk(addresses, weights)
		if err == nil {
			t.Error("Expected error for mismatched slice lengths")
		}
	})

	t.Run("duplicate_address_in_bulk", func(t *testing.T) {
		tempDir := t.TempDir()
		census, err := NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer census.Close()

		// Add an address first
		addr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
		if err := census.Add(addr, big.NewInt(100)); err != nil {
			t.Fatalf("Failed to add initial address: %v", err)
		}

		// Try to bulk add the same address
		addresses := []common.Address{addr}
		weights := []*big.Int{big.NewInt(200)}

		err = census.AddBulk(addresses, weights)
		if err == nil {
			t.Error("Expected error for duplicate address in bulk add")
		}

		// Census should remain unchanged
		if census.Size() != 1 {
			t.Errorf("Census size should remain 1 after failed bulk add")
		}
	})

	t.Run("single_address_bulk", func(t *testing.T) {
		tempDir := t.TempDir()
		census, err := NewCensusIMTWithPebble(tempDir)
		if err != nil {
			t.Fatalf("Failed to create census: %v", err)
		}
		defer census.Close()

		// Bulk add single address
		addresses := []common.Address{
			common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7"),
		}
		weights := []*big.Int{big.NewInt(1000)}

		if err := census.AddBulk(addresses, weights); err != nil {
			t.Fatalf("Failed to bulk add single address: %v", err)
		}

		if census.Size() != 1 {
			t.Errorf("Expected census size 1, got %d", census.Size())
		}

		if !census.Has(addresses[0]) {
			t.Error("Single bulk-added address should exist")
		}
	})
}

func TestCensusIMT_AddBulk_Persistence(t *testing.T) {
	tempDir := t.TempDir()

	// Create census and bulk add data
	census1, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}

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

	// Bulk add addresses
	if err := census1.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to bulk add addresses: %v", err)
	}

	// Get root before closing
	root1, exists1 := census1.Root()
	if !exists1 {
		t.Fatal("Census should have a root")
	}

	// Close the census
	if err := census1.Close(); err != nil {
		t.Fatalf("Failed to close census: %v", err)
	}

	// Reopen the census
	census2, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to reopen census: %v", err)
	}
	defer census2.Close()

	// Verify all bulk-added data was persisted
	if census2.Size() != len(addresses) {
		t.Errorf("Expected size %d after reopening, got %d", len(addresses), census2.Size())
	}

	for i, addr := range addresses {
		if !census2.Has(addr) {
			t.Errorf("Bulk-added address %d should exist after reopening", i)
		}

		weight, exists := census2.GetWeight(addr)
		if !exists || weight.Cmp(weights[i]) != 0 {
			t.Errorf("Weight mismatch for bulk-added address %d after reopening", i)
		}
	}

	// Check root is the same
	root2, exists2 := census2.Root()
	if !exists2 {
		t.Fatal("Reopened census should have a root")
	}

	if root1.Cmp(root2) != 0 {
		t.Errorf("Root mismatch after reopening: %s vs %s", root1, root2)
	}

	t.Log("✅ Bulk add persistence test passed")
}

func TestCensusIMT_AddBulk_Performance(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer census.Close()

	// Generate large number of addresses for performance test
	numAddresses := 1000
	addresses := make([]common.Address, numAddresses)
	weights := make([]*big.Int, numAddresses)

	for i := 0; i < numAddresses; i++ {
		// Generate deterministic addresses
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i >> 8)
		addrBytes[1] = byte(i & 0xff)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	// Bulk add all addresses
	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to bulk add %d addresses: %v", numAddresses, err)
	}

	// Verify final state
	if census.Size() != numAddresses {
		t.Errorf("Expected census size %d, got %d", numAddresses, census.Size())
	}

	// Spot check a few addresses
	testIndices := []int{0, numAddresses / 2, numAddresses - 1}
	for _, idx := range testIndices {
		if !census.Has(addresses[idx]) {
			t.Errorf("Address %d should exist after bulk add", idx)
		}

		weight, exists := census.GetWeight(addresses[idx])
		if !exists || weight.Cmp(weights[idx]) != 0 {
			t.Errorf("Weight mismatch for address %d", idx)
		}
	}

	t.Logf("✅ Successfully bulk added %d addresses", numAddresses)
}
