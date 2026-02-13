package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v6/internal/auth/trae"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v6/sdk/cliproxy/auth"
)

// TraeAuthenticator implements the Authenticator interface for Trae.
type TraeAuthenticator struct{}

// NewTraeAuthenticator creates a new Trae authenticator instance.
func NewTraeAuthenticator() *TraeAuthenticator {
	return &TraeAuthenticator{}
}

// Provider returns the provider name for Trae.
func (a *TraeAuthenticator) Provider() string {
	return "trae"
}

// Login initiates the Trae authentication flow.
func (a *TraeAuthenticator) Login(ctx context.Context, cfg *config.Config, opts *LoginOptions) (*coreauth.Auth, error) {
	// For Trae, we use a bridge page/callback flow since official OAuth is not public.
	// But to match the CLI -trae-login flag, we implement it here.
	
	// Trae doesn't have a standard OAuth redirect that we can fully automate easily
	// without the user's help in extracting the token.
	// So we reuse the logic from management handler but adapted for CLI.
	
	fmt.Println("Please login to Trae in your browser and provide the app-token.")
	
	// In CLI mode, we could either:
	// 1. Open the bridge page locally and wait for callback.
	// 2. Just ask for the token in the terminal.
	
	// Let's implement the local server + callback flow to be consistent.
	port := 8871 // Matching observed Trae callback port
	
	traeAuth := trae.NewTraeAuth(cfg)
	
	// Generate a state/trace ID
	state := fmt.Sprintf("%d", time.Now().UnixNano())
	
	callbackURL := fmt.Sprintf("http://127.0.0.1:%d/authorize", port)
	authURL := traeAuth.GenerateAuthURL(state, callbackURL)
	
	if !opts.NoBrowser {
		fmt.Printf("Opening browser for Trae authentication: %s\n", authURL)
		// We could use a library to open the browser, but for now we print it.
	} else {
		fmt.Printf("Please open the following URL in your browser: %s\n", authURL)
	}
	
	// Start a local listener to catch the callback
	resultChan := make(chan map[string]string, 1)
	server := &http.Server{Addr: fmt.Sprintf(":%d", port)}
	
	http.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		res := make(map[string]string)
		fmt.Printf("Callback received: %s\n", r.URL.String())
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				res[k] = v[0]
				fmt.Printf("  Param %s: %s\n", k, v[0])
			}
		}
		resultChan <- res
		fmt.Fprintf(w, "<html><body><h1>Authentication Successful!</h1><p>You can close this window now.</p></body></html>")
	})
	
	go func() {
		_ = server.ListenAndServe()
	}()
	defer func() {
		_ = server.Shutdown(context.Background())
	}()
	
	fmt.Println("Waiting for authentication callback on port", port, "...")
	
	var resultMap map[string]string
	select {
	case resultMap = <-resultChan:
		// Got it
	case <-time.After(5 * time.Minute):
		return nil, fmt.Errorf("authentication timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	
	tokenStorage := traeAuth.CreateTokenStorage(resultMap)

	if tokenStorage.AppToken == "" && tokenStorage.UserToken == "" {
		return nil, fmt.Errorf("no valid token received in callback")
	}

	email := tokenStorage.Email
	if email == "" {
		email = "trae-user"
	}

	fileName := fmt.Sprintf("trae-%s.json", email)

	return &coreauth.Auth{
		ID:       fileName,
		Provider: "trae",
		FileName: fileName,
		Storage:  tokenStorage,
		Metadata: map[string]any{"email": email, "type": "trae"},
	}, nil
}

// RefreshLead returns the lead time for token refresh. Trae doesn't support refresh.
func (a *TraeAuthenticator) RefreshLead() *time.Duration {
	return nil
}
