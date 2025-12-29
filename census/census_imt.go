package census

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vocdoni/davinci-node/db"
	"github.com/vocdoni/davinci-node/db/metadb"
	leanimt "github.com/vocdoni/lean-imt-go"
)

// CensusIMT is a wrapper around LeanIMT for voting census management
// It stores address+weight pairs and provides efficient address-based lookups
type CensusIMT struct {
	tree           *leanimt.LeanIMT[*big.Int]
	hasher         leanimt.Hasher[*big.Int]
	addressIndex   map[string]int      // hex address -> tree index
	indexToAddress map[int]string      // tree index -> hex address
	weights        map[string]*big.Int // hex address -> weight
	db             db.Database         // optional persistence
	mu             sync.RWMutex
}

// CensusProof contains all data needed for census membership verification
type CensusProof struct {
	Root     *big.Int   // Merkle root
	Siblings []*big.Int // Merkle siblings
	CensusParticipant
}

// CensusParticipant includes the information of a census member, it can be used to
// export or import census data, but also as part of a CensusProof
type CensusParticipant struct {
	Index   uint64         `json:"index"`
	Address common.Address `json:"address"`
	Weight  *big.Int       `json:"weight"`
}

// CensusDump represents a full export of the census state. It can be used to
// import/export census data between nodes serialized as JSON.
type CensusDump struct {
	Root              *big.Int            `json:"root"`
	Timestamp         time.Time           `json:"timestamp"`
	TotalParticipants int                 `json:"totalEntries"`
	TotalWeight       *big.Int            `json:"totalWeight"`
	Participants      []CensusParticipant `json:"participants"`
}

// isEmptyParticipant returns true when the dump entry represents an empty slot.
// We need to consider both zero address and zero weight to avoid treating valid
// zero-address entries as empty during ImportAll/Import.
func isEmptyParticipant(p CensusParticipant) bool {
	if p.Address != (common.Address{}) {
		return false
	}
	if p.Weight == nil {
		return true
	}
	return p.Weight.Sign() == 0
}

// Errors
var (
	ErrAddressAlreadyExists = errors.New("address already exists in census")
	ErrAddressNotFound      = errors.New("address not found in census")
	ErrDataCorruption       = errors.New("census data corruption detected")
	ErrEmptyCensus          = errors.New("census is empty")
	ErrBadCensusDump        = errors.New("invalid census dump")
)

// NewCensusIMT creates a new census tree with the provided database
func NewCensusIMT(database db.Database, hasher leanimt.Hasher[*big.Int]) (*CensusIMT, error) {
	tree, err := leanimt.New(hasher, leanimt.BigIntEqual, database, leanimt.BigIntEncoder, leanimt.BigIntDecoder)
	if err != nil {
		return nil, err
	}

	census := &CensusIMT{
		tree:           tree,
		hasher:         hasher,
		addressIndex:   make(map[string]int),
		indexToAddress: make(map[int]string),
		weights:        make(map[string]*big.Int),
		db:             database,
	}

	// Load existing data
	if err := census.Load(); err != nil && err != db.ErrKeyNotFound {
		return nil, err
	}

	return census, nil
}

// NewCensusIMTWithPebble creates a census tree with Pebble persistence
func NewCensusIMTWithPebble(datadir string, hasher leanimt.Hasher[*big.Int]) (*CensusIMT, error) {
	database, err := metadb.New(db.TypePebble, datadir)
	if err != nil {
		return nil, err
	}

	return NewCensusIMT(database, hasher)
}

// Add adds an address with its voting weight to the census
func (c *CensusIMT) Add(address common.Address, weight *big.Int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	hexAddr := address.Hex()
	if _, exists := c.addressIndex[hexAddr]; exists {
		return ErrAddressAlreadyExists
	}

	// Pack address and weight
	packed := PackAddressWeight(address.Big(), weight)

	// Insert into tree
	c.tree.Insert(packed)

	// Update indices
	newIndex := c.tree.Size() - 1
	c.addressIndex[hexAddr] = newIndex
	c.indexToAddress[newIndex] = hexAddr
	c.weights[hexAddr] = new(big.Int).Set(weight)

	// Persist if database exists
	if c.db != nil {
		if err := c.persistEntry(hexAddr, newIndex, weight); err != nil {
			return err
		}
	}

	return nil
}

