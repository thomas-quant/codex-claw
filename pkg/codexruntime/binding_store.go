package codexruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var bindingFilenameUnsafeChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

const (
	bindingMetadataRecoveryMode     = "recovery_mode"
	bindingMetadataRestartAttempted = "restart_attempted"
	bindingMetadataResumeAttempted  = "resume_attempted"
	bindingMetadataFellBackToFresh  = "fell_back_to_fresh"
	bindingMetadataForceFreshThread = "force_fresh_thread"
	bindingMetadataLastCompactionAt = "last_compaction_at"
)

type Binding struct {
	Key               string         `json:"key"`
	ThreadID          string         `json:"thread_id"`
	AgentID           string         `json:"agent_id,omitempty"`
	Channel           string         `json:"channel,omitempty"`
	ThreadKey         string         `json:"thread_key,omitempty"`
	Model             string         `json:"model,omitempty"`
	ThinkingMode      string         `json:"thinking_mode,omitempty"`
	FastEnabled       bool           `json:"fast_enabled,omitempty"`
	LastUserMessageAt time.Time      `json:"last_user_message_at,omitempty"`
	Metadata          map[string]any `json:"metadata,omitempty"`
	CreatedAt         time.Time      `json:"created_at"`
	UpdatedAt         time.Time      `json:"updated_at"`
}

type BindingStore struct {
	root string
}

func NewBindingStore(root string) *BindingStore {
	return &BindingStore{root: root}
}

func (s *BindingStore) Save(binding Binding) error {
	now := time.Now().UTC()
	if binding.CreatedAt.IsZero() {
		if existing, ok, err := s.Load(binding.Key); err != nil {
			return err
		} else if ok && !existing.CreatedAt.IsZero() {
			binding.CreatedAt = existing.CreatedAt
		} else {
			binding.CreatedAt = now
		}
	}
	binding.UpdatedAt = now

	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create binding store root: %w", err)
	}

	data, err := json.MarshalIndent(binding, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal binding: %w", err)
	}
	data = append(data, '\n')

	path := s.pathForKey(binding.Key)
	tmp, err := os.CreateTemp(s.root, ".binding-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp binding file: %w", err)
	}

	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod temp binding file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write binding: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp binding file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename binding file: %w", err)
	}

	cleanup = false
	return nil
}

func (s *BindingStore) Load(key string) (Binding, bool, error) {
	data, err := os.ReadFile(s.pathForKey(key))
	if err != nil {
		if os.IsNotExist(err) {
			return Binding{}, false, nil
		}
		return Binding{}, false, fmt.Errorf("read binding: %w", err)
	}

	var binding Binding
	if err := json.Unmarshal(data, &binding); err != nil {
		return Binding{}, false, fmt.Errorf("unmarshal binding: %w", err)
	}

	return binding, true, nil
}

func (s *BindingStore) LoadThread(key string) (string, bool, error) {
	binding, ok, err := s.Load(key)
	if err != nil || !ok {
		return "", ok, err
	}

	return binding.ThreadID, true, nil
}

func (s *BindingStore) SaveThread(key, threadID, model string) error {
	binding, ok, err := s.Load(key)
	if err != nil {
		return err
	}
	if !ok {
		binding = Binding{Key: key}
	}

	binding.Key = key
	binding.ThreadID = threadID
	if model != "" {
		binding.Model = model
	}

	return s.Save(binding)
}

func (s *BindingStore) SetModel(key, model string) error {
	return s.mutate(key, func(binding *Binding) {
		binding.Model = model
	})
}

func (s *BindingStore) SetThinkingMode(key, thinkingMode string) error {
	return s.mutate(key, func(binding *Binding) {
		binding.ThinkingMode = thinkingMode
	})
}

func (s *BindingStore) SetFastEnabled(key string, fastEnabled bool) error {
	return s.mutate(key, func(binding *Binding) {
		binding.FastEnabled = fastEnabled
	})
}

func (s *BindingStore) SetLastUserMessageAt(key string, lastUserMessageAt time.Time) error {
	return s.mutate(key, func(binding *Binding) {
		binding.LastUserMessageAt = lastUserMessageAt.UTC()
	})
}

func (s *BindingStore) Delete(key string) error {
	err := os.Remove(s.pathForKey(key))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete binding: %w", err)
	}

	return nil
}

func (s *BindingStore) ResetThread(key string) error {
	binding, ok, err := s.Load(key)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	binding.ThreadID = ""
	if binding.Metadata != nil {
		delete(binding.Metadata, bindingMetadataRecoveryMode)
		delete(binding.Metadata, bindingMetadataRestartAttempted)
		delete(binding.Metadata, bindingMetadataResumeAttempted)
		delete(binding.Metadata, bindingMetadataFellBackToFresh)
		delete(binding.Metadata, bindingMetadataForceFreshThread)
		delete(binding.Metadata, bindingMetadataLastCompactionAt)
		if len(binding.Metadata) == 0 {
			binding.Metadata = nil
		}
	}
	return s.Save(binding)
}

func (s *BindingStore) mutate(key string, apply func(*Binding)) error {
	binding, ok, err := s.Load(key)
	if err != nil {
		return err
	}
	if !ok {
		binding = Binding{Key: key}
	}
	binding.Key = key
	apply(&binding)
	return s.Save(binding)
}

func (s *BindingStore) pathForKey(key string) string {
	return filepath.Join(s.root, sanitizeBindingKey(key)+".json")
}

func sanitizeBindingKey(key string) string {
	label := strings.TrimSpace(key)
	if label == "" {
		label = "binding"
	}

	label = bindingFilenameUnsafeChars.ReplaceAllString(label, "_")
	label = strings.Trim(label, "._-")
	if label == "" {
		label = "binding"
	}

	sum := sha256.Sum256([]byte(key))
	return label + "-" + hex.EncodeToString(sum[:6])
}
