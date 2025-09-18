package leanimt

import (
	"math/big"
	"os"
	"testing"

	"github.com/vocdoni/davinci-node/db"
)

// Helper functions for testing persistence with big.Int

func bigIntEncoder(n *big.Int) ([]byte, error) {
	if n == nil {
		return []byte{}, nil
	}
	// Use a simple encoding that preserves zero values
	bytes := n.Bytes()
	if len(bytes) == 0 && n.Sign() == 0 {
		return []byte{0}, nil // Explicitly encode zero
	}
	return bytes, nil
}

func bigIntDecoder(data []byte) (*big.Int, error) {
	if len(data) == 0 {
		return big.NewInt(0), nil
	}
	if len(data) == 1 && data[0] == 0 {
		return big.NewInt(0), nil // Explicitly decode zero
	}
	return new(big.Int).SetBytes(data), nil
}

func createTempDir(t *testing.T) string {
	dir, err := os.MkdirTemp("", "leanimt-test-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return dir
}

func TestPersistenceBasic(t *testing.T) {
	tempDir := createTempDir(t)

	// Create tree with some data
	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree1.Close() }()

	// Add some leaves
	leaves := []*big.Int{bigInt(1), bigInt(2), bigInt(3), bigInt(4), bigInt(5)}
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// Get root before sync
	root1, ok := tree1.Root()
	if !ok {
		t.Fatal("root should exist")
	}

	// Sync to disk
	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}

	// Close the tree
	if err := tree1.Close(); err != nil {
		t.Fatal(err)
	}

	// Create new tree from same directory
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree2.Close() }()

	// Verify tree was loaded correctly
	if tree2.Size() != 5 {
		t.Fatalf("expected size 5, got %d", tree2.Size())
	}

	root2, ok := tree2.Root()
	if !ok {
		t.Fatal("root should exist")
	}

	if root1.Cmp(root2) != 0 {
		t.Fatal("roots should match after persistence")
	}

	// Verify leaves
	loadedLeaves := tree2.Leaves()
	for i, expected := range leaves {
		if loadedLeaves[i].Cmp(expected) != 0 {
			t.Fatalf("leaf %d mismatch: expected %s, got %s", i, expected, loadedLeaves[i])
		}
	}
}

func TestPersistenceEmpty(t *testing.T) {
	tempDir := createTempDir(t)

	// Create empty tree
	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	if tree1.Size() != 0 {
		t.Fatalf("expected empty tree, got size %d", tree1.Size())
	}

	// Sync empty tree
	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree1.Close()

	// Load empty tree
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree2.Close() }()

	if tree2.Size() != 0 {
		t.Fatalf("expected empty tree after load, got size %d", tree2.Size())
	}
}

func TestPersistenceUpdates(t *testing.T) {
	tempDir := createTempDir(t)

	// Create tree with initial data
	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add initial leaves
	leaves := []*big.Int{bigInt(10), bigInt(20), bigInt(30)}
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// Sync and close
	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree1.Close()

	// Reopen and update
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Update middle leaf
	if err := tree2.Update(1, bigInt(99)); err != nil {
		t.Fatal(err)
	}

	// Add more leaves
	if err := tree2.Insert(bigInt(40)); err != nil {
		t.Fatal(err)
	}

	// Sync and close
	if err := tree2.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree2.Close()

	// Reopen and verify
	tree3, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree3.Close() }()

	if tree3.Size() != 4 {
		t.Fatalf("expected size 4, got %d", tree3.Size())
	}

	expectedLeaves := []*big.Int{bigInt(10), bigInt(99), bigInt(30), bigInt(40)}
	loadedLeaves := tree3.Leaves()
	for i, expected := range expectedLeaves {
		if loadedLeaves[i].Cmp(expected) != 0 {
			t.Fatalf("leaf %d mismatch: expected %s, got %s", i, expected, loadedLeaves[i])
		}
	}
}

func TestPersistenceCleanup(t *testing.T) {
	tempDir := createTempDir(t)

	// Create tree with many leaves
	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Add 10 leaves
	leaves := make([]*big.Int, 10)
	for i := 0; i < 10; i++ {
		leaves[i] = bigInt(int64(i))
	}
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree1.Close()

	// Reopen and create smaller tree (simulating removal)
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Replace with fewer leaves by creating new tree
	tree2.nodes = [][]*big.Int{make([]*big.Int, 0)}
	smallerLeaves := []*big.Int{bigInt(100), bigInt(200)}
	if err := tree2.InsertMany(smallerLeaves); err != nil {
		t.Fatal(err)
	}

	if err := tree2.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree2.Close()

	// Verify cleanup worked - reopen and check
	tree3, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree3.Close() }()

	if tree3.Size() != 2 {
		t.Fatalf("expected size 2 after cleanup, got %d", tree3.Size())
	}

	// Verify old leaves are gone by checking database through tree3's connection
	// Check that old leaf keys don't exist
	for i := 2; i < 10; i++ {
		key := []byte("leaf:" + intToString(i))
		_, err := tree3.db.Get(key)
		if err != db.ErrKeyNotFound {
			t.Fatalf("expected old leaf %d to be cleaned up", i)
		}
	}
}

