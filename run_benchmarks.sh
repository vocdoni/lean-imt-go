#!/bin/bash

echo "=== Lean IMT Large-Scale Benchmarks ==="
echo "Testing practical usability of 20M leaf Merkle trees"
echo ""

echo "1. Testing 20M leaf tree creation with simple hash..."
go test -v -run=^$ -bench=BenchmarkLargeTree_20M_Creation -benchtime=1x

echo ""
echo "2. Testing 10M leaf tree creation with Poseidon2 hash..."
go test -v -run=^$ -bench=BenchmarkLargeTree_10M_Poseidon2_Creation -benchtime=1x

echo ""
echo "3. Testing 20M leaf tree persistence (save/load)..."
go test -v -run=^$ -bench=BenchmarkLargeTree_20M_Persistence -benchtime=1x

echo ""
echo "4. Testing 20M leaf tree updates..."
go test -v -run=^$ -bench=BenchmarkLargeTree_20M_Updates -benchtime=1x

echo ""
echo "5. Testing 20M leaf tree concurrent operations..."
go test -v -run=^$ -bench=BenchmarkLargeTree_20M_Concurrent -benchtime=1x

echo ""
echo "6. Memory usage benchmark for 20M leaves..."
go test -v -run=^$ -bench=BenchmarkMemoryUsage_20M -benchtime=1x

echo ""
echo "=== Comparison with smaller trees ==="
echo ""

echo "7. Testing 1M leaf tree creation (for comparison)..."
go test -v -run=^$ -bench=BenchmarkInsertMany_SimpleHash_1M -benchtime=1x

echo ""
echo "8. Testing 1M leaf tree with Poseidon2 (for comparison)..."
go test -v -run=^$ -bench=BenchmarkInsertMany_Poseidon2_1M -benchtime=1x

echo ""
echo "=== Benchmarks completed ==="
