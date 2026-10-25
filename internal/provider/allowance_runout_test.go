package provider

import (
	"testing"
	"time"
)

// RunsOutOf is the soonest moment one of the account's windows loses what it
// has left, and how much of it: WorkBuddy's credits come in packages that
// each end on their own day. Of two, the sooner; one already lost says
// nothing; a window of other models doesn't count.
func TestRunsOutOfTakesTheSoonestPackage(t *testing.T) {
	now := time.Now()
	soon, later := now.Add(9*time.Hour), now.Add(30*24*time.Hour)
	a := Allowance{
		{Used: 2, Of: 4845, Amount: 87, Unit: "credits", RunsOut: later, RunsOutAmount: 2000},
		{Used: 2, Of: 4845, Amount: 87, Unit: "credits", RunsOut: soon, RunsOutAmount: 98.63},
	}
	if at, amount, unit := a.RunsOutOf("claude-opus-5-5", now); !at.Equal(soon) || amount != 98.63 || unit != "credits" {
		t.Fatalf("RunsOutOf: %v %v %v, want %v 98.63 credits", at, amount, unit, soon)
	}
	// the sooner one already lost: the later stands
	if at, amount, _ := a.RunsOutOf("m", soon.Add(time.Minute)); !at.Equal(later) || amount != 2000 {
		t.Fatalf("after the soon one was lost: %v %v, want %v 2000", at, amount, later)
	}
	// nothing left to lose
	if at, amount, unit := (Allowance{{Used: 40, Of: 100}}).RunsOutOf("m", now); !at.IsZero() || amount != 0 || unit != "" {
		t.Fatalf("no runs out: %v %v %v", at, amount, unit)
	}
	// a window of other models doesn't count
	mixed := Allowance{{Used: 2, RunsOut: soon, RunsOutAmount: 5, Model: "glm"}}
	if at, amount, _ := mixed.RunsOutOf("claude-opus-5-5", now); !at.IsZero() || amount != 0 {
		t.Fatalf("another model's window: %v %v", at, amount)
	}
}

// Count sums the windows that name no used count of their own: a vendor that
// says only what is left (千问办公's 当日 beside 长期) has its packages added
// up for a routing row, while the card keeps them apart. One window that does
// name its used count keeps the fullest, as before.
func TestCountSumsWindowsThatNameNoUsedCount(t *testing.T) {
	now := time.Now()
	daily := Allowance{
		{Used: 0, Of: 98.63, Unit: "credits", UsedUnknown: true},
		{Used: 0, Of: 2000, Unit: "credits", UsedUnknown: true},
	}
	if amount, of, unit := daily.Count("m", now); amount != 0 || of != 2098.63 || unit != "credits" {
		t.Fatalf("Count of packages: %v %v %v, want 0 2098.63 credits", amount, of, unit)
	}
	// one window with a used count of its own: the fullest, not the sum
	withUsed := Allowance{
		{Used: 11, Of: 1100, Amount: 122.73, Unit: "credits"},
		{Used: 0, Of: 2000, Amount: 0, Unit: "credits", UsedUnknown: true},
	}
	if amount, of, unit := withUsed.Count("m", now); amount != 122.73 || of != 1100 || unit != "credits" {
		t.Fatalf("Count with one used: %v %v %v, want the fullest 122.73 of 1100", amount, of, unit)
	}
	// a single window that names no used count stands alone
	if amount, of, unit := (Allowance{{Used: 0, Of: 6507, Unit: "credits", UsedUnknown: true}}).Count("m", now); amount != 0 || of != 6507 {
		t.Fatalf("one window: %v %v %v, want 0 of 6507", amount, of, unit)
	}
	// another model's packages don't join the sum
	if _, of, _ := (Allowance{
		{Used: 0, Of: 100, Unit: "credits", UsedUnknown: true, Model: "glm"},
		{Used: 0, Of: 2000, Unit: "credits", UsedUnknown: true},
	}).Count("claude-opus-5-5", now); of != 2000 {
		t.Fatalf("another model's package joined: %v, want 2000", of)
	}
	// a window whose reset has passed is empty again and joins nothing
	spent := Allowance{
		{Used: 40, Of: 500, Amount: 200, Unit: "credits", UsedUnknown: true, Resets: now.Add(-time.Minute)},
		{Used: 0, Of: 2000, Unit: "credits", UsedUnknown: true},
	}
	if amount, of, _ := spent.Count("m", now); amount != 0 || of != 2500 {
		t.Fatalf("a passed reset: %v %v, want 0 of 2500", amount, of)
	}
}
