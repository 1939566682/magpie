package provider

import (
	"testing"
	"time"
)

// LeftOf and SpentOf watch one package each: what it has left, and how much it
// has used. A vendor whose reading names both a package and the account's whole
// (CodeArts: 总积分 beside 赠送积分包) means the package by both, so a spend on a
// sibling package does not move them.
func TestLeftOfAndSpentOfNameOnePackage(t *testing.T) {
	now := time.Now()
	a := Allowance{
		{Amount: 4.52, Of: 11600, Name: "zong"},
		{Amount: 0, Of: 500, Name: "base"},
		{Amount: 4.73, Of: 11100, RunsOut: now.Add(24 * time.Hour), RunsOutAmount: 100, Name: "gift"},
	}
	left, of, ok := a.LeftOf("m")
	if !ok || of != 11100 || left != 11100-4.73 {
		t.Fatalf("LeftOf = %v of %v ok %v, want the package's own 11100", left, of, ok)
	}
	used, of, ok := a.SpentOf("m")
	if !ok || of != 11100 || used != 4.73 {
		t.Fatalf("SpentOf = %v of %v ok %v, want 4.73 of 11100", used, of, ok)
	}
	// a window that names no used count of its own is not one to watch this way
	a = Allowance{{Amount: 5, Of: 100, RunsOut: now.Add(time.Hour), UsedUnknown: true}}
	if _, _, ok := a.SpentOf("m"); ok {
		t.Fatal("SpentOf read a window that names no used count")
	}
}
