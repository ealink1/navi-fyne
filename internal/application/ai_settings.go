package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"

	"github.com/ealink1/super-link/internal/infra/chat"
	"github.com/ealink1/super-link/internal/infra/secrets"
	"github.com/ealink1/super-link/internal/infra/state"
)

const aiSettingKey = "ai.chat.config.ref"

// AISettings stores provider configuration in the same encrypted local vault as
// remembered connections. Chat messages never pass through this service.
type AISettings struct {
	Store *state.Store
	Vault secrets.Vault
	mu    sync.Mutex
}

// AIConfigSave distinguishes a committed configuration from a cleanup failure.
type AIConfigSave struct {
	Config       chat.Config
	CleanupError error
}

// Load returns a zero configuration when the feature has not been configured.
func (s *AISettings) Load(ctx context.Context) (chat.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(ctx)
}

func (s *AISettings) load(ctx context.Context) (chat.Config, error) {
	ref, err := s.Store.Setting(ctx, aiSettingKey)
	if err != nil || ref == "" {
		return chat.Config{}, err
	}
	if err := ctx.Err(); err != nil {
		return chat.Config{}, err
	}
	raw, err := s.Vault.Get(ref)
	if err != nil {
		return chat.Config{}, errors.New("AI 配置无法解密，请恢复完整工作区备份或重新配置")
	}
	defer clear(raw)
	var config chat.Config
	if len(raw) > 16<<10 || json.Unmarshal(raw, &config) != nil {
		return config, errors.New("AI 配置损坏，请重新配置")
	}
	return config, config.Validate()
}

// Save publishes the new ciphertext before deleting the old committed bundle.
func (s *AISettings) Save(ctx context.Context, config chat.Config) (AIConfigSave, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	config.BaseURL = strings.TrimSpace(config.BaseURL)
	config.Model = strings.TrimSpace(config.Model)
	config.APIKey = strings.TrimSpace(config.APIKey)
	if err := config.Validate(); err != nil {
		return AIConfigSave{}, err
	}
	old, err := s.Store.Setting(ctx, aiSettingKey)
	if err != nil {
		return AIConfigSave{}, err
	}
	if err := ctx.Err(); err != nil {
		return AIConfigSave{}, err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return AIConfigSave{}, errors.New("无法编码 AI 配置")
	}
	defer clear(raw)
	ref, err := s.Vault.Put(raw, true)
	if err != nil {
		return AIConfigSave{}, errors.New("AI 配置加密保存失败，请检查工作区权限")
	}
	if err := s.Store.SetSetting(ctx, aiSettingKey, ref); err != nil {
		return AIConfigSave{}, errors.Join(err, s.Vault.Delete(ref))
	}
	return AIConfigSave{Config: config, CleanupError: s.Vault.Delete(old)}, nil
}
