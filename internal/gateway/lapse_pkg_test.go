package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// What the readings say about a seat's package, when the reading names the
// package itself (Allowance.LeftOf, Allowance.SpentOf): whether its own used
// count grew says whether this seat's traffic spends it.
//
// The used count is what is watched, not what is left: a spend lowers the left
// and a grant raises it, and CodeArts' total moved 601 to 614 while a package was
// being spent. A count that only grows cannot be masked that way.
func TestLapseFutileFollowsThePackageUsedCount(t *testing.T) {
	const key = "lapse-pkg@acct"
	c := candidate{rest: key, model: "m", p: provider.Provider{ID: "lapse-pkg", Account: &provider.Account{User: "acct"}}}
	base := time.Now()
	// left: the package's own count left; used: the vendor's own used count;
	// lapse: what the reading says is about to be lost
	snapshot := func(held, used, lapse float64) left {
		return left{runsOut: base.Add(6 * time.Hour), runsOutAmount: lapse, of: 11100 + used, amount: used,
			lapseLeft: held, lapseOf: 11100, lapseUsed: used, lapseHasPkg: true}
	}
	reset := func() {
		lapse.Lock()
		delete(lapse.m, c.restKey())
		lapse.Unlock()
		seatAnswers.Lock()
		seatAnswers.m[c.restKey()] = 0
		seatAnswers.Unlock()
	}
	traffic := func() {
		seatAnswers.Lock()
		seatAnswers.m[c.restKey()]++
		seatAnswers.Unlock()
	}

	// used grew: this seat spends it, even though a grant refilled the left
	reset()
	if lapseFutile(c, snapshot(90, 10, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	traffic()
	if lapseFutile(c, snapshot(140, 40, 140), base.Add(3*time.Minute)) {
		t.Fatal("a used count that grew was called futile")
	}

	// used stood: a sibling package was spent, and this one loses nothing
	reset()
	if lapseFutile(c, snapshot(90, 10, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	traffic()
	if !lapseFutile(c, snapshot(90, 10, 90), base.Add(3*time.Minute)) {
		t.Fatal("a package whose used count stood is not futile")
	}

	// a newer package is judged by its own reading, not the old one's
	reset()
	if lapseFutile(c, snapshot(90, 10, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	if lapseFutile(c, snapshot(500, 0, 500), base.Add(3*time.Minute)) {
		t.Fatal("a newer package was judged by the old one")
	}
}
