package census

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sync"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	leanimt "github.com/vocdoni/lean-imt-go"
)

func TestCensusIMT_Dump_Empty(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	reader := census.Dump()
	decoder := json.NewDecoder(reader)

	if decoder.More() {
		t.Error("Empty census should produce no entries")
	}
}

func TestCensusIMT_Dump_Small(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

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

	for i, addr := range addresses {
		if err := census.Add(addr, weights[i]); err != nil {
			t.Fatalf("Failed to add address %d: %v", i, err)
		}
	}

	reader := census.Dump()
	decoder := json.NewDecoder(reader)

	entries := make(map[string]*CensusParticipant)
	for decoder.More() {
		var entry CensusParticipant
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("Failed to decode entry: %v", err)
		}
		entries[entry.Address.Hex()] = &entry
	}

	if len(entries) != len(addresses) {
		t.Errorf("Expected %d entries, got %d", len(addresses), len(entries))
	}

	for i, addr := range addresses {
		hexAddr := addr.Hex()
		entry, exists := entries[hexAddr]
		if !exists {
			t.Errorf("Address %s not found in dump", hexAddr)
			continue
		}
		if entry.Weight.Cmp(weights[i]) != 0 {
			t.Errorf("Weight mismatch for %s: expected %s, got %s", hexAddr, weights[i].String(), entry.Weight.String())
		}
		if entry.Index != uint64(i) {
			t.Errorf("Index mismatch for %s: expected %d, got %d", hexAddr, i, entry.Index)
		}
	}
}

func TestCensusIMT_DumpRange_SmallRange(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numEntries := 100
	addresses := make([]common.Address, numEntries)
	weights := make([]*big.Int, numEntries)

	for i := 0; i < numEntries; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i >> 8)
		addrBytes[1] = byte(i & 0xff)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	reader := census.DumpRange(10, 20)
	decoder := json.NewDecoder(reader)

	count := 0
	for decoder.More() {
		var entry CensusParticipant
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("Failed to decode entry: %v", err)
		}
		count++

		expectedAddr := addresses[10+count-1]
		if entry.Address != expectedAddr {
			t.Errorf("Entry %d: expected address %s, got %s", count, expectedAddr.Hex(), entry.Address.Hex())
		}

		expectedWeight := weights[10+count-1]
		if entry.Weight.Cmp(expectedWeight) != 0 {
			t.Errorf("Entry %d: expected weight %s, got %s", count, expectedWeight.String(), entry.Weight.String())
		}
	}

	if count != 20 {
		t.Errorf("Expected 20 entries, got %d", count)
	}
}

func TestCensusIMT_DumpRange_LargeRange(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numEntries := 15000
	addresses := make([]common.Address, numEntries)
	weights := make([]*big.Int, numEntries)

	for i := 0; i < numEntries; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i >> 8)
		addrBytes[1] = byte(i & 0xff)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	reader := census.DumpRange(0, 15000)
	decoder := json.NewDecoder(reader)

	count := 0
	for decoder.More() {
		var entry CensusParticipant
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("Failed to decode entry: %v", err)
		}
		count++
	}

	if count != 15000 {
		t.Errorf("Expected 15000 entries, got %d", count)
	}
}

func TestCensusIMT_DumpRange_Pagination(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numEntries := 250
	addresses := make([]common.Address, numEntries)
	weights := make([]*big.Int, numEntries)

	for i := 0; i < numEntries; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i >> 8)
		addrBytes[1] = byte(i & 0xff)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	pageSize := 50
	allEntries := make([]CensusParticipant, 0, numEntries)

	for page := 0; page < 5; page++ {
		offset := page * pageSize
		reader := census.DumpRange(offset, pageSize)
		decoder := json.NewDecoder(reader)

		pageEntries := 0
		for decoder.More() {
			var entry CensusParticipant
			if err := decoder.Decode(&entry); err != nil {
				t.Fatalf("Failed to decode entry on page %d: %v", page, err)
			}
			allEntries = append(allEntries, entry)
			pageEntries++
		}

		if pageEntries != pageSize {
			t.Errorf("Page %d: expected %d entries, got %d", page, pageSize, pageEntries)
		}
	}

	if len(allEntries) != numEntries {
		t.Errorf("Expected %d total entries, got %d", numEntries, len(allEntries))
	}

	for i, entry := range allEntries {
		expectedAddr := addresses[i]
		if entry.Address != expectedAddr {
			t.Errorf("Entry %d: expected address %s, got %s", i, expectedAddr.Hex(), entry.Address.Hex())
		}
	}
}

