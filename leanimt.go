package leanimt

import (
	"errors"
	"reflect"
	"sync"

	"github.com/vocdoni/davinci-node/db"
	"github.com/vocdoni/davinci-node/db/metadb"
)

// Hasher is the binary hash used for internal nodes.
type Hasher[N any] func(a, b N) N

// Equal is an optional equality comparator used for leaf lookups and proofs.
// If nil, reflect.DeepEqual is used.
type Equal[N any] func(a, b N) bool

// LeanIMT is a binary Lean Incremental Merkle Tree.
//   - dynamic depth (ceil(log2(size)))
//   - no zero nodes; if a right child is missing, parent = left child
//   - proofs omit missing siblings and encode the path as an index integer.
//
// LeanIMT is safe for concurrent use by multiple goroutines.
type LeanIMT[N any] struct {
	mu      sync.RWMutex // protects all fields below
	nodes   [][]N
	hash    Hasher[N]
	eq      Equal[N]
	db      db.Database             // nil for in-memory only
	encoder func(N) ([]byte, error) // serialize leaf to bytes
	decoder func([]byte) (N, error) // deserialize bytes to leaf
	dirty   bool                    // track if changes need syncing
}

// New creates a new empty LeanIMT with the provided hash function.
// If eq is nil, reflect.DeepEqual is used for equality.
// If storage is nil, the tree operates in memory-only mode.
// If storage is provided, encoder and decoder functions must also be provided.
//
// Example usage:
//
//	tree, err := New(BigIntHasher, BigIntEqual, nil, nil, nil)                    // in-memory
//	tree, err := New(BigIntHasher, BigIntEqual, db, BigIntEncoder, BigIntDecoder) // persistent
func New[N any](hash Hasher[N], eq Equal[N], storage db.Database, encoder func(N) ([]byte, error), decoder func([]byte) (N, error)) (*LeanIMT[N], error) {
	if hash == nil {
		return nil, errors.New("parameter 'hash' is not defined")
	}
	if storage != nil && (encoder == nil || decoder == nil) {
		return nil, errors.New("encoder and decoder functions are required when using persistent storage")
	}

	t := &LeanIMT[N]{
		nodes:   [][]N{make([]N, 0)}, // level 0 = leaves
		hash:    hash,
		eq:      eq,
		db:      storage,
		encoder: encoder,
		decoder: decoder,
		dirty:   false,
	}

	// Try to load existing tree from database if storage is provided
	if storage != nil {
		if err := t.Load(); err != nil {
			// If loading fails, start with empty tree
			// This handles the case of a new database
			if err != db.ErrKeyNotFound {
				return nil, err // Return actual errors, not just key not found
			}
			t.nodes = [][]N{make([]N, 0)}
		}
	}

	return t, nil
}

// NewWithPebble is a wrapper around New. Creates a new LeanIMT using a persistent Pebble DB at the specified directory.
func NewWithPebble[N any](hash Hasher[N], eq Equal[N], encoder func(N) ([]byte, error), decoder func([]byte) (N, error), datadir string) (*LeanIMT[N], error) {
	if encoder == nil || decoder == nil {
		return nil, errors.New("encoder and decoder functions are required for persistent storage")
	}

	database, err := metadb.New(db.TypePebble, datadir)
	if err != nil {
		return nil, err
	}

	return New(hash, eq, database, encoder, decoder)
}

// equal compares two values, using provided eq if present, otherwise reflect.DeepEqual.
func (t *LeanIMT[N]) equal(a, b N) bool {
	if t.eq != nil {
		return t.eq(a, b)
	}
	return reflect.DeepEqual(a, b)
}

// Depth returns the current dynamic depth (levels - 1).
func (t *LeanIMT[N]) Depth() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.nodes) - 1
}

// Size returns the number of leaves.
func (t *LeanIMT[N]) Size() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.nodes[0])
}

// Leaves returns a copy of the leaves array.
func (t *LeanIMT[N]) Leaves() []N {
	t.mu.RLock()
	defer t.mu.RUnlock()
	cp := make([]N, len(t.nodes[0]))
	copy(cp, t.nodes[0])
	return cp
}

// Root returns the root and a boolean indicating whether it exists.
func (t *LeanIMT[N]) Root() (N, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rootUnsafe()
}

