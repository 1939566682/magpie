package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
	"github.com/yetone/magpie/internal/routes"
)

// What a seat is losing decides whether the conversation is moved off the
// account whose cache is warm: a balance gone before the deadline is (routes.
// UrgentUntil), a model that costs nothing isn't, a reading past the deadline
// isn't, and one too small to be worth the cache isn't either.
func TestUrgentSeatWeighsWhatIsLostAndWhatSpendsIt(t *testing.T) {
	until := routes.UrgentUntil(time.Now())
	before, after := until.Add(-time.Hour), until.Add(time.Hour)
	wg := weighing{free: map[string]bool{}, urgentUntil: until}
	seat := func(rest string, l left) bool {
		return urgentSeat(candidate{rest: rest, p: provider.Provider{ID: rest}}, wg, l)
	}
	if !seat("paid", left{runsOut: before, runsOutAmount: 100}) {
		t.Fatal("a balance of 100 lost before the deadline is not urgent")
	}
	if seat("paid", left{runsOut: before, runsOutAmount: 10}) {
		t.Fatal("a balance of 10 is urgent: the cache costs more than it")
	}
	if seat("paid", left{runsOut: before}) {
		t.Fatal("a balance of no stated size is urgent")
	}
	if seat("paid", left{runsOut: after, runsOutAmount: 100}) {
		t.Fatal("a balance lost after the deadline is urgent")
	}
	if !seat("paid", left{renews: []time.Time{before}, of: 100, amount: 20}) {
		t.Fatal("a window renewing before the deadline with 80 left is not urgent")
	}
	if seat("paid", left{renews: []time.Time{before}, of: 100, amount: 95}) {
		t.Fatal("a window renewing before the deadline with 5 left is urgent")
	}
	wg.free["free"] = true
	if seat("free", left{runsOut: before, runsOutAmount: 100}) {
		t.Fatal("a free model's own reading going out is urgent")
	}
	// two packages the vendor picks between, shown by its own readings: spending
	// here spends the other one, so the cache would go for nothing
	if seat("paid", left{runsOut: before, runsOutAmount: 100, futile: true}) {
		t.Fatal("a balance shown not to be spent here is urgent")
	}
}

// urgentFirst names the seat the routing put first when it is losing its
// balance: a resting one is passed over, and one losing nothing leaves the
// conversation where it is.
func TestUrgentFirstSkipsRestingAndOnlyLooksAtTheFront(t *testing.T) {
	rest := &Rest{}
	pl := planned{order: []Weighed{{ID: "resting", Rest: rest}, {ID: "urgent", Urgent: true}}}
	if got := urgentFirst(pl); got != "urgent" {
		t.Fatalf("urgentFirst = %q, want urgent", got)
	}
	if got := urgentFirst(planned{order: []Weighed{{ID: "calm"}, {ID: "urgent", Urgent: true}}}); got != "" {
		t.Fatalf("urgentFirst = %q, want none: the front is losing nothing", got)
	}
}

// A provider can be told its own threshold: its points are not another's.
func TestUrgentLeastIsPerProvider(t *testing.T) {
	if got := routes.UrgentLeastOf("codearts"); got != 50 {
		t.Fatalf("UrgentLeastOf(codearts) = %v, want 50 (the default)", got)
	}
}