func TestCensusIMT_DumpRange_EdgeCases(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numEntries := 100
	addresses := make([]common.Address, numEntries)
	weights := make([]*big.Int, numEntries)

	for i := 0; i < numEntries; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	t.Run("negative_offset", func(t *testing.T) {
		reader := census.DumpRange(-10, 5)
		decoder := json.NewDecoder(reader)
		count := 0
		for decoder.More() {
			var entry CensusParticipant
			if err := decoder.Decode(&entry); err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}
			count++
		}
		if count != 5 {
			t.Errorf("Expected 5 entries with negative offset, got %d", count)
		}
	})

	t.Run("offset_beyond_size", func(t *testing.T) {
		reader := census.DumpRange(200, 10)
		decoder := json.NewDecoder(reader)
		if decoder.More() {
			t.Error("Should return no entries when offset beyond size")
		}
	})

	t.Run("limit_beyond_size", func(t *testing.T) {
		reader := census.DumpRange(90, 20)
		decoder := json.NewDecoder(reader)
		count := 0
		for decoder.More() {
			var entry CensusParticipant
			if err := decoder.Decode(&entry); err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}
			count++
		}
		if count != 10 {
			t.Errorf("Expected 10 entries (90-99), got %d", count)
		}
	})

	t.Run("unlimited_range", func(t *testing.T) {
		reader := census.DumpRange(50, -1)
		decoder := json.NewDecoder(reader)
		count := 0
		for decoder.More() {
			var entry CensusParticipant
			if err := decoder.Decode(&entry); err != nil {
				t.Fatalf("Failed to decode: %v", err)
			}
			count++
		}
		if count != 50 {
			t.Errorf("Expected 50 entries (50-99), got %d", count)
		}
	})

	t.Run("zero_limit", func(t *testing.T) {
		reader := census.DumpRange(0, 0)
		decoder := json.NewDecoder(reader)
		if decoder.More() {
			t.Error("Should return no entries with zero limit")
		}
	})
}