// AddBulk adds multiple addresses with their voting weights to the census in a single transaction
// This is more efficient than calling Add() multiple times as it batches database operations
func (c *CensusIMT) AddBulk(addresses []common.Address, weights []*big.Int) error {
	if len(addresses) != len(weights) {
		return errors.New("addresses and weights slices must have the same length")
	}

	if len(addresses) == 0 {
		return nil // Nothing to add
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Pre-validate all addresses don't already exist
	for _, address := range addresses {
		hexAddr := address.Hex()
		if _, exists := c.addressIndex[hexAddr]; exists {
			return fmt.Errorf("address %s already exists in census", hexAddr)
		}
	}

	// Prepare batch data
	packedValues := make([]*big.Int, len(addresses))
	hexAddrs := make([]string, len(addresses))

	for i, address := range addresses {
		hexAddrs[i] = address.Hex()
		packedValues[i] = PackAddressWeight(address.Big(), weights[i])
	}

	// Insert all values into tree
	startingIndex := c.tree.Size()
	for _, packed := range packedValues {
		c.tree.Insert(packed)
	}

	// Update in-memory indices
	for i, hexAddr := range hexAddrs {
		newIndex := startingIndex + i
		c.addressIndex[hexAddr] = newIndex
		c.indexToAddress[newIndex] = hexAddr
		c.weights[hexAddr] = new(big.Int).Set(weights[i])
	}

	// Persist all entries in a single transaction
	if c.db != nil {
		if err := c.persistBulkEntries(hexAddrs, weights, startingIndex); err != nil {
			return fmt.Errorf("failed to persist bulk entries: %w", err)
		}
	}

	return nil
}

// Update updates the voting weight for an existing address in the census. If
// the address does not exist, ErrAddressNotFound is returned.
func (c *CensusIMT) Update(address common.Address, newWeight *big.Int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Look up index
	hexAddr := address.Hex()
	// If not found, return error
	index, exists := c.addressIndex[hexAddr]
	if !exists {
		return ErrAddressNotFound
	}
	// Pack address and new weight
	packed := PackAddressWeight(address.Big(), newWeight)
	// Update tree at index
	if err := c.tree.Update(index, packed); err != nil {
		return err
	}
	// Update in-memory weight
	c.weights[hexAddr] = new(big.Int).Set(newWeight)
	// Persist updated weight if database exists
	if c.db != nil {
		if err := c.persistEntry(hexAddr, index, newWeight); err != nil {
			return err
		}
	}
	return nil
}

// GenerateProof generates a census proof for an address
func (c *CensusIMT) GenerateProof(address common.Address) (*CensusProof, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	hexAddr := address.Hex()

	// Look up index
	index, exists := c.addressIndex[hexAddr]
	if !exists {
		return nil, ErrAddressNotFound
	}

	// Get weight
	weight, exists := c.weights[hexAddr]
	if !exists {
		return nil, ErrDataCorruption
	}

	// Generate tree proof
	treeProof, err := c.tree.GenerateProof(index)
	if err != nil {
		return nil, err
	}

	return &CensusProof{
		Root: treeProof.Root,
		CensusParticipant: CensusParticipant{
			Index:   treeProof.Index,
			Address: address,
			Weight:  new(big.Int).Set(weight),
		},
		Siblings: treeProof.Siblings,
	}, nil
}

// Has checks if an address exists in the census
func (c *CensusIMT) Has(address common.Address) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.addressIndex[address.Hex()]
	return exists
}

// GetWeight returns the weight for an address
func (c *CensusIMT) GetWeight(address common.Address) (*big.Int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	weight, exists := c.weights[address.Hex()]
	if !exists {
		return nil, false
	}
	return new(big.Int).Set(weight), true
}

// Root returns the merkle root
func (c *CensusIMT) Root() (*big.Int, bool) {
	return c.tree.Root()
}

// Size returns the number of census members
func (c *CensusIMT) Size() int {
	return c.tree.Size()
}

// Dump returns an io.Reader that streams all census entries in JSON Lines format.
// Each line contains a JSON object with "address" and "weight" fields.
// The reader is safe to use concurrently with other census operations.
// For large censuses (millions of entries), consider using DumpRange for pagination.
func (c *CensusIMT) Dump() io.Reader {
	return c.DumpRange(0, -1)
}

