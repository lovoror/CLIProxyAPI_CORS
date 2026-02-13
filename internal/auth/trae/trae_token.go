package trae

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/misc"
)

// TraeTokenStorage persists Trae app tokens.
type TraeTokenStorage struct {
	AppToken      string `json:"app_token"`
	RefreshToken  string `json:"refresh_token"`
	UserToken     string `json:"user_token"` // The JWT from userJwt.Token
	Email         string `json:"email"`
	UserID        string `json:"user_id"`
	Host          string `json:"host"`
	Region        string `json:"region"`
	AIRegion      string `json:"ai_region"`
	Expire        string `json:"expired"`
	TokenExpire   int64  `json:"token_expire_at"`
	RefreshExpire int64  `json:"refresh_expire_at"`
	LastRefresh   string `json:"last_refresh"`
	Type          string `json:"type"`
}

// SaveTokenToFile serialises the token storage to disk.
func (ts *TraeTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "trae"
	if err := os.MkdirAll(filepath.Dir(authFilePath), 0o700); err != nil {
		return fmt.Errorf("trae token: create directory failed: %w", err)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("trae token: create file failed: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err = json.NewEncoder(f).Encode(ts); err != nil {
		return fmt.Errorf("trae token: encode token failed: %w", err)
	}
	return nil
}