// rootUnsafe returns the root without acquiring locks (internal use).
func (t *LeanIMT[N]) rootUnsafe() (N, bool) {
	var zero N
	if len(t.nodes) == 0 {
		return zero, false
	}
	depth := len(t.nodes) - 1
	if depth < 0 || len(t.nodes[depth]) == 0 {
		return zero, false
	}
	return t.nodes[depth][0], true
}

// IndexOf returns the index of a leaf by equality; -1 if not present.
func (t *LeanIMT[N]) IndexOf(leaf N) int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.indexOfUnsafe(leaf)
}

// indexOfUnsafe returns the index of a leaf without acquiring locks (internal use).
func (t *LeanIMT[N]) indexOfUnsafe(leaf N) int {
	for i, v := range t.nodes[0] {
		if t.equal(v, leaf) {
			return i
		}
	}
	return -1
}

// Has returns true if the leaf is present.
func (t *LeanIMT[N]) Has(leaf N) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.indexOfUnsafe(leaf) >= 0
}

// Insert inserts a single leaf at the end, updating path to root bottom-up.
func (t *LeanIMT[N]) Insert(leaf N) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	// If next depth increases, add a level.
	nextSize := len(t.nodes[0]) + 1
	if len(t.nodes)-1 < ceilLog2(nextSize) {
		t.nodes = append(t.nodes, make([]N, 0)) // new level
	}

	node := leaf
	index := len(t.nodes[0]) // index of the new leaf

	// ensure capacity at leaves and set
	ensureIndex(&t.nodes[0], index)
	t.nodes[0][index] = node

	// Update parents up to last-but-top; top is assigned after loop.
	depth := len(t.nodes) - 1
	for level := range depth {
		// For non-leaf levels, store the node at [level][index], then compute parent if right child.
		if level > 0 {
			ensureIndex(&t.nodes[level], index)
			t.nodes[level][index] = node
		}

		if (index & 1) == 1 {
			// right child, must hash with left sibling
			sibling := t.nodes[level][index-1]
			node = t.hash(sibling, node)
		}
		// left child; parent equals left unless right exists (handled when inserting next leaf)
		index >>= 1
	}

	// store root as the single element on the top level
	top := depth
	t.nodes[top] = t.nodes[top][:0]
	t.nodes[top] = append(t.nodes[top], node)

	t.markDirty()
	return nil
}

// InsertMany inserts m leaves in batch (more efficient than m x Insert).
func (t *LeanIMT[N]) InsertMany(leaves []N) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(leaves) == 0 {
		return errors.New("there are no leaves to add")
	}

	startIndex := len(t.nodes[0]) >> 1
	// append leaves at level 0
	t.nodes[0] = append(t.nodes[0], leaves...)

	// add necessary new levels
	newLevels := ceilLog2(len(t.nodes[0])) - (len(t.nodes) - 1)
	for range newLevels {
		t.nodes = append(t.nodes, make([]N, 0))
	}

	// compute parents level by level
	for level := 0; level < len(t.nodes)-1; level++ {
		numNodes := (len(t.nodes[level]) + 1) / 2 // ceil
		for index := startIndex; index < numNodes; index++ {
			li := index * 2
			ri := li + 1

			left := t.nodes[level][li]
			var parent N
			if ri < len(t.nodes[level]) {
				right := t.nodes[level][ri]
				parent = t.hash(left, right)
			} else {
				parent = left
			}
			ensureIndex(&t.nodes[level+1], index)
			t.nodes[level+1][index] = parent
		}
		startIndex >>= 1
	}

	t.markDirty()
	return nil
}

// Update replaces the leaf at index with newLeaf and updates path to root.
func (t *LeanIMT[N]) Update(index int, newLeaf N) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if index < 0 || index >= len(t.nodes[0]) {
		return errors.New("index is out of range")
	}

	node := newLeaf
	// first level
	t.nodes[0][index] = node

	depth := len(t.nodes) - 1
	for level := 0; level < depth; level++ {
		if level > 0 {
			ensureIndex(&t.nodes[level], index)
			t.nodes[level][index] = node
		}
		if (index & 1) == 1 {
			// right: must have left sibling
			sibling := t.nodes[level][index-1]
			node = t.hash(sibling, node)
		} else {
			// left: right sibling may or may not exist
			ri := index + 1
			if ri < len(t.nodes[level]) {
				sibling := t.nodes[level][ri]
				node = t.hash(node, sibling)
			}
			// parent equals left child (node) when no right sibling
		}
		index >>= 1
	}

	top := depth
	t.nodes[top] = t.nodes[top][:0]
	t.nodes[top] = append(t.nodes[top], node)

	t.markDirty()
	return nil
}

