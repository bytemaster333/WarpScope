package validator

import (
	"testing"

	"github.com/ava-labs/avalanchego/ids"
)

func makeSet(entries ...*ValidatorEntry) *ValidatorSet {
	var total uint64
	for _, e := range entries {
		total += e.Weight
	}
	return &ValidatorSet{
		Validators:  entries,
		TotalWeight: total,
	}
}

var (
	nodeA = ids.NodeID{1}
	nodeB = ids.NodeID{2}
	nodeC = ids.NodeID{3}
)

func TestDiff_NoChange(t *testing.T) {
	a := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100, PublicKeyBytes: []byte{0x01}},
		&ValidatorEntry{NodeID: nodeB, Weight: 200, PublicKeyBytes: []byte{0x02}},
	)
	b := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100, PublicKeyBytes: []byte{0x01}},
		&ValidatorEntry{NodeID: nodeB, Weight: 200, PublicKeyBytes: []byte{0x02}},
	)
	d := Diff(a, b)
	if !d.IsEmpty() {
		t.Errorf("expected empty diff, got added=%d removed=%d changed=%d",
			len(d.Added), len(d.Removed), len(d.Changed))
	}
}

func TestDiff_ValidatorRemoved(t *testing.T) {
	atSigning := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100},
		&ValidatorEntry{NodeID: nodeB, Weight: 200},
	)
	current := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100},
		// nodeB removed
	)
	d := Diff(atSigning, current)
	if len(d.Removed) != 1 {
		t.Fatalf("expected 1 removed, got %d", len(d.Removed))
	}
	if d.Removed[0].NodeID != nodeB {
		t.Errorf("expected nodeB removed, got %v", d.Removed[0].NodeID)
	}
	if d.RemovedWeight != 200 {
		t.Errorf("expected RemovedWeight=200, got %d", d.RemovedWeight)
	}
	if len(d.Added) != 0 {
		t.Errorf("expected 0 added, got %d", len(d.Added))
	}
}

func TestDiff_ValidatorAdded(t *testing.T) {
	atSigning := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100},
	)
	current := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100},
		&ValidatorEntry{NodeID: nodeC, Weight: 300}, // new validator
	)
	d := Diff(atSigning, current)
	if len(d.Added) != 1 {
		t.Fatalf("expected 1 added, got %d", len(d.Added))
	}
	if d.Added[0].NodeID != nodeC {
		t.Errorf("expected nodeC added, got %v", d.Added[0].NodeID)
	}
	if len(d.Removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(d.Removed))
	}
}

func TestDiff_WeightChanged(t *testing.T) {
	atSigning := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 100},
	)
	current := makeSet(
		&ValidatorEntry{NodeID: nodeA, Weight: 150}, // weight increased
	)
	d := Diff(atSigning, current)
	if len(d.Changed) != 1 {
		t.Fatalf("expected 1 changed, got %d", len(d.Changed))
	}
	if d.Changed[0].OldWeight != 100 || d.Changed[0].NewWeight != 150 {
		t.Errorf("unexpected weight change: %+v", d.Changed[0])
	}
}

func TestDiff_IsEmpty(t *testing.T) {
	d := DiffResult{}
	if !d.IsEmpty() {
		t.Error("empty DiffResult should be IsEmpty()")
	}
	d.Added = []*ValidatorEntry{{}}
	if d.IsEmpty() {
		t.Error("non-empty DiffResult should not be IsEmpty()")
	}
}
