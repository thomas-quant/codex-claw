package commands

import (
	"context"
	"testing"
)

func TestShowModel_RequiresRuntimeStatus(t *testing.T) {
	rt := &Runtime{
		GetModelInfo: func() (string, string) {
			return "stale-model", "stale-provider"
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	var reply string
	res := ex.Execute(context.Background(), Request{
		Text: "/show model",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if reply != unavailableMsg {
		t.Fatalf("reply=%q, want unavailable message", reply)
	}
}

func TestListModels_RequiresRuntimeCatalog(t *testing.T) {
	rt := &Runtime{
		GetModelInfo: func() (string, string) {
			return "stale-model", "stale-provider"
		},
	}
	ex := NewExecutor(NewRegistry(BuiltinDefinitions()), rt)

	var reply string
	res := ex.Execute(context.Background(), Request{
		Text: "/list models",
		Reply: func(text string) error {
			reply = text
			return nil
		},
	})
	if res.Outcome != OutcomeHandled {
		t.Fatalf("outcome=%v, want=%v", res.Outcome, OutcomeHandled)
	}
	if reply != unavailableMsg {
		t.Fatalf("reply=%q, want unavailable message", reply)
	}
}
