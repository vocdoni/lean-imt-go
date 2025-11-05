package census

import (
	"errors"
	"fmt"
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
	Index   uint64         `json:"index"`   // Tree index (as packed bits)
	Address common.Address `json:"address"` // The address being proved
	Weight  *big.Int       `json:"weight"`  // The voting weight
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

// Errors
var (
	ErrAddressAlreadyExists = errors.New("address already exists in census")
	ErrAddressNotFound      = errors.New("address not found in census")
	ErrDataCorruption       = errors.New("census data corruption detected")
	ErrEmptyCensus          = errors.New("census is empty")
	ErrNotEmptyCensus       = errors.New("census is not empty")
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

// Dump exports the entire census state
func (c *CensusIMT) Dump() (*CensusDump, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Get the current root
	root, ok := c.tree.Root()
	if !ok {
		return nil, ErrEmptyCensus
	}

	// Iterate over indexes and address to build entries
	entries := []CensusParticipant{}
	totalWeight := big.NewInt(0)
	for index, hexAddr := range c.indexToAddress {
		weight := c.weights[hexAddr]
		addr := common.HexToAddress(hexAddr)
		entries = append(entries, CensusParticipant{
			Index:   uint64(index),
			Address: addr,
			Weight:  weight,
		})
		totalWeight.Add(totalWeight, weight)
	}

	// Return the dump with the current timestamp
	return &CensusDump{
		Root:              root,
		Participants:      entries,
		TotalWeight:       totalWeight,
		TotalParticipants: len(entries),
		Timestamp:         time.Now(),
	}, nil
}

func (c *CensusIMT) Import(dump *CensusDump) error {
	// Check if the current tree is empty
	if c.Size() != 0 {
		return ErrNotEmptyCensus
	}

	// Sort entries by index to ensure correct insertion order
	sortedEntries := make([]CensusParticipant, len(dump.Participants))
	copy(sortedEntries, dump.Participants)
	slices.SortFunc(sortedEntries, censusEntrySortFunc)

	// Add all entries from the dump
	addresses := make([]common.Address, len(dump.Participants))
	weights := make([]*big.Int, len(dump.Participants))
	for i, entry := range sortedEntries {
		addresses[i] = entry.Address
		weights[i] = entry.Weight
	}
	if err := c.AddBulk(addresses, weights); err != nil {
		return err
	}

	// Verify the root matches
	root, ok := c.tree.Root()
	if !ok {
		return fmt.Errorf("%w: imported census is empty", ErrEmptyCensus)
	}
	if root.Cmp(dump.Root) != 0 {
		return fmt.Errorf("%w: imported root does not match", ErrBadCensusDump)
	}
	return nil
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
