package githubutil

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var githubSSHHosts = map[string]struct{}{
	"ssh.github.com":    {},
	"altssh.github.com": {},
}

func NormalizeRepositoryURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("repository URL is required")
	}

	host, path, err := splitRepositoryURL(raw)
	if err != nil {
		return "", err
	}

	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.Trim(path, "/")
	path = strings.TrimSuffix(path, ".git")

	parts := strings.Split(path, "/")
	if _, ok := githubSSHHosts[host]; ok {
		host = "github.com"
		if len(parts) == 3 && parts[0] == "443" {
			parts = parts[1:]
		}
	}
	if len(parts) != 2 {
		return "", fmt.Errorf("repository URL must point to a GitHub-style owner/repo path")
	}
	if parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("repository URL must include both owner and repo")
	}

	return host + "/" + parts[0] + "/" + parts[1], nil
}

func ValidateRepositoryURL(ctx context.Context, raw string, token string) (string, error) {
	normalized, err := NormalizeRepositoryURL(raw)
	if err != nil {
		return "", err
	}
	if err := ValidateNormalizedRepositoryURL(ctx, normalized, token); err != nil {
		return "", err
	}
	return normalized, nil
}

func ValidateNormalizedRepositoryURL(ctx context.Context, normalized string, token string) error {
	validationURL := "https://" + normalized + ".git/info/refs?service=git-upload-pack"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, validationURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create validation request: %w", err)
	}
	req.Header.Set("User-Agent", "git/2.43.0")
	if token != "" {
		req.SetBasicAuth("x-access-token", token)
	}

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   5 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach repository at %s: %w", validationURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return nil
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("repository exists but is not accessible with the provided credentials")
	}
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("repository does not exist: %s", normalized)
	}
	return fmt.Errorf("repository validation failed for %s: status %s", normalized, resp.Status)
}

func splitRepositoryURL(raw string) (host string, path string, err error) {
	if strings.Contains(raw, "://") {
		parsed, parseErr := url.Parse(raw)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid repository URL: %w", parseErr)
		}
		if parsed.Host == "" {
			return "", "", fmt.Errorf("repository URL host is required")
		}
		return parsed.Hostname(), parsed.EscapedPath(), nil
	}

	if strings.Contains(raw, "@") && strings.Contains(raw, ":") {
		atIdx := strings.LastIndex(raw, "@")
		colonIdx := strings.Index(raw[atIdx+1:], ":")
		if colonIdx >= 0 {
			host := raw[atIdx+1 : atIdx+1+colonIdx]
			path := raw[atIdx+1+colonIdx+1:]
			return host, path, nil
		}
	}

	parts := strings.Split(strings.Trim(raw, "/"), "/")
	if len(parts) >= 3 {
		return parts[0], strings.Join(parts[1:], "/"), nil
	}

	return "", "", fmt.Errorf("invalid repository URL: %s", raw)
}
