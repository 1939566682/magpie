package provider

import (
	"testing"
	"time"
)

func TestKeepLast(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	restart := func() {
		lastQuotas.Lock()
		lastQuotas.m, lastQuotas.loaded = nil, false
		lastQuotas.Unlock()
	}
	restart()
	defer restart()
	later := time.Now().Add(time.Hour)
	good := SubscriptionQuota{Provider: "claude", Name: "Claude Code", User: "A@x.com", Windows: []QuotaWindow{
		{Name: "5 hours", Used: 40, ResetsAt: &later, Span: 5 * time.Hour},
		{Name: "7 days · Opus", Used: 10, Model: "opus", Span: 7 * 24 * time.Hour},
	}}
	if q := keepLast(good, ""); q.AsOf != nil || q.Windows[0].Used != 40 {
		t.Fatalf("%+v", q)
	}
	restart() // what was read is on disk

	limited := SubscriptionQuota{Provider: "claude", Name: "Claude Code", User: "a@x.com", Windows: []QuotaWindow{},
		Error: errClaudeNotAsked.Error()}
	q := keepLast(limited, "")
	if q.Error != "" || q.AsOf == nil || len(q.Windows) != 2 || q.Windows[0].Used != 40 || q.Windows[1].Model != "opus" || q.Windows[0].Span != 5*time.Hour {
		t.Fatalf("not asked: %+v", q)
	}

	// the same through LoginUsage, which knows the user but not the name
	q = keepLast(SubscriptionQuota{Provider: "claude", Windows: []QuotaWindow{}, Error: "Too Many Requests"}, "a@x.com")
	if q.Error != "" || q.Name != "Claude Code" || q.Windows[0].Used != 40 {
		t.Fatalf("login: %+v", q)
	}

	// a sign-in gone bad is said, not covered up
	if q := keepLast(SubscriptionQuota{Provider: "claude", User: "a@x.com", Error: "Claude Code's sign-in has expired"}, ""); q.Error == "" {
		t.Fatalf("expired: %+v", q)
	}

	// a new reading is used at once
	good.Windows[0].Used = 70
	keepLast(good, "")
	if q := keepLast(limited, ""); q.Windows[0].Used != 70 {
		t.Fatalf("newer: %+v", q)
	}

	// a window that has reset since starts again from nothing
	past := time.Now().Add(-time.Minute)
	good.Windows[0].ResetsAt = &past
	keepLast(good, "")
	if q := keepLast(limited, ""); q.Windows[0].Used != 0 {
		t.Fatalf("reset: %+v", q)
	}
}

// A window kept for some models only (WorkBuddy's free models) is read again
// on its own. matches is a closure and can't be written to disk, so the
// window's scope is lost across a restart — but the reading is no longer
// dropped whole for it, and the counts that do serialize come back.
func TestKeepLastKeepsAWindowScopedToModels(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	restart := func() {
		lastQuotas.Lock()
		lastQuotas.m, lastQuotas.loaded = nil, false
		lastQuotas.Unlock()
	}
	restart()
	defer restart()
	later, soon := time.Now().Add(time.Hour), time.Now().Add(14*time.Hour)
	scoped := SubscriptionQuota{Provider: "workbuddy", Name: "WorkBuddy", User: "{{EMAIL_jph3tjaq}}", Windows: []QuotaWindow{
		{Name: "Credits", Used: 2, ResetsAt: &later, Amount: 87, Limit: 4845, Unit: "credits",
			RunsOutAt: &soon, RunsOut: 14.23, matches: func(string) bool { return true }},
	}}
	if q := keepLast(scoped, ""); len(q.Windows) != 1 || q.Windows[0].Used != 2 {
		t.Fatalf("scoped: %+v", q)
	}
	restart() // what was read is on disk

	// not asked: the reading stands, with the counts that serialize
	q := keepLast(SubscriptionQuota{Provider: "workbuddy", Name: "WorkBuddy", User: "{{EMAIL_jph3tjaq}}",
		Windows: []QuotaWindow{}, Error: errNotAsked.Error()}, "")
	if len(q.Windows) != 1 || q.Windows[0].Used != 2 || q.Windows[0].Amount != 87 || q.Windows[0].Limit != 4845 {
		t.Fatalf("read again: %+v", q)
	}
	if q.Windows[0].Unit != "credits" || q.Windows[0].RunsOut != 14.23 || q.Windows[0].RunsOutAt == nil {
		t.Fatalf("the credits' counts: %+v", q.Windows[0])
	}
	// the scope itself can't cross the disk: matches is a closure
	if q.Windows[0].matches != nil {
		t.Fatalf("matches survived the disk: %+v", q.Windows[0])
	}
}
