package trae

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/registry"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/util"
)

const (
	TraeClientID    = "ono9krqynydwx5"
	TraeAuthBaseURL = "https://www.trae.ai/authorization"
)

// TraeAuth encapsulates the helpers for Trae token management.
type TraeAuth struct {
	httpClient *http.Client
}

// NewTraeAuth constructs a new TraeAuth with proxy-aware transport.
func NewTraeAuth(cfg *config.Config) *TraeAuth {
	client := &http.Client{Timeout: 30 * time.Second}
	return &TraeAuth{httpClient: util.SetProxy(&cfg.SDKConfig, client)}
}

// GenerateAuthURL builds the authorization URL for Trae login.
func (ta *TraeAuth) GenerateAuthURL(state string, callbackURL string) string {
	u, _ := url.Parse(TraeAuthBaseURL)
	q := u.Query()
	q.Set("login_version", "1")
	q.Set("auth_from", "trae")
	q.Set("login_channel", "native_ide")
	q.Set("plugin_version", "1.0.27288")
	q.Set("auth_type", "local")
	q.Set("client_id", TraeClientID)
	q.Set("redirect", "0")
	q.Set("login_trace_id", state) // Reusing state as trace id
	q.Set("auth_callback_url", callbackURL)

	// Machine/Device IDs - using static or random-ish values might work
	// In a real app, these should be generated once and persisted.
	machineID := "cpa_" + state[:16]
	q.Set("machine_id", machineID)
	q.Set("device_id", machineID)
	q.Set("x_device_id", machineID)
	q.Set("x_machine_id", machineID)

	q.Set("x_device_brand", "CLIProxyAPI")
	q.Set("x_device_type", "windows")
	q.Set("x_os_version", "Windows 10 Pro")
	q.Set("x_app_version", "3.5.13")
	q.Set("x_app_type", "stable")

	u.RawQuery = q.Encode()
	return u.String()
}

// CreateTokenStorage converts callback parameters into persistent storage.
func (ta *TraeAuth) CreateTokenStorage(params map[string]string) *TraeTokenStorage {
	ts := &TraeTokenStorage{
		LastRefresh: time.Now().Format(time.RFC3339),
		Type:        "trae",
	}

	// Host
	ts.Host = params["host"]

	// AppToken (userTag)
	ts.AppToken = params["userTag"]
	if ts.AppToken == "" {
		ts.AppToken = params["app_token"]
	}

	// UserInfo parsing
	if userInfoStr := params["userInfo"]; userInfoStr != "" {
		var ui struct {
			NonPlainTextEmail string `json:"NonPlainTextEmail"`
			UserID            string `json:"UserID"`
			Region            string `json:"Region"`
			AIRegion          string `json:"AIRegion"`
		}
		if err := json.Unmarshal([]byte(userInfoStr), &ui); err == nil {
			ts.Email = ui.NonPlainTextEmail
			ts.UserID = ui.UserID
			ts.Region = ui.Region
			ts.AIRegion = ui.AIRegion
		}
	}

	// UserJwt parsing
	if userJwtStr := params["userJwt"]; userJwtStr != "" {
		var uj struct {
			RefreshToken  string `json:"RefreshToken"`
			Token         string `json:"Token"`
			TokenExpireAt int64  `json:"TokenExpireAt"`
			RefreshExpire int64  `json:"RefreshExpireAt"`
		}
		if err := json.Unmarshal([]byte(userJwtStr), &uj); err == nil {
			ts.RefreshToken = uj.RefreshToken
			ts.UserToken = uj.Token
			ts.TokenExpire = uj.TokenExpireAt
			ts.RefreshExpire = uj.RefreshExpire
		}
	}

	// Fallbacks
	if ts.Email == "" {
		ts.Email = params["email"]
	}
	if ts.RefreshToken == "" {
		ts.RefreshToken = params["refreshToken"]
	}

	return ts
}

// ValidateToken is a placeholder for validating the token against Trae's internal API.
func (ta *TraeAuth) ValidateToken(ctx context.Context, appToken string) (string, error) {
	if strings.TrimSpace(appToken) == "" {
		return "", fmt.Errorf("trae: app-token is empty")
	}
	return "trae-user", nil
}

// FetchModels retrieves the list of available models from Trae.
func (ta *TraeAuth) FetchModels(ctx context.Context, ts *TraeTokenStorage) ([]*registry.ModelInfo, error) {
	if ts == nil || (ts.AppToken == "" && ts.UserToken == "") {
		return nil, fmt.Errorf("trae: credentials missing")
	}

	host := ts.Host
	if host == "" {
		host = "https://api-sg-central.trae.ai"
	}
	u := host + "/v1/models"

	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}

	// Trae headers
	if ts.UserToken != "" {
		req.Header.Set("Authorization", "Bearer "+ts.UserToken)
	} else {
		req.Header.Set("Authorization", "Bearer "+ts.AppToken)
	}
	req.Header.Set("App-Token", ts.AppToken)

	resp, err := ta.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trae: fetch models failed with status %d", resp.StatusCode)
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]*registry.ModelInfo, 0, len(result.Data))
	for _, m := range result.Data {
		models = append(models, &registry.ModelInfo{
			ID:      m.ID,
			Object:  "model",
			OwnedBy: m.OwnedBy,
			Type:    "trae",
		})
	}
	return models, nil
}
