package secrets

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/ealink1/navi-fyne/internal/upstream/connection"
)

// Split also covers credentials nested in SSH, proxy and other nested configuration.
// URI/DSN/extra parameters are treated as opaque secrets, avoiding fragile parsing.
func Split(cfg connection.ConnectionConfig) (connection.ConnectionConfig, []byte, error) {
	public, err := configMap(cfg)
	if err != nil {
		return cfg, nil, err
	}
	secret := splitMap(public)
	raw, err := json.Marshal(public)
	if err != nil {
		return cfg, nil, err
	}
	var result connection.ConnectionConfig
	if err = json.Unmarshal(raw, &result); err != nil {
		return cfg, nil, err
	}
	bundle, err := json.Marshal(secret)
	return result, bundle, err
}

// PublicJSON omits secret keys entirely. Marshaling the public configuration
// struct would reintroduce empty non-omitempty fields and erase opaque credentials
// when an unchanged advanced editor is merged back into the original config.
func PublicJSON(cfg connection.ConnectionConfig) ([]byte, error) {
	public, err := configMap(cfg)
	if err != nil {
		return nil, err
	}
	splitMap(public)
	return json.MarshalIndent(public, "", "  ")
}

func configMap(cfg connection.ConnectionConfig) (map[string]any, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(raw, &result)
	return result, err
}

func splitMap(public map[string]any) map[string]any {
	secret := map[string]any{}
	for key, value := range public {
		if nested, ok := value.(map[string]any); ok {
			if part := splitMap(nested); len(part) > 0 {
				secret[key] = part
			}
			continue
		}
		if sensitiveKey(key) || secretURL(value) {
			if value != "" && value != nil {
				secret[key] = value
			}
			delete(public, key)
		}
	}
	return secret
}
func sensitiveKey(key string) bool {
	key = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "_", ""), "-", ""))
	if key == "savepassword" {
		return false
	}
	return strings.Contains(key, "password") || strings.Contains(key, "token") || strings.Contains(key, "secret") || strings.Contains(key, "apikey") || key == "dsn" || key == "uri" || key == "connectionparams" || key == "authorization"
}
func secretURL(value any) bool {
	if values, ok := value.([]any); ok {
		for _, item := range values {
			if secretURL(item) {
				return true
			}
		}
		return false
	}
	text, ok := value.(string)
	if !ok || !strings.Contains(text, "://") {
		return false
	}
	u, err := url.Parse(text)
	if err != nil {
		return false
	}
	if u.User != nil {
		return true
	}
	for key := range u.Query() {
		if sensitiveKey(key) {
			return true
		}
	}
	return false
}
func Join(public connection.ConnectionConfig, bundle []byte) (connection.ConnectionConfig, error) {
	raw, err := json.Marshal(public)
	if err != nil {
		return public, err
	}
	var base, secret map[string]any
	if err = json.Unmarshal(raw, &base); err != nil {
		return public, err
	}
	if err = json.Unmarshal(bundle, &secret); err != nil {
		return public, err
	}
	merge(base, secret)
	raw, err = json.Marshal(base)
	if err != nil {
		return public, err
	}
	var result connection.ConnectionConfig
	err = json.Unmarshal(raw, &result)
	return result, err
}
func merge(base, secret map[string]any) {
	for key, value := range secret {
		if nested, ok := value.(map[string]any); ok {
			target, ok := base[key].(map[string]any)
			if !ok {
				target = map[string]any{}
				base[key] = target
			}
			merge(target, nested)
		} else {
			base[key] = value
		}
	}
}
