# Lean Incremental Merkle Tree (Go Implementation)

This is a Go implementation of the Lean Incremental Merkle Tree, originally developed by the [ZK-Kit team](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/packages/lean-imt). The original TypeScript implementation has been audited as part of the Semaphore V4 PSE audit.

The LeanIMT is an optimized binary version of traditional Incremental Merkle Trees (IMT), eliminating the need for zero values and allowing dynamic depth adjustment. Unlike standard IMTs that use zero hashes for incomplete nodes, the LeanIMT directly adopts the left child's value when a node lacks a right counterpart. The tree's depth dynamically adjusts to the count of leaves, enhancing efficiency by reducing the number of required hash calculations.

## 🚀 Features

- **High Performance**: Optimized for large-scale applications (tested with 20M+ leaves)
- **Thread-Safe**: Concurrent read/write operations with RWMutex protection
- **Persistent Storage**: Optional Pebble database backend for disk persistence
- **Parallel Insertion**: Batch operations with configurable goroutine pools
- **Generic Types**: Type-safe implementation with Go generics
- **Zero Dependencies**: Core functionality requires no external dependencies
- **Memory Efficient**: Optimized memory usage for cryptographic operations

## 📊 Performance

Our benchmarks demonstrate excellent performance for large-scale applications using Poseidon2 cryptographic hash:

- **Creation**: 87K leaves/second with Poseidon2 (10M leaves in ~114 seconds)
- **Updates**: 221K updates/second (4.5µs average latency)
- **Concurrency**: 295K concurrent operations/second
- **Persistence**: 20 seconds for 10M leaf save/load cycle
- **Memory**: ~19.5GB for 10M leaves with Poseidon2 hashing

## 🛠 Installation

```bash
go get github.com/vocdoni/lean-imt-go
```

## 📜 Usage

### Basic Usage

```go
package main

import (
    "fmt"
    "math/big"
    
    leanimt "github.com/vocdoni/lean-imt-go"
)

func main() {
    // Create a new tree with a simple hash function
    tree, err := leanimt.New(
        leanimt.BigIntHasher,     // Hash function
        leanimt.BigIntEqual,      // Equality function
        nil, nil, nil,            // No persistence
    )
    if err != nil {
        panic(err)
    }

    // Insert leaves
    err = tree.Insert(big.NewInt(1))
    if err != nil {
        panic(err)
    }
    
    err = tree.Insert(big.NewInt(3))
    if err != nil {
        panic(err)
    }

    fmt.Printf("Tree size: %d\n", tree.Size())        // 2
    fmt.Printf("Tree depth: %d\n", tree.Depth())      // 1
    
    root, exists := tree.Root()
    if exists {
        fmt.Printf("Root: %s\n", root.String())
    }

    // Check if tree contains a value
    has := tree.Has(big.NewInt(3))
    fmt.Printf("Contains 3: %t\n", has)               // true

    // Get index of a value
    index, found := tree.IndexOf(big.NewInt(3))
    if found {
        fmt.Printf("Index of 3: %d\n", index)         // 1
    }

    // Update a leaf
    err = tree.Update(1, big.NewInt(2))
    if err != nil {
        panic(err)
    }

    // Generate and verify proof
    proof, err := tree.GenerateProof(0)
    if err != nil {
        panic(err)
    }
    
    isValid := tree.VerifyProof(proof)
    fmt.Printf("Proof valid: %t\n", isValid)          // true
}
```

### Batch Operations

```go
// Insert many leaves at once (much faster)
leaves := make([]*big.Int, 1000000)
for i := 0; i < 1000000; i++ {
    leaves[i] = big.NewInt(int64(i))
}

err := tree.InsertMany(leaves)
if err != nil {
    panic(err)
}

fmt.Printf("Inserted %d leaves\n", tree.Size())
```

### With Poseidon2 Hash (Cryptographic)

