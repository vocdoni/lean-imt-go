package leanimt

import (
	"math/big"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Large-scale benchmarks for 20M leaves with persistence and concurrency testing

// BenchmarkLargeTree_20M_Creation tests creating a 20M leaf tree
func BenchmarkLargeTree_20M_Creation(b *testing.B) {
	benchmarkLargeTreeCreation(b, 20_000_000, bigIntHasher, BigIntEqual)
}

// BenchmarkLargeTree_10M_Poseidon2_Creation tests creating a 10M leaf tree with Poseidon2
func BenchmarkLargeTree_10M_Poseidon2_Creation(b *testing.B) {
	benchmarkLargeTreeCreation(b, 10_000_000, Poseidon2Hasher, BigIntEqual)
}

// BenchmarkLargeTree_10M_Persistence tests saving and loading a 10M leaf tree
func BenchmarkLargeTree_20M_Persistence(b *testing.B) {
	benchmarkLargeTreePersistence(b, 10_000_000, bigIntHasher, BigIntEqual)
}

// BenchmarkLargeTree_20M_Updates tests updating leaves in a 20M leaf tree
func BenchmarkLargeTree_20M_Updates(b *testing.B) {
	benchmarkLargeTreeUpdates(b, 20_000_000, bigIntHasher, BigIntEqual)
}

// BenchmarkLargeTree_20M_Concurrent tests concurrent operations on a 20M leaf tree
func BenchmarkLargeTree_20M_Concurrent(b *testing.B) {
	benchmarkLargeTreeConcurrent(b, 20_000_000, bigIntHasher, BigIntEqual)
}

// Smaller benchmarks for comparison
func BenchmarkInsertMany_SimpleHash_1M(b *testing.B) {
	benchmarkInsertMany(b, 1_000_000, bigIntHasher, BigIntEqual)
}

func BenchmarkInsertMany_Poseidon2_1M(b *testing.B) {
	benchmarkInsertMany(b, 1_000_000, Poseidon2Hasher, BigIntEqual)
}

func BenchmarkGenerateProof_SimpleHash_1M(b *testing.B) {
	benchmarkGenerateProof(b, 1_000_000, bigIntHasher, BigIntEqual)
}

func BenchmarkUpdate_SimpleHash_1M(b *testing.B) {
	benchmarkUpdate(b, 1_000_000, bigIntHasher, BigIntEqual)
}

// Helper functions for benchmarks

func benchmarkInsertMany(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		// Prepare leaves
		leaves := make([]*big.Int, numLeaves)
		for j := 0; j < numLeaves; j++ {
			leaves[j] = big.NewInt(int64(j))
		}
		tree, _ := New(hash, eq, nil, nil, nil)
		b.StartTimer()

		// Benchmark the insertion
		err := tree.InsertMany(leaves)
		if err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		// Verify the tree was built correctly
		if tree.Size() != numLeaves {
			b.Fatalf("Expected %d leaves, got %d", numLeaves, tree.Size())
		}
		if _, ok := tree.Root(); !ok {
			b.Fatal("Root should exist")
		}
		b.StartTimer() // Ensure timer is started before loop ends
	}
}

func benchmarkGenerateProof(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	// Setup: create a tree with the specified number of leaves
	leaves := make([]*big.Int, numLeaves)
	for i := 0; i < numLeaves; i++ {
		leaves[i] = big.NewInt(int64(i))
	}
	tree, err := New(hash, eq, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := tree.InsertMany(leaves); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Generate proof for a random leaf (middle of the tree)
		index := numLeaves / 2
		proof, err := tree.GenerateProof(index)
		if err != nil {
			b.Fatal(err)
		}

		// Verify the proof to ensure correctness
		if !tree.VerifyProof(proof) {
			b.Fatal("Proof verification failed")
		}
	}
}

