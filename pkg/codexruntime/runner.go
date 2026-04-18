package codexruntime

import (
	"context"
	"fmt"
	"time"
)

const (
	recoveryModeFresh              = "fresh"
	recoveryModeResumed            = "resumed"
	recoveryModeResumeAfterRestart = "resume_after_restart"
)

type RecoveryRequest struct {
	AllowServerRestart bool
	AllowResume        bool
}

type ControlRequest struct {
	ThinkingMode      string
	FastEnabled       bool
	FastEnabledSet    bool
	LastUserMessageAt string
	ForceFreshThread  bool
}

type RunRequest struct {
	BindingKey     string
	Model          string
	Input          []TurnInputItem
	SandboxPolicy  *SandboxPolicy
	Recovery       RecoveryRequest
	Control        ControlRequest
	DynamicTools   []DynamicToolDefinition
	HandleToolCall ToolCallHandler
	OnChunk        func(string)
}

type RunResult struct {
	Content  string
	ThreadID string
}

type RunnerClient interface {
	Start(context.Context) error
	ResumeThread(context.Context, string, []DynamicToolDefinition) error
	StartThread(context.Context, string, []DynamicToolDefinition) (string, error)
	RunTextTurn(context.Context, RunTurnRequest) (string, error)
	Restart(context.Context) error
	StartNativeCompaction(context.Context, string) error
	ListModels(context.Context) ([]ModelCatalogEntry, error)
	ReadAccount(context.Context, bool) (AccountSnapshot, error)
	ReadRateLimits(context.Context) ([]RateLimitSnapshot, error)
	Close() error
	Status() ClientStatus
}

type Runner struct {
	client   RunnerClient
	bindings *BindingStore
	catalog  *Catalog
}

func NewRunner(client RunnerClient, bindings *BindingStore) *Runner {
	var catalog *Catalog
	if client != nil {
		catalog = NewCatalog(client)
	}
	return &Runner{client: client, bindings: bindings, catalog: catalog}
}

func (r *Runner) ensureClientStarted(ctx context.Context) error {
	if r.client == nil {
		return fmt.Errorf("codexruntime: client not configured")
	}
	return r.client.Start(ctx)
}

func (r *Runner) RunTextTurn(ctx context.Context, req RunRequest) (RunResult, error) {
	if err := r.ensureClientStarted(ctx); err != nil {
		return RunResult{}, err
	}

	binding, ok, err := r.loadBinding(req.BindingKey)
	if err != nil {
		return RunResult{}, err
	}
	threadID := binding.ThreadID
	startedNewThread := false
	recoveryMode := recoveryModeFresh
	resumeAttempted := false
	restartAttempted := false
	allowResume := req.Recovery.AllowResume && !req.Control.ForceFreshThread

	if ok && threadID != "" && !allowResume {
		threadID = ""
	}
	if ok && threadID != "" && allowResume {
		resumeAttempted = true
		if err := r.client.ResumeThread(ctx, threadID, req.DynamicTools); err != nil {
			if req.Recovery.AllowServerRestart {
				restartAttempted = true
				if restartErr := r.client.Restart(ctx); restartErr == nil {
					resumeAttempted = true
					if err := r.client.ResumeThread(ctx, threadID, req.DynamicTools); err == nil {
						recoveryMode = recoveryModeResumeAfterRestart
					} else {
						threadID = ""
					}
				} else {
					threadID = ""
				}
			} else {
				threadID = ""
			}
		} else {
			recoveryMode = recoveryModeResumed
		}
	}
	if threadID == "" {
		threadID, err = r.client.StartThread(ctx, req.Model, req.DynamicTools)
		if err != nil {
			return RunResult{}, err
		}
		startedNewThread = true
	}

	content, err := r.client.RunTextTurn(ctx, RunTurnRequest{
		ThreadID:       threadID,
		Input:          req.Input,
		SandboxPolicy:  req.SandboxPolicy,
		HandleToolCall: req.HandleToolCall,
		OnChunk:        req.OnChunk,
	})
	if err != nil {
		return RunResult{}, err
	}
	if err := r.saveBinding(req.BindingKey, binding, threadID, req, runnerSaveOptions{
		StartedNewThread: startedNewThread,
		Recovery: RecoveryStatus{
			RestartAttempted: restartAttempted,
			ResumeAttempted:  resumeAttempted,
			FellBackToFresh:  startedNewThread && resumeAttempted,
			Mode:             recoveryMode,
		},
	}); err != nil {
		return RunResult{}, err
	}

	return RunResult{
		Content:  content,
		ThreadID: threadID,
	}, nil
}

func (r *Runner) CompactThread(ctx context.Context, bindingKey string) error {
	if err := r.ensureClientStarted(ctx); err != nil {
		return err
	}

	binding, ok, err := r.loadBinding(bindingKey)
	if err != nil {
		return err
	}
	if !ok || binding.ThreadID == "" {
		return nil
	}

	if err := r.client.StartNativeCompaction(ctx, binding.ThreadID); err != nil {
		return err
	}

	if binding.Metadata == nil {
		binding.Metadata = make(map[string]any)
	}
	binding.Metadata[bindingMetadataLastCompactionAt] = time.Now().UTC().Format(time.RFC3339Nano)
	return r.bindings.Save(binding)
}