// DumpRange returns an io.Reader that streams census entries in the specified range.
// Entries are returned in JSON Lines format (one JSON object per line).
// Parameters:
//   - offset: starting index (0-based), negative values are treated as 0
//   - limit: maximum number of entries to return, -1 means unlimited
//
// The method automatically optimizes based on range size:
//   - Small ranges (≤10000): single lock, snapshot entire range
//   - Large ranges: batched streaming with brief locks per batch
//
// Returns entries from [offset, min(offset+limit, size)).
// The reader is safe to use concurrently with other census operations.
func (c *CensusIMT) DumpRange(offset, limit int) io.Reader {
	pr, pw := io.Pipe()

	go func() {
		defer func() {
			_ = pw.Close()
		}()

		c.mu.RLock()
		size := c.tree.Size()
		c.mu.RUnlock()

		if offset < 0 {
			offset = 0
		}
		if offset >= size {
			return
		}

		end := size
		if limit >= 0 {
			end = min(offset+limit, size)
		}

		rangeSize := end - offset
		const batchThreshold = 10000

		if limit >= 0 && rangeSize <= batchThreshold {
			c.dumpRangeSingleLock(pw, offset, end)
		} else {
			c.dumpRangeBatched(pw, offset, end)
		}
	}()

	return pr
}

// dumpRangeSingleLock fetches the entire range under a single lock.
// Used for small bounded ranges to minimize lock overhead.
func (c *CensusIMT) dumpRangeSingleLock(pw *io.PipeWriter, start, end int) {
	c.mu.RLock()
	entries := make([]CensusParticipant, 0, end-start)
	for i := start; i < end; i++ {
		addr, exists := c.indexToAddress[i]
		var participant CensusParticipant
		if !exists {
			// Empty entry (gap in tree)
			participant = CensusParticipant{
				Index:   uint64(i),
				Address: common.Address{},
				Weight:  big.NewInt(0),
			}
		} else {
			weight, exists := c.weights[addr]
			if !exists {
				c.mu.RUnlock()
				pw.CloseWithError(fmt.Errorf("data corruption: missing weight for %s", addr))
				return
			}
			participant = CensusParticipant{
				Index:   uint64(i),
				Address: common.HexToAddress(addr),
				Weight:  weight,
			}
		}
		entries = append(entries, participant)
	}
	c.mu.RUnlock()

	encoder := json.NewEncoder(pw)
	for i := range entries {
		if err := encoder.Encode(&entries[i]); err != nil {
			pw.CloseWithError(err)
			return
		}
	}
}

// dumpRangeBatched streams entries in batches with brief locks per batch.
// Used for large or unlimited ranges to minimize lock contention.
func (c *CensusIMT) dumpRangeBatched(pw *io.PipeWriter, start, end int) {
	const batchSize = 1000
	encoder := json.NewEncoder(pw)

	for i := start; i < end; i += batchSize {
		batchEnd := min(i+batchSize, end)

		c.mu.RLock()
		batch := make([]CensusParticipant, 0, batchEnd-i)
		for j := i; j < batchEnd; j++ {
			addr, exists := c.indexToAddress[j]
			var participant CensusParticipant
			if !exists {
				// Empty entry (gap in tree)
				participant = CensusParticipant{
					Index:   uint64(j),
					Address: common.Address{},
					Weight:  big.NewInt(0),
				}
			} else {
				weight, exists := c.weights[addr]
				if !exists {
					c.mu.RUnlock()
					pw.CloseWithError(fmt.Errorf("data corruption: missing weight for %s", addr))
					return
				}
				participant = CensusParticipant{
					Index:   uint64(j),
					Address: common.HexToAddress(addr),
					Weight:  weight,
				}
			}
			batch = append(batch, participant)
		}
		c.mu.RUnlock()

		for k := range batch {
			if err := encoder.Encode(&batch[k]); err != nil {
				pw.CloseWithError(err)
				return
			}
		}
	}
}

// persistEntry saves a single entry atomically
func (c *CensusIMT) persistEntry(hexAddr string, index int, weight *big.Int) error {
	tx := c.db.WriteTx()
	defer tx.Discard()

	// Save index mapping
	if err := tx.Set([]byte("idx:addr:"+hexAddr), encodeInt(index)); err != nil {
		return err
	}

	// Save reverse mapping
	if err := tx.Set([]byte("idx:rev:"+intToString(index)), []byte(hexAddr)); err != nil {
		return err
	}

	// Save weight
	if err := tx.Set([]byte("weight:"+hexAddr), weight.Bytes()); err != nil {
		return err
	}

	// Update census size
	if err := tx.Set([]byte("meta:census_size"), encodeInt(c.tree.Size())); err != nil {
		return err
	}

	return tx.Commit()
}