func TestPersistenceDirtyFlag(t *testing.T) {
	tempDir := createTempDir(t)

	tree, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree.Close() }()

	// Initially not dirty
	if tree.dirty {
		t.Fatal("new tree should not be dirty")
	}

	// Insert should make it dirty
	if err := tree.Insert(bigInt(1)); err != nil {
		t.Fatal(err)
	}
	if !tree.dirty {
		t.Fatal("tree should be dirty after insert")
	}

	// Sync should clear dirty flag
	if err := tree.Sync(); err != nil {
		t.Fatal(err)
	}
	if tree.dirty {
		t.Fatal("tree should not be dirty after sync")
	}

	// Update should make it dirty again
	if err := tree.Update(0, bigInt(2)); err != nil {
		t.Fatal(err)
	}
	if !tree.dirty {
		t.Fatal("tree should be dirty after update")
	}

	// Sync with no changes should be no-op
	if err := tree.Sync(); err != nil {
		t.Fatal(err)
	}
	if tree.dirty {
		t.Fatal("tree should not be dirty after sync")
	}

	// Second sync should be no-op (not dirty)
	if err := tree.Sync(); err != nil {
		t.Fatal(err)
	}
}

func TestPersistenceInMemoryMode(t *testing.T) {
	// Test that in-memory mode still works
	tree, err := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Add some data
	if err := tree.Insert(bigInt(1)); err != nil {
		t.Fatal(err)
	}

	// Sync should be no-op
	if err := tree.Sync(); err != nil {
		t.Fatal(err)
	}

	// Close should be no-op
	if err := tree.Close(); err != nil {
		t.Fatal(err)
	}

	if tree.Size() != 1 {
		t.Fatal("in-memory tree should work normally")
	}
}

func TestPersistenceProofs(t *testing.T) {
	tempDir := createTempDir(t)

	// Create tree and add data
	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	leaves := []*big.Int{bigInt(10), bigInt(20), bigInt(30), bigInt(40), bigInt(50)}
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// Generate proof before persistence
	proof1, err := tree1.GenerateProof(2)
	if err != nil {
		t.Fatal(err)
	}

	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree1.Close()

	// Load tree and generate same proof
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree2.Close() }()

	proof2, err := tree2.GenerateProof(2)
	if err != nil {
		t.Fatal(err)
	}

	// Proofs should be identical
	if proof1.Index != proof2.Index {
		t.Fatal("proof indices should match")
	}
	if proof1.Leaf.Cmp(proof2.Leaf) != 0 {
		t.Fatal("proof leaves should match")
	}
	if proof1.Root.Cmp(proof2.Root) != 0 {
		t.Fatal("proof roots should match")
	}
	if len(proof1.Siblings) != len(proof2.Siblings) {
		t.Fatal("proof siblings length should match")
	}
	for i, sib1 := range proof1.Siblings {
		if sib1.Cmp(proof2.Siblings[i]) != 0 {
			t.Fatalf("proof sibling %d should match", i)
		}
	}

	// Both proofs should verify
	if !tree2.VerifyProof(proof1) {
		t.Fatal("original proof should verify on loaded tree")
	}
	if !tree2.VerifyProof(proof2) {
		t.Fatal("new proof should verify")
	}
}

func TestPersistenceLargeTree(t *testing.T) {
	tempDir := createTempDir(t)

	tree1, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create a larger tree (1000 leaves)
	const numLeaves = 1000
	leaves := make([]*big.Int, numLeaves)
	for i := 0; i < numLeaves; i++ {
		leaves[i] = bigInt(int64(i * 7)) // some pattern
	}

	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	root1, _ := tree1.Root()
	depth1 := tree1.Depth()

	if err := tree1.Sync(); err != nil {
		t.Fatal(err)
	}
	_ = tree1.Close()

	// Load and verify
	tree2, err := NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, tempDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tree2.Close() }()

	if tree2.Size() != numLeaves {
		t.Fatalf("expected %d leaves, got %d", numLeaves, tree2.Size())
	}

	root2, _ := tree2.Root()
	if root1.Cmp(root2) != 0 {
		t.Fatal("roots should match for large tree")
	}

	if tree2.Depth() != depth1 {
		t.Fatalf("depths should match: expected %d, got %d", depth1, tree2.Depth())
	}

	// Verify some random leaves
	testIndices := []int{0, 100, 500, 999}
	for _, idx := range testIndices {
		if tree2.Leaves()[idx].Cmp(leaves[idx]) != 0 {
			t.Fatalf("leaf %d should match", idx)
		}
	}
}

func TestPersistenceErrorHandling(t *testing.T) {
	// Test with invalid encoder/decoder
	tempDir := createTempDir(t)

	// Missing encoder
	_, err := NewWithPebble(bigIntHasher, BigIntEqual, nil, bigIntDecoder, tempDir)
	if err == nil {
		t.Fatal("should fail with nil encoder")
	}

	// Missing decoder
	_, err = NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, nil, tempDir)
	if err == nil {
		t.Fatal("should fail with nil decoder")
	}

	// Test with invalid directory
	_, err = NewWithPebble(bigIntHasher, BigIntEqual, bigIntEncoder, bigIntDecoder, "/invalid/path/that/does/not/exist")
	if err == nil {
		t.Fatal("should fail with invalid directory")
	}
}