func benchmarkUpdate(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	// Setup: create a tree with the specified number of leaves
	leaves := make([]*big.Int, numLeaves)
	for i := range numLeaves {
		leaves[i] = big.NewInt(int64(i))
	}
	tree, err := New(hash, eq, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}
	if err := tree.InsertMany(leaves); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Update a leaf in the middle of the tree
		index := numLeaves / 2
		newValue := big.NewInt(int64(numLeaves + i))
		err := tree.Update(index, newValue)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// Memory usage benchmark
func BenchmarkMemoryUsage_20M(b *testing.B) {
	const numLeaves = 20_000_000

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		leaves := make([]*big.Int, numLeaves)
		for j := 0; j < numLeaves; j++ {
			leaves[j] = big.NewInt(int64(j))
		}
		b.StartTimer()

		tree, err := New(bigIntHasher, BigIntEqual, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}
		if err := tree.InsertMany(leaves); err != nil {
			b.Fatal(err)
		}

		b.StopTimer()
		// Report memory stats
		if tree.Size() != numLeaves {
			b.Fatalf("Expected %d leaves, got %d", numLeaves, tree.Size())
		}
		b.Logf("Tree depth: %d", tree.Depth())
		b.Logf("Tree size: %d", tree.Size())
	}
}

// Large-scale benchmark implementations

func benchmarkLargeTreeCreation(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Prepare leaves
		b.Logf("Preparing %d leaves...", numLeaves)
		start := time.Now()
		leaves := make([]*big.Int, numLeaves)
		for j := 0; j < numLeaves; j++ {
			leaves[j] = big.NewInt(int64(j))
		}
		prepTime := time.Since(start)
		b.Logf("Leaf preparation took: %v", prepTime)

		// Create tree
		tree, err := New(hash, eq, nil, nil, nil)
		if err != nil {
			b.Fatal(err)
		}

		b.StartTimer()
		start = time.Now()

		// Parallel insertion in batches
		const batchSize = 10000
		const maxGoroutines = 10

		numBatches := (numLeaves + batchSize - 1) / batchSize
		goroutines := min(numBatches, maxGoroutines)

		batchChan := make(chan []int, numBatches)
		var wg sync.WaitGroup

		// Create batches
		for batchStart := 0; batchStart < numLeaves; batchStart += batchSize {
			batchEnd := batchStart + batchSize
			if batchEnd > numLeaves {
				batchEnd = numLeaves
			}

			batch := make([]int, batchEnd-batchStart)
			for j := 0; j < len(batch); j++ {
				batch[j] = batchStart + j
			}
			batchChan <- batch
		}
		close(batchChan)

		// Process batches in parallel
		for g := 0; g < goroutines; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for batch := range batchChan {
					batchLeaves := make([]*big.Int, len(batch))
					for j, idx := range batch {
						batchLeaves[j] = leaves[idx]
					}
					err := tree.InsertMany(batchLeaves)
					if err != nil {
						b.Errorf("Batch insertion failed: %v", err)
						return
					}
				}
			}()
		}

		wg.Wait()
		creationTime := time.Since(start)
		b.StopTimer()

		// Verify and report
		if tree.Size() != numLeaves {
			b.Fatalf("Expected %d leaves, got %d", numLeaves, tree.Size())
		}
		if _, ok := tree.Root(); !ok {
			b.Fatal("Root should exist")
		}

		b.Logf("Tree creation took: %v", creationTime)
		b.Logf("Tree depth: %d", tree.Depth())
		b.Logf("Leaves per second: %.0f", float64(numLeaves)/creationTime.Seconds())
		b.Logf("Used %d goroutines with %d batches of %d leaves each", goroutines, numBatches, batchSize)

		// Memory stats
		var m runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m)
		b.Logf("Memory allocated: %d MB", m.Alloc/1024/1024)
		b.Logf("Total allocations: %d MB", m.TotalAlloc/1024/1024)
	}
}

func benchmarkLargeTreePersistence(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "leanimt-bench-*")
	if err != nil {
		b.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()

		// Prepare leaves
		b.Logf("Creating tree with %d leaves for persistence test...", numLeaves)
		leaves := make([]*big.Int, numLeaves)
		for j := 0; j < numLeaves; j++ {
			leaves[j] = big.NewInt(int64(j))
		}

		// Create tree with persistence
		tree, err := NewWithPebble(hash, eq, BigIntEncoder, BigIntDecoder, tempDir)
		if err != nil {
			b.Fatal(err)
		}

		// Insert leaves
		start := time.Now()
		err = tree.InsertMany(leaves)
		if err != nil {
			b.Fatal(err)
		}
		insertTime := time.Since(start)
		b.Logf("Insert time: %v", insertTime)

		b.StartTimer()

		// Benchmark saving to disk
		start = time.Now()
		err = tree.Sync()
		if err != nil {
			b.Fatal(err)
		}
		saveTime := time.Since(start)

		// Close the tree
		err = tree.Close()
		if err != nil {
			b.Fatal(err)
		}

		// Benchmark loading from disk
		start = time.Now()
		tree2, err := NewWithPebble(hash, eq, BigIntEncoder, BigIntDecoder, tempDir)
		if err != nil {
			b.Fatal(err)
		}
		loadTime := time.Since(start)

		b.StopTimer()

		// Verify loaded tree
		if tree2.Size() != numLeaves {
			b.Fatalf("Expected %d leaves after load, got %d", numLeaves, tree2.Size())
		}

		_ = tree2.Close()

		b.Logf("Save time: %v", saveTime)
		b.Logf("Load time: %v", loadTime)
		b.Logf("Total persistence time: %v", saveTime+loadTime)

		// Clean up for next iteration
		_ = os.RemoveAll(tempDir)
		tempDir, _ = os.MkdirTemp("", "leanimt-bench-*")
	}
}

