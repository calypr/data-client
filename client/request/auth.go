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

	t.mu.Lock()
	t.Cred.AccessToken = result.AccessToken
	if t.Manager != nil {
		t.Manager.Save(t.Cred)
	}
	t.mu.Unlock()
	return nil
}

type AuthTransport struct {
	Manager   conf.ManagerInterface
	Base      http.RoundTripper
	Cred      *conf.Credential
	mu        sync.RWMutex
	refreshMu sync.Mutex
}

func (t *AuthTransport) RoundTrip(req *http.Request) (*http.Response, error) {

	resp, err := t.Base.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusBadGateway {
		resp.Body.Close()

		newToken, refreshErr := t.tryRefresh(req.Context())
		if refreshErr != nil {
			return nil, refreshErr
		}

		retryReq := req.Clone(req.Context())
		retryReq.Header.Set("Authorization", "Bearer "+newToken)
		return t.Base.RoundTrip(retryReq)
	}

	return resp, nil
}

func (t *AuthTransport) tryRefresh(ctx context.Context) (string, error) {
	// Only one goroutine can enter this block
	t.refreshMu.Lock()
	defer t.refreshMu.Unlock()

	if err := t.NewAccessToken(ctx); err != nil {
		return "", err
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.Cred.AccessToken, nil
}