func TestCensusIMT_Dump_Concurrent(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numInitial := 1000
	addresses := make([]common.Address, numInitial)
	weights := make([]*big.Int, numInitial)

	for i := 0; i < numInitial; i++ {
		addrBytes := make([]byte, 20)
		addrBytes[0] = byte(i >> 8)
		addrBytes[1] = byte(i & 0xff)
		addresses[i] = common.BytesToAddress(addrBytes)
		weights[i] = big.NewInt(int64(i + 1))
	}

	if err := census.AddBulk(addresses, weights); err != nil {
		t.Fatalf("Failed to add bulk addresses: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		reader := census.Dump()
		decoder := json.NewDecoder(reader)
		count := 0
		for decoder.More() {
			var entry CensusParticipant
			if err := decoder.Decode(&entry); err != nil {
				t.Errorf("Failed to decode during concurrent dump: %v", err)
				return
			}
			count++
		}
		if count < numInitial {
			t.Errorf("Dump returned fewer entries than expected: %d < %d", count, numInitial)
		}
	}()

	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			addrBytes := make([]byte, 20)
			addrBytes[0] = byte((numInitial + i) >> 8)
			addrBytes[1] = byte((numInitial + i) & 0xff)
			addr := common.BytesToAddress(addrBytes)
			if err := census.Add(addr, big.NewInt(int64(numInitial+i+1))); err != nil {
				t.Errorf("Failed to add during concurrent dump: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}

func TestCensusIMT_Dump_DataIntegrity(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

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

	for i, addr := range addresses {
		if err := census.Add(addr, weights[i]); err != nil {
			t.Fatalf("Failed to add address %d: %v", i, err)
		}
	}

	reader := census.Dump()
	decoder := json.NewDecoder(reader)

	dumpedEntries := make(map[string]*CensusParticipant)
	for decoder.More() {
		var entry CensusParticipant
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("Failed to decode entry: %v", err)
		}
		dumpedEntries[entry.Address.Hex()] = &entry
	}

	for i, addr := range addresses {
		hexAddr := addr.Hex()
		entry, exists := dumpedEntries[hexAddr]
		if !exists {
			t.Errorf("Address %s not found in dump", hexAddr)
			continue
		}

		storedWeight, exists := census.GetWeight(addr)
		if !exists {
			t.Errorf("Address %s not found in census", hexAddr)
			continue
		}

		if entry.Weight.Cmp(storedWeight) != 0 {
			t.Errorf("Weight mismatch for %s: dumped=%s, stored=%s", hexAddr, entry.Weight.String(), storedWeight.String())
		}

		if entry.Weight.Cmp(weights[i]) != 0 {
			t.Errorf("Weight mismatch for %s: dumped=%s, expected=%s", hexAddr, entry.Weight.String(), weights[i].String())
		}
	}
}

func TestCensusIMT_Dump_LargeScale(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping large scale test in short mode")
	}

	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	numEntries := 100000
	t.Logf("Adding %d entries...", numEntries)

	batchSize := 10000
	for batch := 0; batch < numEntries/batchSize; batch++ {
		addresses := make([]common.Address, batchSize)
		weights := make([]*big.Int, batchSize)

		for i := 0; i < batchSize; i++ {
			idx := batch*batchSize + i
			addrBytes := make([]byte, 20)
			addrBytes[0] = byte(idx >> 16)
			addrBytes[1] = byte(idx >> 8)
			addrBytes[2] = byte(idx & 0xff)
			addresses[i] = common.BytesToAddress(addrBytes)
			weights[i] = big.NewInt(int64(idx + 1))
		}

		if err := census.AddBulk(addresses, weights); err != nil {
			t.Fatalf("Failed to add bulk batch %d: %v", batch, err)
		}
	}

	t.Logf("Dumping %d entries...", numEntries)
	reader := census.Dump()
	decoder := json.NewDecoder(reader)

	count := 0
	for decoder.More() {
		var entry CensusParticipant
		if err := decoder.Decode(&entry); err != nil {
			t.Fatalf("Failed to decode entry %d: %v", count, err)
		}
		count++
	}

	if count != numEntries {
		t.Errorf("Expected %d entries, got %d", numEntries, count)
	}

	t.Logf("Successfully dumped and verified %d entries", count)
}

func TestCensusIMT_DumpRange_JSONFormat(t *testing.T) {
	tempDir := t.TempDir()
	census, err := NewCensusIMTWithPebble(tempDir, leanimt.PoseidonHasher)
	if err != nil {
		t.Fatalf("Failed to create census: %v", err)
	}
	defer func() {
		if err := census.Close(); err != nil {
			t.Errorf("Failed to close census: %v", err)
		}
	}()

	addr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
	weight := big.NewInt(1000)

	if err := census.Add(addr, weight); err != nil {
		t.Fatalf("Failed to add address: %v", err)
	}

	reader := census.Dump()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("Failed to read dump: %v", err)
	}

	var entry CensusParticipant
	if err := json.Unmarshal(data, &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry.Address != addr {
		t.Errorf("Address mismatch: expected %s, got %s", addr.Hex(), entry.Address.Hex())
	}

	if entry.Weight.Cmp(weight) != 0 {
		t.Errorf("Weight mismatch: expected %s, got %s", weight.String(), entry.Weight.String())
	}
}

// Test that importing a dump on top of an existing persisted census does not
// append to previously stored leaves (the regression that produced a different
// root). It uses the same Pebble store across two CensusIMT instances to mimic
// reusing the database between runs.
func TestImportAllClearsPersistedTreeState(t *testing.T) {
	dir := t.TempDir()

	c1, err := NewCensusIMTWithPebble(dir, leanimt.SHA256Hasher)
	if err != nil {
		t.Fatalf("failed to create census: %v", err)
	}

	addrs := []common.Address{
		common.HexToAddress(fmt.Sprintf("%040x", 1)),
		common.HexToAddress(fmt.Sprintf("%040x", 2)),
		common.HexToAddress(fmt.Sprintf("%040x", 3)),
	}
	weights := []*big.Int{big.NewInt(10), big.NewInt(20), big.NewInt(30)}

	for i, addr := range addrs {
		if err := c1.Add(addr, weights[i]); err != nil {
			t.Fatalf("add participant %d failed: %v", i, err)
		}
	}

	dump, err := c1.DumpAll()
	if err != nil {
		t.Fatalf("dump failed: %v", err)
	}
	if err := c1.Sync(); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if err := c1.Close(); err != nil {
		t.Fatalf("close failed: %v", err)
	}

	// Re-open the same persistent store and import the dump.
	c2, err := NewCensusIMTWithPebble(dir, leanimt.SHA256Hasher)
	if err != nil {
		t.Fatalf("failed to reopen census: %v", err)
	}
	defer c2.Close()

	if err := c2.ImportAll(dump); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	root, ok := c2.Root()
	if !ok {
		t.Fatalf("imported census has no root")
	}
	if root.Cmp(dump.Root) != 0 {
		t.Fatalf("root mismatch after import: got %s, want %s", root, dump.Root)
	}
	if got, want := c2.Size(), len(dump.Participants); got != want {
		t.Fatalf("imported census size mismatch: got %d, want %d", got, want)
	}
}
