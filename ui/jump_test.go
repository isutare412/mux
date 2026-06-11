package ui

import "testing"

func TestAssignLabels_SkipsPanesAndOrders(t *testing.T) {
	items := []listItem{
		{kind: itemSession},
		{kind: itemWindow},
		{kind: itemPane},
		{kind: itemWindow},
	}
	got := assignLabels(items)
	want := []string{"a", "s", "", "d"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("labels[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestAssignLabels_Overflow(t *testing.T) {
	n := len(jumpAlphabet) + 2
	items := make([]listItem, n)
	for i := range items {
		items[i] = listItem{kind: itemSession}
	}
	got := assignLabels(items)
	if got[len(jumpAlphabet)-1] == "" {
		t.Errorf("last in-range row should have a label")
	}
	if got[len(jumpAlphabet)] != "" {
		t.Errorf("overflow row should have empty label, got %q", got[len(jumpAlphabet)])
	}
}