// persistBulkEntries saves multiple entries in a single transaction
func (c *CensusIMT) persistBulkEntries(hexAddrs []string, weights []*big.Int, startingIndex int) error {
	tx := c.db.WriteTx()
	defer tx.Discard()

	// Save all entries in the transaction
	for i, hexAddr := range hexAddrs {
		index := startingIndex + i

		// Save index mapping
		if err := tx.Set([]byte("idx:addr:"+hexAddr), encodeInt(index)); err != nil {
			return err
		}

		// Save reverse mapping
		if err := tx.Set([]byte("idx:rev:"+intToString(index)), []byte(hexAddr)); err != nil {
			return err
		}

		// Save weight
		if err := tx.Set([]byte("weight:"+hexAddr), weights[i].Bytes()); err != nil {
			return err
		}
	}

	// Update census size once at the end
	if err := tx.Set([]byte("meta:census_size"), encodeInt(c.tree.Size())); err != nil {
		return err
	}

	return tx.Commit()
}

// Load restores the census from disk
func (c *CensusIMT) Load() error {
	if c.db == nil {
		return nil
	}

	// Load census size
	sizeBytes, err := c.db.Get([]byte("meta:census_size"))
	if err != nil {
		if err == db.ErrKeyNotFound {
			return nil // Empty census
		}
		return err
	}

	censusSize := decodeInt(sizeBytes)

	// Load all reverse mappings to rebuild indices
	for i := range censusSize {
		// Get address for this index
		addrBytes, err := c.db.Get([]byte("idx:rev:" + intToString(i)))
		if err != nil {
			return fmt.Errorf("corrupted index %d: %w", i, err)
		}

		hexAddr := string(addrBytes)

		// Load weight
		weightBytes, err := c.db.Get([]byte("weight:" + hexAddr))
		if err != nil {
			return fmt.Errorf("missing weight for %s: %w", hexAddr, err)
		}

		// Rebuild in-memory indices
		c.addressIndex[hexAddr] = i
		c.indexToAddress[i] = hexAddr
		c.weights[hexAddr] = new(big.Int).SetBytes(weightBytes)
	}

	return nil
}

// Sync ensures all data is persisted to disk
func (c *CensusIMT) Sync() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Sync the tree
	if err := c.tree.Sync(); err != nil {
		return err
	}

	// Our indices are already persisted on each Add
	return nil
}

// Close cleanly shuts down the census
func (c *CensusIMT) Close() error {
	if err := c.Sync(); err != nil {
		return err
	}

	if c.tree != nil {
		if err := c.tree.Close(); err != nil {
			return err
		}
	}

	return nil
}

// DumpAll exports the entire census state as a CensusDump structure.
// This includes all participants (including empty entries), metadata, and the merkle root.
// For large censuses, consider using Dump() for streaming export instead.
func (c *CensusIMT) DumpAll() (*CensusDump, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	root, ok := c.tree.Root()
	if !ok {
		return nil, ErrEmptyCensus
	}

	size := c.tree.Size()
	participants := make([]CensusParticipant, 0, size)
	totalWeight := big.NewInt(0)
	nonEmptyCount := 0

	for i := range size {
		addr, exists := c.indexToAddress[i]
		if !exists {
			// Empty entry
			participants = append(participants, CensusParticipant{
				Index:   uint64(i),
				Address: common.Address{},
				Weight:  big.NewInt(0),
			})
		} else {
			weight := c.weights[addr]
			participants = append(participants, CensusParticipant{
				Index:   uint64(i),
				Address: common.HexToAddress(addr),
				Weight:  weight,
			})
			totalWeight.Add(totalWeight, weight)
			nonEmptyCount++
		}
	}

	return &CensusDump{
		Root:              root,
		Participants:      participants,
		TotalWeight:       totalWeight,
		TotalParticipants: nonEmptyCount,
		Timestamp:         time.Now(),
	}, nil
}