func (r *Runner) SteerTurn(ctx context.Context, threadID string, input []TurnInputItem) error {
	if err := r.ensureClientStarted(ctx); err != nil {
		return err
	}

	steerer, ok := r.client.(interface {
		SteerTurn(context.Context, string, []TurnInputItem) error
	})
	if !ok {
		return fmt.Errorf("codexruntime: client does not support turn steering")
	}

	return steerer.SteerTurn(ctx, threadID, input)
}

func (r *Runner) ListModels(ctx context.Context) ([]ModelCatalogEntry, error) {
	if r.catalog == nil {
		return append([]ModelCatalogEntry(nil), fallbackModelCatalog...), nil
	}
	return r.catalog.List(ctx)
}

func (r *Runner) ReadRateLimits(ctx context.Context) ([]RateLimitSnapshot, error) {
	if err := r.ensureClientStarted(ctx); err != nil {
		return nil, err
	}
	return r.client.ReadRateLimits(ctx)
}

func (r *Runner) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}

func (r *Runner) ReadStatus(ctx context.Context, bindingKey string) (RuntimeStatusSnapshot, error) {
	binding, _, err := r.loadBinding(bindingKey)
	if err != nil {
		return RuntimeStatusSnapshot{}, err
	}

	clientStatus := ClientStatus{}
	if r.client != nil {
		clientStatus = r.client.Status()
	}
	models, err := r.ListModels(ctx)
	if err == nil {
		clientStatus.KnownModels = make([]string, 0, len(models))
		for _, model := range models {
			if model.ID != "" {
				clientStatus.KnownModels = append(clientStatus.KnownModels, model.ID)
			}
		}
	}

	return BuildRuntimeStatus(RuntimeStatusInput{
		Binding: binding,
		Client:  clientStatus,
	}), nil
}

func (r *Runner) SetModel(_ context.Context, bindingKey, model string) (string, error) {
	if r.bindings == nil || bindingKey == "" {
		return "", nil
	}

	binding, _, err := r.loadBinding(bindingKey)
	if err != nil {
		return "", err
	}
	old := binding.Model
	binding.Key = bindingKey
	binding.Model = model
	return old, r.bindings.Save(binding)
}

func (r *Runner) SetThinkingMode(_ context.Context, bindingKey, thinkingMode string) (string, error) {
	if r.bindings == nil || bindingKey == "" {
		return "", nil
	}

	binding, _, err := r.loadBinding(bindingKey)
	if err != nil {
		return "", err
	}
	old := binding.ThinkingMode
	binding.Key = bindingKey
	binding.ThinkingMode = thinkingMode
	return old, r.bindings.Save(binding)
}

func (r *Runner) ToggleFast(_ context.Context, bindingKey string) (bool, error) {
	if r.bindings == nil || bindingKey == "" {
		return false, nil
	}

	binding, _, err := r.loadBinding(bindingKey)
	if err != nil {
		return false, err
	}
	binding.Key = bindingKey
	binding.FastEnabled = !binding.FastEnabled
	return binding.FastEnabled, r.bindings.Save(binding)
}

func (r *Runner) ResetThread(_ context.Context, bindingKey string) error {
	if r.bindings == nil || bindingKey == "" {
		return nil
	}
	return r.bindings.ResetThread(bindingKey)
}

func (r *Runner) loadBinding(key string) (Binding, bool, error) {
	if r.bindings == nil || key == "" {
		return Binding{Key: key}, false, nil
	}

	return r.bindings.Load(key)
}

type runnerSaveOptions struct {
	StartedNewThread bool
	Recovery         RecoveryStatus
}

func (r *Runner) saveBinding(key string, binding Binding, threadID string, req RunRequest, opts runnerSaveOptions) error {
	if r.bindings == nil || key == "" || threadID == "" {
		return nil
	}

	binding.Key = key
	binding.ThreadID = threadID
	if opts.StartedNewThread || binding.Model == "" {
		binding.Model = req.Model
	}
	if req.Control.ThinkingMode != "" {
		binding.ThinkingMode = req.Control.ThinkingMode
	}
	if req.Control.FastEnabledSet {
		binding.FastEnabled = req.Control.FastEnabled
	}
	if req.Control.LastUserMessageAt != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, req.Control.LastUserMessageAt); err == nil {
			binding.LastUserMessageAt = parsed.UTC()
		}
	}
	if binding.Metadata == nil {
		binding.Metadata = make(map[string]any)
	}
	binding.Metadata[bindingMetadataRecoveryMode] = opts.Recovery.Mode
	binding.Metadata[bindingMetadataRestartAttempted] = opts.Recovery.RestartAttempted
	binding.Metadata[bindingMetadataResumeAttempted] = opts.Recovery.ResumeAttempted
	binding.Metadata[bindingMetadataFellBackToFresh] = opts.Recovery.FellBackToFresh
	delete(binding.Metadata, bindingMetadataForceFreshThread)

	return r.bindings.Save(binding)
}
