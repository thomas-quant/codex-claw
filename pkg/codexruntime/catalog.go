package codexruntime

import (
	"context"
	"sync"
)

var fallbackModelCatalog = []ModelCatalogEntry{
	{
		ID:                     "gpt-5.4",
		Label:                  "GPT-5.4",
		ReasoningEffortOptions: []string{"minimal", "medium", "high"},
		SpeedTier:              "standard",
	},
	{
		ID:                     "gpt-5.4-mini",
		Label:                  "GPT-5.4 mini",
		ReasoningEffortOptions: []string{"minimal", "medium"},
		SpeedTier:              "fast",
	},
}

type CatalogClient interface {
	ListModels(context.Context) ([]ModelCatalogEntry, error)
}

type Catalog struct {
	client CatalogClient

	mu     sync.Mutex
	cached []ModelCatalogEntry
}

func NewCatalog(client CatalogClient) *Catalog {
	return &Catalog{client: client}
}

func (c *Catalog) List(ctx context.Context) ([]ModelCatalogEntry, error) {
	c.mu.Lock()
	if len(c.cached) > 0 {
		models := append([]ModelCatalogEntry(nil), c.cached...)
		c.mu.Unlock()
		return models, nil
	}
	c.mu.Unlock()

	if c.client == nil {
		return append([]ModelCatalogEntry(nil), fallbackModelCatalog...), nil
	}

	models, err := c.client.ListModels(ctx)
	if err != nil || len(models) == 0 {
		return append([]ModelCatalogEntry(nil), fallbackModelCatalog...), nil
	}

	c.mu.Lock()
	c.cached = append([]ModelCatalogEntry(nil), models...)
	c.mu.Unlock()

	return append([]ModelCatalogEntry(nil), models...), nil
}