// ImportAll imports a complete census dump, replacing any existing census data.
// The import validates that the resulting merkle root matches the dump's root.
// This method will clear any existing census data before importing.
func (c *CensusIMT) ImportAll(dump *CensusDump) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset state to prevent conflicts
	if err := c.resetPersistentState(); err != nil {
		return err
	}

	// Clear existing data
	c.addressIndex = make(map[string]int)
	c.indexToAddress = make(map[int]string)
	c.weights = make(map[string]*big.Int)

	// Recreate tree
	var err error
	c.tree, err = leanimt.New(c.hasher, leanimt.BigIntEqual, c.db, leanimt.BigIntEncoder, leanimt.BigIntDecoder)
	if err != nil {
		return err
	}

	// Sort entries by index to ensure correct insertion order
	participants := make([]CensusParticipant, len(dump.Participants))
	copy(participants, dump.Participants)
	slices.SortFunc(participants, censusEntrySortFunc)

	// Track expected index for validation
	expectedIndex := uint64(0)
	weights := []*big.Int{}
	hexAddrs := []string{}

	for _, p := range participants {
		// Fill gaps with empty entries if needed
		for expectedIndex < p.Index {
			c.tree.Insert(big.NewInt(0))
			expectedIndex++
		}

		// Check if this is an empty entry
		if isEmptyParticipant(p) {
			// Insert zero value for empty entry
			c.tree.Insert(big.NewInt(0))
		} else {
			// Insert actual participant
			packed := PackAddressWeight(p.Address.Big(), p.Weight)
			c.tree.Insert(packed)

			// Track for maps and persistence
			hexAddr := p.Address.Hex()
			c.addressIndex[hexAddr] = int(p.Index)
			c.indexToAddress[int(p.Index)] = hexAddr
			c.weights[hexAddr] = new(big.Int).Set(p.Weight)

			hexAddrs = append(hexAddrs, hexAddr)
			weights = append(weights, p.Weight)
		}
		expectedIndex++
	}

	// Verify root matches
	root, ok := c.tree.Root()
	if !ok {
		return fmt.Errorf("%w: imported census is empty", ErrEmptyCensus)
	}
	if root.Cmp(dump.Root) != 0 {
		return fmt.Errorf("%w: imported root does not match (expected %s, got %s)",
			ErrBadCensusDump, dump.Root.String(), root.String())
	}

	// Persist if database exists
	if c.db != nil {
		if err := c.persistImportedData(hexAddrs, weights); err != nil {
			return fmt.Errorf("failed to persist imported data: %w", err)
		}
	}

	return nil
}

// Import imports census data from a JSON Lines stream (io.Reader).
// Each line should contain a CensusParticipant JSON object.
// This method will replace any existing census data.
// Note: Unlike ImportAll, this method does not verify the merkle root since
// the stream format doesn't include it. Use ImportAll for root verification.
func (c *CensusIMT) Import(root *big.Int, reader io.Reader) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Reset state to prevent conflicts
	if err := c.resetPersistentState(); err != nil {
		return err
	}

	// Clear existing data
	c.addressIndex = make(map[string]int)
	c.indexToAddress = make(map[int]string)
	c.weights = make(map[string]*big.Int)

	// Recreate tree
	var err error
	c.tree, err = leanimt.New(c.hasher, leanimt.BigIntEqual, c.db, leanimt.BigIntEncoder, leanimt.BigIntDecoder)
	if err != nil {
		return err
	}

	// Read and sort participants
	decoder := json.NewDecoder(reader)
	participants := []CensusParticipant{}

	for decoder.More() {
		var p CensusParticipant
		if err := decoder.Decode(&p); err != nil {
			return fmt.Errorf("failed to decode participant: %w", err)
		}
		participants = append(participants, p)
	}

	if len(participants) == 0 {
		return ErrEmptyCensus
	}

	// Sort by index
	slices.SortFunc(participants, censusEntrySortFunc)

	// Insert participants
	expectedIndex := uint64(0)
	hexAddrs := []string{}
	weights := []*big.Int{}

	for _, p := range participants {
		// Fill gaps with empty entries if needed
		for expectedIndex < p.Index {
			c.tree.Insert(big.NewInt(0))
			expectedIndex++
		}

		// Check if this is an empty entry
		if isEmptyParticipant(p) {
			c.tree.Insert(big.NewInt(0))
		} else {
			packed := PackAddressWeight(p.Address.Big(), p.Weight)
			c.tree.Insert(packed)

			hexAddr := p.Address.Hex()
			c.addressIndex[hexAddr] = int(p.Index)
			c.indexToAddress[int(p.Index)] = hexAddr
			c.weights[hexAddr] = new(big.Int).Set(p.Weight)

			hexAddrs = append(hexAddrs, hexAddr)
			weights = append(weights, new(big.Int).Set(p.Weight))
		}
		expectedIndex++
	}

	// Verify root matches
	newRoot, ok := c.tree.Root()
	if !ok {
		return fmt.Errorf("%w: imported census is empty", ErrEmptyCensus)
	}
	if root.Cmp(newRoot) != 0 {
		return fmt.Errorf("%w: imported root does not match (expected %s, got %s)",
			ErrBadCensusDump, root.String(), newRoot.String())
	}

	// Persist if database exists
	if c.db != nil {
		if err := c.persistImportedData(hexAddrs, weights); err != nil {
			return fmt.Errorf("failed to persist imported data: %w", err)
		}
	}

	return nil
}

