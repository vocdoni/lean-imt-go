package census

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"sync"

	"github.com/ethereum/go-ethereum/common"
	"github.com/vocdoni/davinci-node/db"
	"github.com/vocdoni/davinci-node/db/metadb"
	leanimt "github.com/vocdoni/lean-imt-go"
)

// CensusIMT is a wrapper around LeanIMT for voting census management
// It stores address+weight pairs and provides efficient address-based lookups
type CensusIMT struct {
	tree           *leanimt.LeanIMT[*big.Int]
	addressIndex   map[string]int      // hex address -> tree index
	indexToAddress map[int]string      // tree index -> hex address
	weights        map[string]*big.Int // hex address -> weight
	db             db.Database         // optional persistence
	mu             sync.RWMutex
}

// CensusProof contains all data needed for census membership verification
type CensusProof struct {
	Root     *big.Int       // Merkle root
	Address  common.Address // The address being proved
	Weight   *big.Int       // The voting weight
	Index    uint64         // Tree index (as packed bits)
	Siblings []*big.Int     // Merkle siblings
}

// CensusEntry represents a single census entry for export operations
type CensusEntry struct {
	Address string `json:"address"`
	Weight  string `json:"weight"`
}

// Errors
var (
	ErrAddressAlreadyExists = errors.New("address already exists in census")
	ErrAddressNotFound      = errors.New("address not found in census")
	ErrDataCorruption       = errors.New("census data corruption detected")
)

// NewCensusIMT creates a new census tree with the provided database
func NewCensusIMT(database db.Database, hasher leanimt.Hasher[*big.Int]) (*CensusIMT, error) {
	tree, err := leanimt.New(hasher, leanimt.BigIntEqual, database, leanimt.BigIntEncoder, leanimt.BigIntDecoder)
	if err != nil {
		return nil, err
	}

	census := &CensusIMT{
		tree:           tree,
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
	if err := c.tree.Insert(packed); err != nil {
		return err
	}

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
		if err := c.tree.Insert(packed); err != nil {
			return fmt.Errorf("failed to insert into tree: %w", err)
		}
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
		Root:     treeProof.Root,
		Address:  address,
		Weight:   new(big.Int).Set(weight),
		Index:    treeProof.Index,
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
			end = offset + limit
			if end > size {
				end = size
			}
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
	entries := make([]CensusEntry, 0, end-start)
	for i := start; i < end; i++ {
		addr, exists := c.indexToAddress[i]
		if !exists {
			c.mu.RUnlock()
			pw.CloseWithError(fmt.Errorf("data corruption: missing index %d", i))
			return
		}
		weight, exists := c.weights[addr]
		if !exists {
			c.mu.RUnlock()
			pw.CloseWithError(fmt.Errorf("data corruption: missing weight for %s", addr))
			return
		}
		entries = append(entries, CensusEntry{
			Address: addr,
			Weight:  weight.String(),
		})
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
		batchEnd := i + batchSize
		if batchEnd > end {
			batchEnd = end
		}

		c.mu.RLock()
		batch := make([]CensusEntry, 0, batchEnd-i)
		for j := i; j < batchEnd; j++ {
			addr, exists := c.indexToAddress[j]
			if !exists {
				c.mu.RUnlock()
				pw.CloseWithError(fmt.Errorf("data corruption: missing index %d", j))
				return
			}
			weight, exists := c.weights[addr]
			if !exists {
				c.mu.RUnlock()
				pw.CloseWithError(fmt.Errorf("data corruption: missing weight for %s", addr))
				return
			}
			batch = append(batch, CensusEntry{
				Address: addr,
				Weight:  weight.String(),
			})
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
	for i := 0; i < censusSize; i++ {
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