```go
import leanimt "github.com/vocdoni/lean-imt-go"

// Create tree with cryptographic Poseidon2 hash
tree, err := leanimt.New(
    leanimt.Poseidon2Hasher,  // Cryptographic hash function
    leanimt.BigIntEqual,      // Equality function
    nil, nil, nil,            // No persistence
)
if err != nil {
    panic(err)
}

// Use the tree normally...
```

### With Persistence

```go
// Create tree with Pebble database persistence
tree, err := leanimt.NewWithPebble(
    leanimt.BigIntHasher,
    leanimt.BigIntEqual,
    leanimt.BigIntEncoder,    // Encoder function
    leanimt.BigIntDecoder,    // Decoder function
    "./tree_data",           // Database directory
)
if err != nil {
    panic(err)
}

// Insert data
err = tree.InsertMany(leaves)
if err != nil {
    panic(err)
}

// Sync to disk
err = tree.Sync()
if err != nil {
    panic(err)
}

// Close the tree
err = tree.Close()
if err != nil {
    panic(err)
}

// Reopen the tree (data is automatically loaded)
tree2, err := leanimt.NewWithPebble(
    leanimt.BigIntHasher,
    leanimt.BigIntEqual,
    leanimt.BigIntEncoder,
    leanimt.BigIntDecoder,
    "./tree_data",
)
if err != nil {
    panic(err)
}

fmt.Printf("Loaded tree size: %d\n", tree2.Size())
```

### Import/Export

```go
// Export tree data
data, err := tree.Export()
if err != nil {
    panic(err)
}

// Save to file
err = os.WriteFile("tree.json", data, 0644)
if err != nil {
    panic(err)
}

// Load from file
data, err = os.ReadFile("tree.json")
if err != nil {
    panic(err)
}

// Import tree data
tree2, err := leanimt.Import(
    leanimt.BigIntHasher,
    leanimt.BigIntEqual,
    data,
)
if err != nil {
    panic(err)
}
```

### Custom Types

```go
// Define custom hash and equality functions for strings
func stringHasher(a, b string) string {
    return fmt.Sprintf("hash(%s,%s)", a, b)
}

func stringEqual(a, b string) bool {
    return a == b
}

// Create tree with custom type
tree, err := leanimt.New(stringHasher, stringEqual, nil, nil, nil)
if err != nil {
    panic(err)
}

err = tree.Insert("hello")
if err != nil {
    panic(err)
}

err = tree.Insert("world")
if err != nil {
    panic(err)
}
```

## 📚 API Reference

### Core Functions

- `New(hash, eq, encoder, decoder, storage)` - Create a new tree
- `NewWithPebble(hash, eq, encoder, decoder, path)` - Create tree with persistence
- `Import(hash, eq, data)` - Import tree from JSON data

### Tree Operations

- `Insert(value)` - Insert a single leaf
- `InsertMany(values)` - Insert multiple leaves (batch operation)
- `Update(index, value)` - Update leaf at index
- `Has(value)` - Check if tree contains value
- `IndexOf(value)` - Get index of value
- `Size()` - Get number of leaves
- `Depth()` - Get tree depth
- `Root()` - Get tree root

### Proofs

- `GenerateProof(index)` - Generate Merkle proof for leaf
- `VerifyProof(proof)` - Verify a Merkle proof

### Persistence

- `Export()` - Export tree to JSON
- `Sync()` - Sync changes to disk (persistent trees)
- `Close()` - Close the tree and database

## 🔗 References

- **Original Implementation**: [ZK-Kit Lean IMT (TypeScript)](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/packages/lean-imt)
- **Research Paper**: [Lean IMT Paper](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/papers/leanimt)
- **ZK-Kit Project**: [Privacy Scaling Explorations](https://github.com/privacy-scaling-explorations/zk-kit)
- **Semaphore Audit**: [Semaphore V4 PSE Audit](https://semaphore.pse.dev/Semaphore_4.0.0_Audit.pdf)
