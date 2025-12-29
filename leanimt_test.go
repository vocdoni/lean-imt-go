package leanimt

import (
	"math/big"
	"testing"
)

func bigInt(n int64) *big.Int { return new(big.Int).SetInt64(n) }

func TestNewBigInt(t *testing.T) {
	tree, err := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if d := tree.Depth(); d != 0 {
		t.Fatalf("depth=%d, want 0", d)
	}
	if s := tree.Size(); s != 0 {
		t.Fatalf("size=%d, want 0", s)
	}
	if _, ok := tree.Root(); ok {
		t.Fatalf("root should be undefined for empty tree")
	}
}

func TestInitWithLeavesBigInt(t *testing.T) {
	for size := 100; size < 116; size++ {
		leaves := make([]*big.Int, size)
		for i := range leaves {
			leaves[i] = bigInt(int64(i))
		}
		tree1, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
		if err := tree1.InsertMany(leaves); err != nil {
			t.Fatal(err)
		}

		tree2, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
		for _, lf := range leaves {
			tree2.Insert(new(big.Int).Set(lf))
		}

		r1, _ := tree1.Root()
		r2, _ := tree2.Root()
		if r1.Cmp(r2) != 0 {
			t.Fatalf("roots differ")
		}
		wantDepth := ceilLog2(size)
		if tree1.Depth() != wantDepth {
			t.Fatalf("depth=%d, want=%d", tree1.Depth(), wantDepth)
		}
		if tree1.Size() != size {
			t.Fatalf("size=%d, want=%d", tree1.Size(), size)
		}
	}
}

func TestInsertManyFiveLeavesAndUpdates(t *testing.T) {
	const treeSize = 5
	leaves := make([]*big.Int, treeSize)
	for i := 0; i < treeSize; i++ {
		leaves[i] = bigInt(int64(i))
	}
	tree, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)

	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// Compute expected root like the TS test does (manual levels).
	h := bigIntHasher
	n1_0 := h(leaves[0], leaves[1])
	n1_1 := h(leaves[2], leaves[3])
	n2_0 := h(n1_0, n1_1)
	expected := h(n2_0, leaves[4])

	got, _ := tree.Root()
	if got.Cmp(expected) != 0 {
		t.Fatalf("root mismatch")
	}

	// Now update all to 0 and compare with recomputed expectation.
	for i := range treeSize {
		if err := tree.Update(i, bigInt(0)); err != nil {
			t.Fatal(err)
		}
	}
	n1 := h(bigInt(0), bigInt(0))
	n2 := h(n1, n1)
	expected2 := h(n2, bigInt(0))
	got, _ = tree.Root()
	if got.Cmp(expected2) != 0 {
		t.Fatalf("root mismatch after updates")
	}
}

func TestIndexHasAndUpdate(t *testing.T) {
	leaves := []*big.Int{bigInt(0), bigInt(1), bigInt(2), bigInt(3), bigInt(4)}
	tree, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	if idx := tree.IndexOf(bigInt(2)); idx != 2 {
		t.Fatalf("index=%d, want=2", idx)
	}
	if !tree.Has(bigInt(3)) || tree.Has(bigInt(99)) {
		t.Fatalf("has() mismatch")
	}

	// single update
	tree2, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree2.InsertMany([]*big.Int{bigInt(0), bigInt(1)}); err != nil {
		t.Fatal(err)
	}
	if err := tree2.Update(0, bigInt(2)); err != nil {
		t.Fatal(err)
	}
	r, _ := tree2.Root()
	if r.Cmp(bigIntHasher(bigInt(2), bigInt(1))) != 0 {
		t.Fatalf("unexpected root after single update")
	}
}

func TestUpdateManyValidationAndEquivalence(t *testing.T) {
	leaves := []*big.Int{bigInt(0), bigInt(1), bigInt(2), bigInt(3), bigInt(4)}
	tree, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// empty lists is no-op
	prev, _ := tree.Root()
	if err := tree.UpdateMany(nil, []*big.Int{}); err == nil {
		// We expect an error: indices not defined
	} else if err.Error() != "parameter 'indices' is not defined" {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := tree.UpdateMany([]int{}, []*big.Int{}); err != nil {
		t.Fatalf("empty update should be no-op: %v", err)
	}
	cur, _ := tree.Root()
	if cur.Cmp(prev) != 0 {
		t.Fatalf("root changed on empty update")
	}

	// range checks
	if err := tree.UpdateMany([]int{-1}, []*big.Int{bigInt(7)}); err == nil {
		t.Fatalf("expected out-of-range error")
	}
	if err := tree.UpdateMany([]int{len(leaves)}, []*big.Int{bigInt(7)}); err == nil {
		t.Fatalf("expected out-of-range error")
	}

	// duplicates
	if err := tree.UpdateMany([]int{1, 1}, []*big.Int{bigInt(8), bigInt(9)}); err == nil {
		t.Fatalf("expected duplicate error")
	}

	// equivalence with multiple Update calls
	treeA, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := treeA.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}
	treeB, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := treeB.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	indices := []int{0, 2, 4}
	values := []*big.Int{bigInt(10), bigInt(11), bigInt(12)}

	for i := range indices {
		if err := treeA.Update(indices[i], values[i]); err != nil {
			t.Fatal(err)
		}
	}
	if err := treeB.UpdateMany(indices, values); err != nil {
		t.Fatal(err)
	}

	ra, _ := treeA.Root()
	rb, _ := treeB.Root()
	if ra.Cmp(rb) != 0 {
		t.Fatalf("updateMany mismatch with repeated Update")
	}
}

func TestProofs(t *testing.T) {
	// single leaf proof
	one := []*big.Int{bigInt(1)}
	tree, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree.InsertMany(one); err != nil {
		t.Fatal(err)
	}
	pr, err := tree.GenerateProof(0)
	if err != nil {
		t.Fatal(err)
	}
	if pr.Index != 0 || len(pr.Siblings) != 0 {
		t.Fatalf("unexpected single-leaf proof")
	}
	if !tree.VerifyProof(pr) {
		t.Fatalf("expected proof to verify")
	}

	// 5 leaves, check every proof verifies
	leaves := []*big.Int{bigInt(0), bigInt(1), bigInt(2), bigInt(3), bigInt(4)}
	tree2, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree2.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(leaves); i++ {
		p, err := tree2.GenerateProof(i)
		if err != nil {
			t.Fatal(err)
		}
		if !tree2.VerifyProof(p) {
			t.Fatalf("proof %d did not verify", i)
		}
	}

	// malformed index
	if _, err := tree2.GenerateProof(999); err == nil {
		t.Fatalf("expected error for OOB index")
	}
}

func TestImportExportBigInt(t *testing.T) {
	leaves := []*big.Int{bigInt(0), bigInt(1), bigInt(2), bigInt(3), bigInt(4)}
	tree1, _ := New(bigIntHasher, BigIntEqual, nil, nil, nil)
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	jsonStr, err := tree1.Export()
	if err != nil {
		t.Fatal(err)
	}

	tree2, err := Import(bigIntHasher, jsonStr, BigIntEqual, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Do an insert on both, roots must match.
	tree1.Insert(bigInt(4))
	tree2.Insert(bigInt(4))
	r1, _ := tree1.Root()
	r2, _ := tree2.Root()
	if r1.Cmp(r2) != 0 {
		t.Fatalf("imported tree root mismatch")
	}
}
