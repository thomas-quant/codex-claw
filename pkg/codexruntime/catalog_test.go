package codexruntime

import (
	"context"
	"errors"
	"slices"
	"testing"
)

func TestCatalog_ListUsesFallbackAndCachesSuccessfulLookup(t *testing.T) {
	t.Parallel()

	t.Run("fallback on list error", func(t *testing.T) {
		t.Parallel()

		client := &fakeCatalogClient{err: errors.New("app-server unavailable")}
		catalog := NewCatalog(client)

		models, err := catalog.List(context.Background())
		if err != nil {
			t.Fatalf("List() error = %v", err)
		}
		if client.calls != 1 {
			t.Fatalf("client calls = %d, want %d", client.calls, 1)
		}
		if got := modelIDs(models); !slices.Equal(got, []string{"gpt-5.4", "gpt-5.4-mini"}) {
			t.Fatalf("List() ids = %v, want %v", got, []string{"gpt-5.4", "gpt-5.4-mini"})
		}
	})

	t.Run("cache reused after success", func(t *testing.T) {
		t.Parallel()

		client := &fakeCatalogClient{
			models: []ModelCatalogEntry{
				{ID: "gpt-5.4", Label: "GPT-5.4"},
				{ID: "gpt-5.4-mini", Label: "GPT-5.4 mini"},
				{ID: "gpt-6-preview", Label: "GPT-6 preview"},
			},
		}
		catalog := NewCatalog(client)

		first, err := catalog.List(context.Background())
		if err != nil {
			t.Fatalf("first List() error = %v", err)
		}
		second, err := catalog.List(context.Background())
		if err != nil {
			t.Fatalf("second List() error = %v", err)
		}

		if client.calls != 1 {
			t.Fatalf("client calls = %d, want %d", client.calls, 1)
		}
		if !slices.Equal(modelIDs(first), modelIDs(second)) {
			t.Fatalf("cached ids = %v, want %v", modelIDs(second), modelIDs(first))
		}
	})
}

type fakeCatalogClient struct {
	models []ModelCatalogEntry
	err    error
	calls  int
}

func (c *fakeCatalogClient) ListModels(context.Context) ([]ModelCatalogEntry, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}

	return append([]ModelCatalogEntry(nil), c.models...), nil
}

func modelIDs(models []ModelCatalogEntry) []string {
	ids := make([]string, 0, len(models))
	for _, model := range models {
		ids = append(ids, model.ID)
	}

	return ids
}