// UpdateMany updates multiple leaves efficiently in O(n).
// It validates indices (range and duplicates).
func (t *LeanIMT[N]) UpdateMany(indices []int, leaves []N) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if indices == nil {
		return errors.New("parameter 'indices' is not defined")
	}
	if leaves == nil {
		return errors.New("parameter 'leaves' is not defined")
	}
	if len(indices) != len(leaves) {
		return errors.New("there is no correspondence between indices and leaves")
	}
	// validate all indices
	seen := make(map[int]struct{}, len(indices))
	for i, idx := range indices {
		if idx < 0 || idx >= len(t.nodes[0]) {
			return errors.New("index " + itoa(i) + " is out of range")
		}
		if _, ok := seen[idx]; ok {
			return errors.New("leaf " + itoa(idx) + " is repeated")
		}
		seen[idx] = struct{}{}
	}
	if len(indices) == 0 {
		// no-op
		return nil
	}

	// level 0 assignments and track modified parents
	modified := make(map[int]struct{})
	for i, idx := range indices {
		t.nodes[0][idx] = leaves[i]
		modified[idx>>1] = struct{}{}
	}

	// propagate up
	for level := 1; level <= len(t.nodes)-1; level++ {
		next := make(map[int]struct{})
		for idx := range modified {
			li := 2 * idx
			ri := li + 1
			left := t.nodes[level-1][li]
			var parent N
			if ri < len(t.nodes[level-1]) {
				right := t.nodes[level-1][ri]
				parent = t.hash(left, right)
			} else {
				parent = left
			}
			ensureIndex(&t.nodes[level], idx)
			t.nodes[level][idx] = parent
			next[idx>>1] = struct{}{}
		}
		modified = next
	}

	t.markDirty()
	return nil
}

// ceilLog2 returns minimal d >= 0 such that 2^d >= n.
func ceilLog2(n int) int {
	if n <= 1 {
		return 0
	}
	// Equivalent to floor(log2(n-1)) + 1 without floats
	d := 0
	x := n - 1
	for x > 0 {
		d++
		x >>= 1
	}
	return d
}

// itoa is tiny, local integer to string (no fmt in hot paths).
func itoa(x int) string {
	// fast path for small ints
	if x >= 0 && x < 10 {
		return string('0' + byte(x))
	}
	// fallback
	return intToString(x)
}

