package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// The readings themselves say whether sending requests to a seat spends the
// balance about to lapse: the package just as full while the account as a whole
// was spent means the vendor spends something else, and there is nothing to be
// had by moving a conversation there.
func TestLapseFutileWatchesWhetherTheTrafficSpendsIt(t *testing.T) {
	const key = "lapse-check@acct"
	c := candidate{rest: key, model: "m", p: provider.Provider{ID: "lapse-check"}}
	now := time.Now()
	lapseAt := now.Add(6 * time.Hour)
	snapshot := func(lapse, used float64) left {
		return left{runsOut: lapseAt, runsOutAmount: lapse, of: 5000, amount: used}
	}
	traffic := func(n float64) {
		seatAnswers.Lock()
		seatAnswers.m[key] += n
		seatAnswers.Unlock()
	}
	lapse.Lock()
	delete(lapse.m, key)
	lapse.Unlock()

	// a first reading is a snapshot, and proves nothing
	if lapseFutile(c, snapshot(100, 0), now) {
		t.Fatal("a first reading was called futile")
	}
	// traffic, and the package itself fell: this seat spends it
	traffic(3)
	if lapseFutile(c, snapshot(90, 11), now.Add(3*time.Minute)) {
		t.Fatal("a package that fell was called futile")
	}
	// a reading that names no package apart from its count can show one thing
	// only: a count that stood while the lapse shrank is a spend somewhere.
	// a newer package is judged by its own reading, not the old one's
	if lapseFutile(c, snapshot(300, 60), now.Add(9*time.Minute)) {
		t.Fatal("a newer package was judged by the old one")
	}
	// nothing about to go: the check forgets it
	if lapseFutile(c, left{runsOut: time.Time{}, of: 5000, amount: 60}, now.Add(10*time.Minute)) {
		t.Fatal("nothing lapsing was called futile")
	}
	lapse.Lock()
	_, still := lapse.m[key]
	lapse.Unlock()
	if still {
		t.Fatal("the check kept a seat with nothing about to lapse")
	}
}

// Silence proves nothing: with no traffic between two readings, the answer
// stands as it was rather than being read from an account that was never asked.
func TestLapseFutileWaitsForTraffic(t *testing.T) {
	const key = "lapse-quiet@acct"
	c := candidate{rest: key, model: "m", p: provider.Provider{ID: "lapse-quiet"}}
	now := time.Now()
	l := left{runsOut: now.Add(6 * time.Hour), runsOutAmount: 100, of: 5000, amount: 0}
	lapse.Lock()
	delete(lapse.m, key)
	lapse.Unlock()
	if lapseFutile(c, l, now) {
		t.Fatal("a first reading was called futile")
	}
	moved := left{runsOut: now.Add(6 * time.Hour), runsOutAmount: 100, of: 5000, amount: 80}
	if lapseFutile(c, moved, now.Add(10*time.Minute)) {
		t.Fatal("a spend with no traffic to the seat was called futile")
	}
}
