# Lean Incremental Merkle Tree (Go Implementation)

This is a Go implementation of the Lean Incremental Merkle Tree, originally developed by the [ZK-Kit team](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/packages/lean-imt). The original TypeScript implementation has been audited as part of the Semaphore V4 PSE audit.

The LeanIMT is an optimized binary version of traditional Incremental Merkle Trees (IMT), eliminating the need for zero values and allowing dynamic depth adjustment. Unlike standard IMTs that use zero hashes for incomplete nodes, the LeanIMT directly adopts the left child's value when a node lacks a right counterpart. The tree's depth dynamically adjusts to the count of leaves, enhancing efficiency by reducing the number of required hash calculations.

A compatible Solidity implementation is available at [zk-kit.solidity](https://github.com/zk-kit/zk-kit.solidity/tree/main/packages/lean-imt). Which uses [poseidon-solidity](https://github.com/chancehudson/poseidon-solidity) for hashing, an optimized version of Poseidon consuming ~20k gas.

## Features

- **High Performance**: Optimized for large-scale applications (tested with 20M+ leaves)
- **Thread-Safe**: Concurrent read/write operations with RWMutex protection
- **Persistent Storage**: Optional Pebble database backend for disk persistence
- **Parallel Insertion**: Batch operations with configurable goroutine pools
- **Generic Types**: Type-safe implementation with Go generics
- **Zero Dependencies**: Core functionality requires no external dependencies
- **Memory Efficient**: Optimized memory usage for cryptographic operations
- **Gnark zk-SNARK Circuit**: Built-in circuit for verifying Merkle proofs in zero-knowledge proofs

## Installation

```bash
go get github.com/vocdoni/lean-imt-go
```

## Usage

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
    index := tree.IndexOf(big.NewInt(3))
    if index > -1 {
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

### With Poseidon Hash (Cryptographic)

```go
import leanimt "github.com/vocdoni/lean-imt-go"

// Create tree with cryptographic Poseidon hash
tree, err := leanimt.New(
    leanimt.PoseidonHasher,   // Cryptographic hash function
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

## Census Package

The `census` package provides a voting census implementation using Lean IMT for efficient address-weight storage with zero-knowledge proof support. It packs Ethereum addresses (160 bits) and voting weights (88 bits) into single 248-bit values that fit safely within the BN254 scalar field (~254 bits) for circuit compatibility.

The packing scheme combines address and weight into a single tree leaf: `packed = (address << 88) | weight`.

### Usage

```go
import "github.com/vocdoni/lean-imt-go/census"

// Create census with database persistence
census, err := census.NewCensusIMTWithPebble("./census_data")
if err != nil {
    panic(err)
}
defer census.Close()

// Add single address
addr := common.HexToAddress("0x742d35Cc6634C0532925a3b844Bc9e7595f0bEb7")
weight := big.NewInt(1000)
err = census.Add(addr, weight)
if err != nil {
    panic(err)
}

// Bulk add multiple addresses (more efficient)
addresses := []common.Address{
    common.HexToAddress("0x8ba1f109551bD432803012645Hac136c22C177ec"),
    common.HexToAddress("0x1234567890123456789012345678901234567890"),
}
weights := []*big.Int{big.NewInt(250), big.NewInt(75)}
err = census.AddBulk(addresses, weights)
if err != nil {
    panic(err)
}

// Generate proof for circuit verification
proof, err := census.GenerateProof(addr)
if err != nil {
    panic(err)
}

fmt.Printf("Census size: %d\n", census.Size())
fmt.Printf("Root: %s\n", proof.Root.String())
```

## Gnark Circuit

The `circuit` package provides zero-knowledge proof verification of Lean IMT Merkle proofs using Gnark. It includes both generic proof verification and census-specific verification with address-weight packing.

The circuit uses `github.com/vocdoni/gnark-crypto-primitives/hash/bn254/poseidon` for hashing.

### Basic Proof Verification

```go
func (myCircuit *MyCircuit) Define(api frontend.API) error {
    isValid, err := circuit.VerifyLeanIMTProof(
        api,
        myCircuit.MerkleRoot,
        myCircuit.LeafValue,
        myCircuit.LeafIndex,
        myCircuit.ProofSiblings,
    )
    if err != nil {
        return err
    }
    
    // Assert proof is valid
    api.AssertIsEqual(isValid, 1)
    return nil
}
```

### Census Proof Verification

```go
func (votingCircuit *VotingCircuit) Define(api frontend.API) error {
    // Verify census membership
    isValid, err := circuit.VerifyCensusProof(
        api,
        votingCircuit.CensusRoot,
        votingCircuit.VoterAddress,
        votingCircuit.Weight,
        votingCircuit.Index,
        votingCircuit.Siblings,
    )
    if err != nil {
        return err
    }
    
    // Assert proof is valid (or use isValid in other logic)
    api.AssertIsEqual(isValid, 1)
    return nil
}
```

### Constraints

| Max Depth | Constraints | Variables | Scaling Rate |
|-----------|-------------|-----------|--------------|
| 3         | 745         | 747       | Base         |
| 5         | 1,239       | 1,241     | +247/level   |
| 8         | 1,980       | 1,982     | +247/level   |
| 10        | 2,474       | 2,476     | +247/level   |


## 🔗 References

- **Original Implementation**: [ZK-Kit Lean IMT (TypeScript)](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/packages/lean-imt)
- **Research Paper**: [Lean IMT Paper](https://github.com/privacy-scaling-explorations/zk-kit/tree/main/papers/leanimt)
- **ZK-Kit Project**: [Privacy Scaling Explorations](https://github.com/privacy-scaling-explorations/zk-kit)
- **Semaphore Audit**: [Semaphore V4 PSE Audit](https://semaphore.pse.dev/Semaphore_4.0.0_Audit.pdf)