// intToString avoids pulling fmt into hot files
func intToString(x int) string {
	neg := x < 0
	if neg {
		x = -x
	}
	buf := [20]byte{}
	i := len(buf)
	for {
		i--
		buf[i] = byte('0' + (x % 10))
		x /= 10
		if x == 0 {
			break
		}
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ensureIndex grows s so that s[index] is addressable.
func ensureIndex[N any](s *[]N, index int) {
	if index < len(*s) {
		return
	}
	missing := index + 1 - len(*s)
	*s = append(*s, make([]N, missing)...)
}

// Load restores the tree from persistent storage.
// It reads all leaves from the database and rebuilds the tree structure.
func (t *LeanIMT[N]) Load() error {
	if t.db == nil {
		return errors.New("no database configured for loading")
	}
	if t.decoder == nil {
		return errors.New("no decoder function configured")
	}

	// Read tree size from metadata
	sizeBytes, err := t.db.Get([]byte("meta:size"))
	if err != nil {
		if err == db.ErrKeyNotFound {
			// No existing tree, start empty
			t.nodes = [][]N{make([]N, 0)}
			return nil
		}
		return err
	}

	size := decodeInt(sizeBytes)
	if size == 0 {
		t.nodes = [][]N{make([]N, 0)}
		return nil
	}

	// Load all leaves
	leaves := make([]N, size)
	for i := range size {
		key := []byte("leaf:" + intToString(i))
		leafBytes, err := t.db.Get(key)
		if err != nil {
			return err
		}
		leaf, err := t.decoder(leafBytes)
		if err != nil {
			return err
		}
		leaves[i] = leaf
	}

	// Rebuild tree structure
	t.nodes = [][]N{leaves}
	if err := t.rebuildTree(); err != nil {
		return err
	}

	t.dirty = false
	return nil
}

// Sync persists the current tree state to disk atomically.
// Only the leaves are stored; intermediate nodes are computed on load.
func (t *LeanIMT[N]) Sync() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.db == nil {
		return nil // no-op for in-memory trees
	}
	if t.encoder == nil {
		return errors.New("no encoder function configured")
	}
	if !t.dirty {
		return nil // no changes to sync
	}

	tx := t.db.WriteTx()
	defer tx.Discard()

	currentSize := len(t.nodes[0]) // Use direct access instead of Size()

	// Write all current leaves
	for i, leaf := range t.nodes[0] {
		key := []byte("leaf:" + intToString(i))
		value, err := t.encoder(leaf)
		if err != nil {
			return err
		}
		if err := tx.Set(key, value); err != nil {
			return err
		}
	}

	// Clean up any leaves beyond current size
	// This handles the case where the tree has shrunk
	if err := t.cleanupStaleLeaves(tx, currentSize); err != nil {
		return err
	}

	// Update metadata
	sizeBytes := encodeInt(currentSize)
	if err := tx.Set([]byte("meta:size"), sizeBytes); err != nil {
		return err
	}

	// Set version for future migrations
	if err := tx.Set([]byte("meta:version"), []byte("1")); err != nil {
		return err
	}

	// Commit atomically
	if err := tx.Commit(); err != nil {
		return err
	}

	t.dirty = false
	return nil
}

// Close ensures all changes are synced and closes the database connection.
func (t *LeanIMT[N]) Close() error {
	if err := t.Sync(); err != nil {
		return err
	}
	if t.db != nil {
		return t.db.Close()
	}
	return nil
}

// rebuildTree reconstructs the internal tree structure from leaves.
func (t *LeanIMT[N]) rebuildTree() error {
	if len(t.nodes[0]) == 0 {
		return nil
	}

	// Calculate required depth
	size := len(t.nodes[0])
	depth := ceilLog2(size)

	// Save the leaves before reinitializing
	leaves := t.nodes[0]

	// Initialize all levels
	t.nodes = make([][]N, depth+1)
	t.nodes[0] = leaves // restore leaves

	// Build tree level by level
	for level := 0; level < depth; level++ {
		currentLevel := t.nodes[level]
		numParents := (len(currentLevel) + 1) / 2
		t.nodes[level+1] = make([]N, numParents)

		for i := 0; i < numParents; i++ {
			leftIdx := i * 2
			rightIdx := leftIdx + 1

			if rightIdx < len(currentLevel) {
				// Both children exist
				t.nodes[level+1][i] = t.hash(currentLevel[leftIdx], currentLevel[rightIdx])
			} else {
				// Only left child exists
				t.nodes[level+1][i] = currentLevel[leftIdx]
			}
		}
	}

	return nil
}

// cleanupStaleLeaves removes leaf entries beyond the current tree size.
func (t *LeanIMT[N]) cleanupStaleLeaves(tx db.WriteTx, currentSize int) error {
	// Get the previous size from database to know what to clean up
	sizeBytes, err := t.db.Get([]byte("meta:size"))
	if err != nil {
		if err == db.ErrKeyNotFound {
			return nil // no previous size, nothing to clean
		}
		return err
	}

	previousSize := decodeInt(sizeBytes)

	// Delete any leaves beyond current size
	for i := currentSize; i < previousSize; i++ {
		key := []byte("leaf:" + intToString(i))
		if err := tx.Delete(key); err != nil {
			return err
		}
	}

	return nil
}

// markDirty marks the tree as needing synchronization.
func (t *LeanIMT[N]) markDirty() {
	t.dirty = true
}

// encodeInt encodes an integer as bytes.
func encodeInt(n int) []byte {
	return []byte(intToString(n))
}

// decodeInt decodes bytes as an integer.
func decodeInt(b []byte) int {
	result := 0
	for _, digit := range b {
		if digit >= '0' && digit <= '9' {
			result = result*10 + int(digit-'0')
		}
	}
	return result
}
