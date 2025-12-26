package request

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/conf"
)

func (t *AuthTransport) NewAccessToken(ctx context.Context) error {
	if t.Cred.APIKey == "" {
		return errors.New("APIKey is required to refresh access token")
	}

	// Important: Use t.Base (the raw transport) for the refresh call.
	// If you use an http.Client that points back to AuthTransport,
	// you will trigger an infinite loop/deadlock.
	refreshClient := &http.Client{Transport: t.Base}

	payload := map[string]string{"api_key": t.Cred.APIKey}
	reader, err := common.ToJSONReader(payload)
	if err != nil {
		return err
	}

	refreshUrl := t.Cred.APIEndpoint + common.FenceAccessTokenEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, refreshUrl, reader)
	if err != nil {
		return err
	}
	req.Header.Set(common.HeaderContentType, common.MIMEApplicationJSON)

	resp, err := refreshClient.Do(req)
	if err != nil {
		return fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("failed to refresh token, status: " + strconv.Itoa(resp.StatusCode))
	}

	var result common.AccessTokenStruct
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Thread-safe update of the credential
	t.mu.Lock()
	t.Cred.AccessToken = result.AccessToken
	t.mu.Unlock()

	return nil
}

type AuthTransport struct {
	Base      http.RoundTripper
	Cred      *conf.Credential
	mu        sync.RWMutex
	refreshMu sync.Mutex
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 1. Thread-safe read of the current token
	t.mu.RLock()
	token := t.Cred.AccessToken
	t.mu.RUnlock()

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	// 2. Execute the request
	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	// 3. Check for auth failure
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close() // Must close the body before retrying

		// 4. Critical: tryRefresh handles the "Thundering Herd" problem
		newToken, refreshErr := t.tryRefresh(req.Context(), token)
		if refreshErr != nil {
			return nil, refreshErr
		}

		// 5. Retry the request with the new token
		retryReq := req.Clone(req.Context())
		retryReq.Header.Set("Authorization", "Bearer "+newToken)
		return t.Base.RoundTrip(retryReq)
	}

	return resp, nil
}

func (t *AuthTransport) tryRefresh(ctx context.Context, failedToken string) (string, error) {
	// Only one goroutine can enter this block
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	// Double-Check: Has someone else refreshed it while we were waiting for the lock?
	t.mu.RLock()
	currentToken := t.Cred.AccessToken
	t.mu.RUnlock()

	if currentToken != failedToken && currentToken != "" {
		return currentToken, nil // Success, someone else did the work!
	}

	// If we are here, we are the designated "refresher"
	if err := t.NewAccessToken(ctx); err != nil {
		return "", err
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Cred.AccessToken, nil
}
