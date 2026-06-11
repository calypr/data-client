package credentials

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/fence"
	"github.com/calypr/calypr-cli/logs"
	"github.com/calypr/calypr-cli/request"
)

// EnsureValidCredential validates a profile credential and refreshes its
// access token from the API key when the token has expired.
func EnsureValidCredential(ctx context.Context, cred *conf.Credential, baseLogger *slog.Logger) error {
	manager := conf.NewConfigure(baseLogger)
	logger := logs.NewGen3Logger(baseLogger, "", cred.Profile)

	valid, err := manager.IsCredentialValid(cred)
	if valid {
		return nil
	}
	if err == nil {
		return fmt.Errorf("invalid credential")
	}

	if !strings.Contains(err.Error(), "access_token is invalid but api_key is valid") {
		return fmt.Errorf("invalid credential: %v", err)
	}

	req := request.NewRequestInterface(logger, cred, manager)
	fClient := fence.NewFenceClient(req, cred, baseLogger)
	newToken, refreshErr := fClient.NewAccessToken(ctx)
	if refreshErr != nil {
		return fmt.Errorf("failed to refresh access token: %v (original error: %v)", refreshErr, err)
	}

	cred.AccessToken = newToken
	if saveErr := manager.Save(cred); saveErr != nil {
		logger.Warn(fmt.Sprintf("failed to save refreshed token: %v", saveErr))
	}
	return nil
}
