package miromind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/misc"
)

// MiroMindTokenStorage stores session information for MiroMind AI authentication.
type MiroMindTokenStorage struct {
	// SessionToken is the secret session token (likely a cookie or JWT).
	SessionToken string `json:"session_token"`
	// LastRefresh is the timestamp of the last token save/refresh.
	LastRefresh string `json:"last_refresh"`
	// Email is the account email associated with this session.
	Email string `json:"email"`
	// Type indicates the authentication provider type, always "miromind".
	Type string `json:"type"`
	// Expire is the timestamp when the session is expected to expire.
	Expire string `json:"expired"`
}

// SaveTokenToFile serializes the MiroMind token storage to a JSON file.
func (ts *MiroMindTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "miromind"
	if err := os.MkdirAll(filepath.Dir(authFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	if err = json.NewEncoder(f).Encode(ts); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}