func benchmarkLargeTreeUpdates(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	b.StopTimer()

	// Setup: create a large tree
	b.Logf("Setting up tree with %d leaves for update benchmark...", numLeaves)
	leaves := make([]*big.Int, numLeaves)
	for i := 0; i < numLeaves; i++ {
		leaves[i] = big.NewInt(int64(i))
	}

	tree, err := New(hash, eq, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	start := time.Now()
	err = tree.InsertMany(leaves)
	if err != nil {
		b.Fatal(err)
	}
	setupTime := time.Since(start)
	b.Logf("Tree setup took: %v", setupTime)

	b.ResetTimer()
	b.StartTimer()

	// Benchmark updates
	const numUpdates = 1000
	start = time.Now()
	for i := 0; i < numUpdates; i++ {
		// Update random leaves
		index := i % numLeaves
		newValue := big.NewInt(int64(numLeaves + i))
		err := tree.Update(index, newValue)
		if err != nil {
			b.Fatal(err)
		}
	}
	updateTime := time.Since(start)

	b.StopTimer()

	b.Logf("Updated %d leaves in: %v", numUpdates, updateTime)
	b.Logf("Updates per second: %.0f", float64(numUpdates)/updateTime.Seconds())
	b.Logf("Average update time: %v", updateTime/numUpdates)
}

func benchmarkLargeTreeConcurrent(b *testing.B, numLeaves int, hash Hasher[*big.Int], eq Equal[*big.Int]) {
	b.StopTimer()

	// Setup: create a large tree
	b.Logf("Setting up tree with %d leaves for concurrency benchmark...", numLeaves)
	leaves := make([]*big.Int, numLeaves)
	for i := 0; i < numLeaves; i++ {
		leaves[i] = big.NewInt(int64(i))
	}

	tree, err := New(hash, eq, nil, nil, nil)
	if err != nil {
		b.Fatal(err)
	}

	err = tree.InsertMany(leaves)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.StartTimer()

	// Test concurrent operations - separate readers and writers to avoid proof invalidation
	const numGoroutines = 10
	const operationsPerGoroutine = 100

	start := time.Now()
	var wg sync.WaitGroup

	// Phase 1: Concurrent readers only (proof generation and verification)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine/2; j++ {
				index := (goroutineID*operationsPerGoroutine + j) % numLeaves
				proof, err := tree.GenerateProof(index)
				if err != nil {
					b.Errorf("Proof generation failed: %v", err)
					return
				}
				if !tree.VerifyProof(proof) {
					b.Errorf("Proof verification failed for index %d", index)
					return
				}
			}
		}(i)
	}

	wg.Wait()

	// Phase 2: Concurrent writers only (updates)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()
			for j := 0; j < operationsPerGoroutine/2; j++ {
				index := (goroutineID*operationsPerGoroutine + j) % numLeaves
				newValue := big.NewInt(int64(numLeaves + goroutineID*1000 + j))
				err := tree.Update(index, newValue)
				if err != nil {
					b.Errorf("Update failed: %v", err)
					return
				}
			}
		}(i)
	}

	wg.Wait()
	concurrentTime := time.Since(start)

	b.StopTimer()

	totalOps := numGoroutines * operationsPerGoroutine
	b.Logf("Performed %d concurrent operations in: %v", totalOps, concurrentTime)
	b.Logf("Operations per second: %.0f", float64(totalOps)/concurrentTime.Seconds())
	b.Logf("Used %d goroutines in 2 phases (readers then writers)", numGoroutines)
}
