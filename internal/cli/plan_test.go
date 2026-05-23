package cli

import (
	"reflect"
	"testing"

	"github.com/tzone85/px-dispatch/internal/planner"
)

func TestScopeStoriesForRequirement(t *testing.T) {
	stories := []planner.PlannedStory{
		{
			ID:        "s-1",
			Title:     "First",
			DependsOn: nil,
		},
		{
			ID:        "s-2",
			Title:     "Second",
			DependsOn: []string{"s-1", "external-dep"},
		},
	}

	scoped := scopeStoriesForRequirement("01KM674MCZ3XHPN4SR9W8EK22A", stories)

	if got, want := scoped[0].ID, "01KM674M-s-1"; got != want {
		t.Fatalf("scoped[0].ID = %q, want %q", got, want)
	}
	if got, want := scoped[1].ID, "01KM674M-s-2"; got != want {
		t.Fatalf("scoped[1].ID = %q, want %q", got, want)
	}

	wantDeps := []string{"01KM674M-s-1", "01KM674M-external-dep"}
	if !reflect.DeepEqual(scoped[1].DependsOn, wantDeps) {
		t.Fatalf("scoped[1].DependsOn = %#v, want %#v", scoped[1].DependsOn, wantDeps)
	}
}
