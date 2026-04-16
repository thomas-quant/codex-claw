package account

import (
	"context"

	"github.com/thomas-quant/codex-claw/pkg/codexaccounts"
)

type stubManager struct {
	addFn     func(context.Context, string, codexaccounts.AddOptions) error
	importFn  func(context.Context, string, codexaccounts.ImportOptions) error
	listFn    func(context.Context) ([]codexaccounts.AccountSummary, error)
	statusFn  func(context.Context) (codexaccounts.StatusSummary, error)
	removeFn  func(context.Context, string) error
	enableFn  func(context.Context, string) error
	disableFn func(context.Context, string) error
}

func (s stubManager) Add(ctx context.Context, alias string, options codexaccounts.AddOptions) error {
	if s.addFn != nil {
		return s.addFn(ctx, alias, options)
	}
	return nil
}

func (s stubManager) Import(ctx context.Context, alias string, options codexaccounts.ImportOptions) error {
	if s.importFn != nil {
		return s.importFn(ctx, alias, options)
	}
	return nil
}

func (s stubManager) List(ctx context.Context) ([]codexaccounts.AccountSummary, error) {
	if s.listFn != nil {
		return s.listFn(ctx)
	}
	return nil, nil
}

func (s stubManager) Status(ctx context.Context) (codexaccounts.StatusSummary, error) {
	if s.statusFn != nil {
		return s.statusFn(ctx)
	}
	return codexaccounts.StatusSummary{}, nil
}

func (s stubManager) Remove(ctx context.Context, alias string) error {
	if s.removeFn != nil {
		return s.removeFn(ctx, alias)
	}
	return nil
}

func (s stubManager) Enable(ctx context.Context, alias string) error {
	if s.enableFn != nil {
		return s.enableFn(ctx, alias)
	}
	return nil
}

func (s stubManager) Disable(ctx context.Context, alias string) error {
	if s.disableFn != nil {
		return s.disableFn(ctx, alias)
	}
	return nil
}
