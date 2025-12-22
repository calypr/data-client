package conf

import (
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func ValidateUrl(apiEndpoint string) (*url.URL, error) {
	parsedURL, err := url.Parse(apiEndpoint)
	if err != nil {
		return parsedURL, errors.New("Error occurred when parsing apiendpoint URL: " + err.Error())
	}
	if parsedURL.Host == "" {
		return parsedURL, errors.New("Invalid endpoint. A valid endpoint looks like: https://www.tests.com")
	}
	return parsedURL, nil
}

func (man *Manager) IsValid(profileConfig *Credential) (bool, error) {
	if profileConfig == nil {
		return false, fmt.Errorf("profileConfig is nil")
	}
	/* Checks to see if credential in credential file is still valid */
	// Parse the token without verifying the signature to access the claims.
	token, _, err := new(jwt.Parser).ParseUnverified(profileConfig.APIKey, jwt.MapClaims{})
	if err != nil {
		return false, fmt.Errorf("invalid token format: %v", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return false, fmt.Errorf("unable to parse claims from provided token %#v", token)
	}

	exp, ok := claims["exp"].(float64)
	if !ok {
		return false, fmt.Errorf("'exp' claim not found or is not a number for claims %s", claims)
	}

	iat, ok := claims["iat"].(float64)
	if !ok {
		return false, fmt.Errorf("'iat' claim not found or is not a number for claims %s", claims)
	}

	now := time.Now().UTC()
	expTime := time.Unix(int64(exp), 0).UTC()
	iatTime := time.Unix(int64(iat), 0).UTC()

	if expTime.Before(now) {
		return false, fmt.Errorf("key %s expired %s < %s", profileConfig.APIKey, expTime.Format(time.RFC3339), now.Format(time.RFC3339))
	}
	if iatTime.After(now) {
		return false, fmt.Errorf("key %s not yet valid %s > %s", profileConfig.APIKey, iatTime.Format(time.RFC3339), now.Format(time.RFC3339))
	}

	delta := expTime.Sub(now)
	// threshold days set to 10
	if delta > 0 && delta.Hours() < float64(10*24) {
		daysUntilExpiration := int(delta.Hours() / 24)
		if daysUntilExpiration > 0 {
			return true, fmt.Errorf("warning %s: Key will expire in %d days, on %s", profileConfig.APIKey, daysUntilExpiration, expTime.Format(time.RFC3339))
		}
	}
	return true, nil
}
