package auth

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/miromind"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/browser"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

const (
	DefaultMiroMindCallbackPort = 8086
)

// MiroMindAuthenticator implements the login for MiroMind accounts.
type MiroMindAuthenticator struct{}

// NewMiroMindAuthenticator constructs a MiroMind authenticator.
func NewMiroMindAuthenticator() *MiroMindAuthenticator {
	return &MiroMindAuthenticator{}
}

func (a *MiroMindAuthenticator) Provider() string {
	return "miromind"
}

func (a *MiroMindAuthenticator) RefreshLead() *time.Duration {
	d := 24 * time.Hour
	return &d
}

func (a *MiroMindAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cliproxy auth: configuration is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if opts == nil {
		opts = &LoginOptions{}
	}

	if opts.Prompt == nil {
		return nil, fmt.Errorf("cliproxy auth: prompt function is required for MiroMind login")
	}

	callbackPort := DefaultMiroMindCallbackPort
	if opts.CallbackPort > 0 {
		callbackPort = opts.CallbackPort
	}

	tokenChan := make(chan string, 1)
	errChan := make(chan error, 1)

	mux := http.NewServeMux()
	server := &http.Server{Addr: fmt.Sprintf(":%d", callbackPort), Handler: mux}

	// Serve a visual guide page at "/" that instructs the user how to capture the token.
	guideHTML := fmt.Sprintf(`<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<title>MiroMind Login Helper</title>
<style>
  body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; max-width: 700px; margin: 40px auto; padding: 20px; background: #1a1a2e; color: #e0e0e0; }
  h1 { color: #64ffda; }
  .step { background: #16213e; border-radius: 12px; padding: 20px; margin: 16px 0; border-left: 4px solid #64ffda; }
  .step h3 { color: #64ffda; margin-top: 0; }
  a.bookmarklet { display: inline-block; background: linear-gradient(135deg, #667eea, #764ba2); color: white; padding: 12px 24px; border-radius: 8px; text-decoration: none; font-weight: bold; font-size: 16px; cursor: grab; }
  a.bookmarklet:hover { transform: scale(1.05); box-shadow: 0 4px 15px rgba(102,126,234,0.4); }
  .highlight { background: #0f3460; padding: 4px 8px; border-radius: 4px; font-family: monospace; }
  .success { color: #64ffda; font-weight: bold; }
</style>
</head>
<body>
<h1>🔑 MiroMind Token Capture</h1>
<div class="step">
  <h3>Step 1: Drag this to your Bookmarks Bar</h3>
  <p>⬇️ Drag the button below to your browser's bookmarks bar:</p>
  <p><a class="bookmarklet" href="javascript:void(location.href='http://localhost:%d/callback?token='+encodeURIComponent(document.cookie))">🔑 Capture MiroMind Token</a></p>
</div>
<div class="step">
  <h3>Step 2: Go to MiroMind</h3>
  <p>Visit <a href="https://dr.miromind.ai/chat" target="_blank" style="color:#64ffda">https://dr.miromind.ai/chat</a> and log in (or you may already be logged in).</p>
</div>
<div class="step">
  <h3>Step 3: Click the Bookmarklet</h3>
  <p>Once you are on the MiroMind chat page, click the <span class="highlight">🔑 Capture MiroMind Token</span> bookmark you just saved.</p>
  <p>The page will redirect here and your token will be captured automatically!</p>
</div>
<hr>
<p style="font-size:13px; color:#888;">Waiting for token callback... This page will update when the token is received.</p>
</body>
</html>`, callbackPort)

	// Serve the guide page
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, guideHTML)
	})

	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token := r.URL.Query().Get("token")
		if token == "" {
			fmt.Fprintf(w, "<html><body><h1>Missing token</h1><p>No token parameter found. Please try again.</p></body></html>")
			return
		}
		// URL-decode the token (it was encodeURIComponent'd by the bookmarklet)
		if decoded, decErr := url.QueryUnescape(token); decErr == nil {
			token = decoded
		}
		// Clean the token if it came from document.cookie (might have multiple cookies)
		if strings.Contains(token, "__client=") {
			parts := strings.Split(token, ";")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if strings.HasPrefix(p, "__client=") {
					token = p
					break
				}
			}
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><style>body{font-family:sans-serif;text-align:center;padding:60px;background:#1a1a2e;color:#64ffda;}</style></head><body><h1>✅ Token Captured!</h1><p>You can close this window now. Return to the terminal.</p></body></html>`)
		select {
		case tokenChan <- token:
		default:
		}
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			select {
			case errChan <- err:
			default:
			}
		}
	}()
	defer server.Shutdown(ctx)

	// Open browser to the LOCAL guide page, NOT dr.miromind.ai
	if !opts.NoBrowser {
		guideURL := fmt.Sprintf("http://localhost:%d/", callbackPort)
		fmt.Printf("Opening token capture guide: %s\n", guideURL)
		if err := browser.OpenURL(guideURL); err != nil {
			fmt.Printf("Warning: failed to open browser: %v\n", err)
			fmt.Printf("Please manually visit: %s\n", guideURL)
		}
	}

	fmt.Println("\n--- MiroMind Automated Login ---")
	fmt.Printf("Guide page: http://localhost:%d/\n", callbackPort)
	fmt.Println("Waiting for token capture via bookmarklet callback...")

	var sessionToken string
	select {
	case t := <-tokenChan:
		sessionToken = t
		fmt.Println("\nToken captured automatically!")
	case err := <-errChan:
		fmt.Printf("Warning: local server failed: %v\n", err)
	case <-time.After(5 * time.Minute):
		fmt.Println("Auto-capture timed out.")
	}

	if sessionToken == "" {
		// Use manual prompt as fallback if auto-capture doesn't happen
		t, err := opts.Prompt("Paste the Session Token or full URL here:")
		if err != nil {
			return nil, err
		}
		sessionToken = strings.TrimSpace(t)
	}

	if sessionToken == "" {
		return nil, fmt.Errorf("session token is required")
	}

	// Simple URL extraction if the user pasted a full callback URL
	if strings.Contains(sessionToken, "callback?token=") {
		if u, err := url.Parse(sessionToken); err == nil {
			sessionToken = u.Query().Get("token")
		}
	}

	email := "miromind-user"

	tokenStorage := &miromind.MiroMindTokenStorage{
		SessionToken: sessionToken,
		Email:        email,
		LastRefresh:  time.Now().Format(time.RFC3339),
		Type:         "miromind",
		Expire:       time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339),
	}

	fileName := fmt.Sprintf("miromind-%s.json", strings.ReplaceAll(email, "@", "_"))
	metadata := map[string]any{
		"email": tokenStorage.Email,
		"type":  "miromind",
	}

	fmt.Println("MiroMind authentication configured")

	return &coreauth.Auth{
		ID:       fileName,
		Provider: a.Provider(),
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: metadata,
	}, nil
}
