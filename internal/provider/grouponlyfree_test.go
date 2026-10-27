package provider

import (
	"strings"
	"testing"
)

// A group kept to free members (OnlyFree) drops the models its vendors no
// longer serve at no cost, and takes them back should one turn free again:
// a vendor retiring a free tier empties that member out of the pool by
// itself, the way a model leaving its catalog does — no one edits the
// group, and a paid model never routes among the free ones.
func TestGroupOnlyFreeKeepsToWhatCostsNothing(t *testing.T) {
	prefsHome(t)
	for _, id := range []string{"a", "b"} {
		if err := Save(Provider{ID: id, Name: id, Key: "k", Chat: "http://127.0.0.1:1/v1", Models: []string{"free-one", "paid-one"}}); err != nil {
			t.Fatal(err)
		}
	}
	liveFree(t, "a", `{"models":[{"id":"free-one","free":true},{"id":"paid-one"}]}`)
	liveFree(t, "b", `{"models":[{"id":"free-one","free":true},{"id":"paid-one"}]}`)
	if err := SaveGroup(Group{Name: "Free", Members: []string{"a/free-one", "a/paid-one", "b/free-one"}, OnlyFree: true, Routing: Ordered}); err != nil {
		t.Fatal(err)
	}
	ids := func() string {
		t.Helper()
		_, ms, ok := FindGroup("group/free")
		if !ok {
			return "-"
		}
		var out []string
		for _, m := range ms {
			out = append(out, m.ID)
		}
		return strings.Join(out, " ")
	}
	if s := ids(); s != "a/free-one b/free-one" {
		t.Fatalf("routed: %s, want the two free members alone", s)
	}
	// a member the group is of a group: the flag inherits down
	if err := SaveGroup(Group{Name: "Outer", Members: []string{"group/free", "a/paid-one"}, OnlyFree: true, Routing: Ordered}); err != nil {
		t.Fatal(err)
	}
	_, ms, ok := FindGroup("group/outer")
	if !ok {
		t.Fatal("outer gone")
	}
	// the nested group's members arrive under its own name, and the paid
	// model the outer names is cut by the inherited flag
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	if s := strings.Join(out, " "); s != "group/free group/free" {
		t.Fatalf("outer routed: %s", s)
	}
}