// persistImportedData saves all imported data in a single transaction
func (c *CensusIMT) persistImportedData(hexAddrs []string, weights []*big.Int) error {
	tx := c.db.WriteTx()
	defer tx.Discard()

	// Save all entries
	for i, hexAddr := range hexAddrs {
		index := c.addressIndex[hexAddr]

		// Save index mapping
		if err := tx.Set([]byte("idx:addr:"+hexAddr), encodeInt(index)); err != nil {
			return err
		}

		// Save reverse mapping
		if err := tx.Set([]byte("idx:rev:"+intToString(index)), []byte(hexAddr)); err != nil {
			return err
		}

		// Save weight
		if err := tx.Set([]byte("weight:"+hexAddr), weights[i].Bytes()); err != nil {
			return err
		}
	}

	// Update census size
	if err := tx.Set([]byte("meta:census_size"), encodeInt(c.tree.Size())); err != nil {
		return err
	}

	return tx.Commit()
}

// resetPersistentState removes any previously persisted census and tree data so imports
// start from a clean slate. Without this, a persisted tree would be loaded by
// leanimt.New and new leaves would be appended after the old ones, yielding a
// different root even if the imported participants are identical.
func (c *CensusIMT) resetPersistentState() error {
	if c.db == nil {
		return nil
	}

	tx := c.db.WriteTx()
	defer tx.Discard()

	// Remove tree leaves using the current in-memory size when available.
	treeSize := 0
	if c.tree != nil {
		treeSize = c.tree.Size()
	} else {
		if sizeBytes, err := c.db.Get([]byte("meta:size")); err == nil {
			treeSize = decodeInt(sizeBytes)
		} else if err != db.ErrKeyNotFound {
			return err
		}
	}

	for i := 0; i < treeSize; i++ {
		if err := tx.Delete([]byte("leaf:" + intToString(i))); err != nil && err != db.ErrKeyNotFound {
			return err
		}
	}

	// Clear tree and census metadata.
	metaKeys := [][]byte{
		[]byte("meta:size"),
		[]byte("meta:version"),
		[]byte("meta:census_size"),
	}
	for _, key := range metaKeys {
		if err := tx.Delete(key); err != nil && err != db.ErrKeyNotFound {
			return err
		}
	}

	// Clear index and weight entries we know about from the current census.
	for addr := range c.addressIndex {
		if err := tx.Delete([]byte("idx:addr:" + addr)); err != nil && err != db.ErrKeyNotFound {
			return err
		}
		if err := tx.Delete([]byte("weight:" + addr)); err != nil && err != db.ErrKeyNotFound {
			return err
		}
	}
	for idx := range c.indexToAddress {
		if err := tx.Delete([]byte("idx:rev:" + intToString(idx))); err != nil && err != db.ErrKeyNotFound {
			return err
		}
	}

	return tx.Commit()
}

// Helper functions for integer encoding/decoding
func encodeInt(n int) []byte {
	return []byte(intToString(n))
}

func decodeInt(b []byte) int {
	result := 0
	for _, digit := range b {
		if digit >= '0' && digit <= '9' {
			result = result*10 + int(digit-'0')
		}
	}
	return result
}

func intToString(x int) string {
	if x >= 0 && x < 10 {
		return string('0' + byte(x))
	}
	// fallback for larger numbers
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

// censusEntrySortFunc helper function returns -1, 0, or 1 based on the
// comparison of two CensusEntry by Index. It is used for sorting CensusEntry
// slices.
func censusEntrySortFunc(a, b CensusParticipant) int {
	if a.Index < b.Index {
		return -1
	} else if a.Index > b.Index {
		return 1
	}
	return 0
}
