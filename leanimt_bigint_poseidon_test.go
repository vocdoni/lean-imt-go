package leanimt

import (
	"math/big"
	"testing"

	iden3poseidon "github.com/iden3/go-iden3-crypto/poseidon"
)

// poseidonBig hash for *big.Int using iden3 poseidon.
func poseidonBig(a, b *big.Int) *big.Int {
	out, err := iden3poseidon.Hash([]*big.Int{a, b})
	if err != nil {
		panic(err)
	}
	return out
}

func BigIntEqualPoseidon(a, b *big.Int) bool { return a.Cmp(b) == 0 }

func bigIntPoseidon(n int64) *big.Int { return new(big.Int).SetInt64(n) }

func TestInitEmptyBigInt(t *testing.T) {
	tree, err := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
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

func TestInitWithLeavesConsistencyBigInt(t *testing.T) {
	for size := 100; size < 116; size++ {
		leaves := make([]*big.Int, size)
		for i := range leaves {
			leaves[i] = bigIntPoseidon(int64(i))
		}
		tree1, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
		if err := tree1.InsertMany(leaves); err != nil {
			t.Fatal(err)
		}

		tree2, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
		for _, lf := range leaves {
			if err := tree2.Insert(new(big.Int).Set(lf)); err != nil {
				t.Fatal(err)
			}
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

func TestInsertManyFiveLeavesAndUpdatesPoseidon(t *testing.T) {
	const treeSize = 5
	leaves := make([]*big.Int, treeSize)
	for i := 0; i < treeSize; i++ {
		leaves[i] = bigIntPoseidon(int64(i))
	}
	tree, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)

	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// Expected root after 5 insertions (compute like TS test)
	n1_0 := poseidonBig(leaves[0], leaves[1])
	n1_1 := poseidonBig(leaves[2], leaves[3])
	n2_0 := poseidonBig(n1_0, n1_1)
	expected := poseidonBig(n2_0, leaves[4])

	got, _ := tree.Root()
	if got.Cmp(expected) != 0 {
		t.Fatalf("root mismatch")
	}

	// Update all to 0 and check the expected root.
	for i := range treeSize {
		if err := tree.Update(i, bigIntPoseidon(0)); err != nil {
			t.Fatal(err)
		}
	}
	n1 := poseidonBig(bigIntPoseidon(0), bigIntPoseidon(0))
	n2 := poseidonBig(n1, n1)
	expected2 := poseidonBig(n2, bigIntPoseidon(0))
	got, _ = tree.Root()
	if got.Cmp(expected2) != 0 {
		t.Fatalf("root mismatch after updates")
	}
}

func TestIndexHasAndSingleUpdateBigInt(t *testing.T) {
	leaves := []*big.Int{bigIntPoseidon(0), bigIntPoseidon(1), bigIntPoseidon(2), bigIntPoseidon(3), bigIntPoseidon(4)}
	tree, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	if idx := tree.IndexOf(bigIntPoseidon(2)); idx != 2 {
		t.Fatalf("index=%d, want=2", idx)
	}
	if !tree.Has(bigIntPoseidon(3)) || tree.Has(bigIntPoseidon(99)) {
		t.Fatalf("has() mismatch")
	}

	// single update
	tree2, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := tree2.InsertMany([]*big.Int{bigIntPoseidon(0), bigIntPoseidon(1)}); err != nil {
		t.Fatal(err)
	}
	if err := tree2.Update(0, bigIntPoseidon(2)); err != nil {
		t.Fatal(err)
	}
	r, _ := tree2.Root()
	if r.Cmp(poseidonBig(bigIntPoseidon(2), bigIntPoseidon(1))) != 0 {
		t.Fatalf("unexpected root after single update")
	}
}

func TestUpdateManyValidationBigInt(t *testing.T) {
	leaves := []*big.Int{bigIntPoseidon(0), bigIntPoseidon(1), bigIntPoseidon(2), bigIntPoseidon(3), bigIntPoseidon(4)}
	tree, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := tree.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	// empty lists is no-op
	prev, _ := tree.Root()
	if err := tree.UpdateMany([]int{}, []*big.Int{}); err != nil {
		t.Fatalf("empty update should be no-op: %v", err)
	}
	cur, _ := tree.Root()
	if cur.Cmp(prev) != 0 {
		t.Fatalf("root changed on empty update")
	}

	// range checks
	if err := tree.UpdateMany([]int{-1}, []*big.Int{bigIntPoseidon(7)}); err == nil {
		t.Fatalf("expected out-of-range error")
	}
	if err := tree.UpdateMany([]int{len(leaves)}, []*big.Int{bigIntPoseidon(7)}); err == nil {
		t.Fatalf("expected out-of-range error")
	}

	// duplicates
	if err := tree.UpdateMany([]int{1, 1}, []*big.Int{bigIntPoseidon(8), bigIntPoseidon(9)}); err == nil {
		t.Fatalf("expected duplicate error")
	}

	// equivalence with multiple Update calls
	treeA, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := treeA.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}
	treeB, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := treeB.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	indices := []int{0, 2, 4}
	values := []*big.Int{bigIntPoseidon(10), bigIntPoseidon(11), bigIntPoseidon(12)}

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

func TestProofsBigIntPoseidon(t *testing.T) {
	// single leaf proof
	one := []*big.Int{bigIntPoseidon(1)}
	tree, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
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
	leaves := []*big.Int{bigIntPoseidon(0), bigIntPoseidon(1), bigIntPoseidon(2), bigIntPoseidon(3), bigIntPoseidon(4)}
	tree2, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
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

func TestImportExportBigIntPoseidon(t *testing.T) {
	leaves := []*big.Int{bigIntPoseidon(0), bigIntPoseidon(1), bigIntPoseidon(2), bigIntPoseidon(3), bigIntPoseidon(4)}
	tree1, _ := New(poseidonBig, BigIntEqualPoseidon, nil, nil, nil)
	if err := tree1.InsertMany(leaves); err != nil {
		t.Fatal(err)
	}

	jsonStr, err := tree1.Export()
	if err != nil {
		t.Fatal(err)
	}

	tree2, err := Import(poseidonBig, jsonStr, BigIntEqualPoseidon, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Do an insert on both, roots must match.
	if err := tree1.Insert(bigIntPoseidon(4)); err != nil {
		t.Fatal(err)
	}
	if err := tree2.Insert(bigIntPoseidon(4)); err != nil {
		t.Fatal(err)
	}
	r1, _ := tree1.Root()
	r2, _ := tree2.Root()
	if r1.Cmp(r2) != 0 {
		t.Fatalf("imported tree root mismatch")
	}
}
