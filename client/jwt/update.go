package jwt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/calypr/data-client/client/common"
	"github.com/calypr/data-client/client/logs"
	"github.com/hashicorp/go-version"
)

func UpdateConfig(logger logs.Logger, cred *Credential) error {
	var conf Configure
	var req Request = Request{Ctx: context.Background()}

	if cred.Profile == "" {
		return fmt.Errorf("profile name is required")
	}
	if cred.APIEndpoint == "" {
		return fmt.Errorf("API endpoint is required")
	}

	// Normalize endpoint
	cred.APIEndpoint = strings.TrimSpace(cred.APIEndpoint)
	cred.APIEndpoint = strings.TrimSuffix(cred.APIEndpoint, "/")

	// Validate URL format
	parsedURL, err := conf.ValidateUrl(cred.APIEndpoint)
	if err != nil {
		return fmt.Errorf("invalid apiendpoint URL: %w", err)
	}
	fenceBase := parsedURL.Scheme + "://" + parsedURL.Host
	if existingCfg, err := conf.ParseConfig(cred.Profile); err == nil {
		// Only copy optional fields if the user didn't override them via flags
		if cred.UseShepherd == "" {
			cred.UseShepherd = existingCfg.UseShepherd
		}
		if cred.MinShepherdVersion == "" {
			cred.MinShepherdVersion = existingCfg.MinShepherdVersion
		}
	} else if !errors.Is(err, ErrProfileNotFound) {
		return err
	}

	if cred.APIKey != "" {
		// Always refresh the access token — ignore any old one that might be in the struct
		err = req.RequestNewAccessToken(fenceBase+common.FenceAccessTokenEndpoint, cred)
		if err != nil {
			if strings.Contains(err.Error(), "401") {
				return fmt.Errorf("authentication failed (401) for %s — your API key is invalid, revoked, or expired", fenceBase)
			}
			if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "no such host") {
				return fmt.Errorf("cannot reach Fence at %s — is this a valid Gen3 commons?", fenceBase)
			}
			return fmt.Errorf("failed to refresh access token: %w", err)
		}
	} else {
		logger.Printf("WARNING: Your profile will only be valid for 24 hours since you have only provided a refresh token for authentication")
	}

	// Clean up shepherd flags
	cred.UseShepherd = strings.TrimSpace(cred.UseShepherd)
	cred.MinShepherdVersion = strings.TrimSpace(cred.MinShepherdVersion)

	if cred.MinShepherdVersion != "" {
		if _, err = version.NewVersion(cred.MinShepherdVersion); err != nil {
			return fmt.Errorf("invalid min-shepherd-version: %w", err)
		}
	}

	if err := conf.UpdateConfigFile(*cred); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
