package gateway

import (
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// What the readings say about a seat's balance about to lapse, when the reading
// names the package itself (Allowance.LeftOf): the package's own count falling
// is a spend however the account's total moved; it standing while the total was
// spent is a sibling's spend, and not what a request here spends.
//
// A reading is compared only with one kept at least lapseEvery before, with
// traffic to the seat between them; silence proves nothing.
func TestLapseFutileWatchesThePackageWhenTheReadingNamesIt(t *testing.T) {
	const key = "lapse-pkg@acct"
	c := candidate{rest: key, model: "m", p: provider.Provider{ID: "lapse-pkg", Account: &provider.Account{User: "acct"}}}
	base := time.Now()
	// pkgLeft: what the watched package holds; totalUsed: the account's own
	// count used; lapse: what the reading says is about to be lost
	snapshot := func(pkgLeft, totalUsed, lapse float64) left {
		return left{runsOut: base.Add(6 * time.Hour), runsOutAmount: lapse, of: 5000, amount: totalUsed,
			lapseLeft: pkgLeft, lapseOf: 5000, lapseHasPkg: true}
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

	// a package that fell is spent, whatever the account's total did
	reset()
	if lapseFutile(c, snapshot(100, 0, 100), base) {
		t.Fatal("a first reading was called futile")
	}
	traffic()
	if lapseFutile(c, snapshot(90, 5, 90), base.Add(3*time.Minute)) {
		t.Fatal("the watched package that fell was called futile")
	}

	// A package that stood whole while the account's count was spent. The count
	// the check watches here is the account's own (the reading names none apart
	// from it), so its fall reads as this seat's spend: what a 鎬荤Н鍒?style
	// reading cannot tell a package's spend from a sibling's, and does not
	// pretend to.
	reset()
	if lapseFutile(c, snapshot(5000, 0, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	traffic()
	if lapseFutile(c, snapshot(4940, 60, 90), base.Add(3*time.Minute)) {
		t.Fatal("a count that fell is called futile, whatever else stood")
	}

	// a package that stood while the account's whole fell, where the reading
	// names the package apart from the count: the vendor spent a sibling, and a
	// request here loses nothing by waiting
	reset()
	if lapseFutile(c, snapshot(100, 0, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	traffic()
	if !lapseFutile(c, snapshot(100, 40, 90), base.Add(3*time.Minute)) {
		t.Fatal("a package standing while a sibling was spent is not futile")
	}

	// a newer package is judged by its own reading, not the old one's
	reset()
	if lapseFutile(c, snapshot(100, 0, 90), base) {
		t.Fatal("a first reading was called futile")
	}
	if lapseFutile(c, snapshot(300, 0, 300), base.Add(3*time.Minute)) {
		t.Fatal("a newer package was judged by the old one")
	}
}
