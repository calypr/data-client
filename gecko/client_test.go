package gecko

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/calypr/calypr-cli/conf"
	"github.com/calypr/calypr-cli/request"
	"github.com/hashicorp/go-retryablehttp"
)

type fakeRequest struct {
	doFn func(rb *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeRequest) New(method, u string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: u, Headers: map[string]string{}}
}

func (f *fakeRequest) Do(ctx context.Context, rb *request.RequestBuilder) (*http.Response, error) {
	return f.doFn(rb)
}

func jsonResp(status int, v any) *http.Response {
	var body io.ReadCloser = http.NoBody
	if v != nil {
		buf, _ := json.Marshal(v)
		body = io.NopCloser(bytes.NewReader(buf))
	}
	return &http.Response{StatusCode: status, Body: body}
}

func TestGeckoClientExplorerConfigOperations(t *testing.T) {
	client := &Client{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}

				switch {
				case rb.Method == http.MethodGet && u.Path == "/gecko/explorer/list":
					return jsonResp(http.StatusOK, []string{"default", "study-a"}), nil
				case rb.Method == http.MethodGet && u.Path == "/gecko/explorer/default":
					return jsonResp(http.StatusOK, Config{
						ExplorerConfig: []ConfigItem{{TabTitle: "Cases"}},
					}), nil
				case rb.Method == http.MethodPut && u.Path == "/gecko/explorer/default":
					var payload Config
					if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
						t.Fatalf("decode put payload: %v", err)
					}
					if len(payload.ExplorerConfig) != 1 || payload.ExplorerConfig[0].TabTitle != "Cases" {
						t.Fatalf("unexpected put payload: %+v", payload)
					}
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "ACCEPTED"}), nil
				case rb.Method == http.MethodDelete && u.Path == "/gecko/explorer/default":
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "DELETED"}), nil
				default:
					return jsonResp(http.StatusNotFound, ErrorResponse{
						Error: HTTPError{Code: 404, Message: "not found"},
					}), nil
				}
			},
		},
		Endpoint: "https://example.org",
	}

	configs, err := ListExplorerConfigs(context.Background(), client)
	if err != nil {
		t.Fatalf("ListExplorerConfigs failed: %v", err)
	}
	if len(configs) != 2 || configs[0] != "default" {
		t.Fatalf("unexpected config list: %+v", configs)
	}

	cfg, err := GetExplorerConfig(context.Background(), client, "default")
	if err != nil {
		t.Fatalf("GetExplorerConfig failed: %v", err)
	}
	if len(cfg.ExplorerConfig) != 1 || cfg.ExplorerConfig[0].TabTitle != "Cases" {
		t.Fatalf("unexpected config: %+v", cfg)
	}

	putResp, err := PutExplorerConfig(context.Background(), client, "default", *cfg)
	if err != nil {
		t.Fatalf("PutExplorerConfig failed: %v", err)
	}
	if putResp.Message != "ACCEPTED" {
		t.Fatalf("unexpected put response: %+v", putResp)
	}

	deleteResp, err := DeleteExplorerConfig(context.Background(), client, "default")
	if err != nil {
		t.Fatalf("DeleteExplorerConfig failed: %v", err)
	}
	if deleteResp.Message != "DELETED" {
		t.Fatalf("unexpected delete response: %+v", deleteResp)
	}
}

func TestGeckoClientSupportsAllOfficialConfigTypes(t *testing.T) {
	client := &Client{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}
				switch {
				case rb.Method == http.MethodGet && u.Path == "/gecko/types":
					return jsonResp(http.StatusOK, []string{"explorer", "nav", "file_summary", "apps_page", "project", "projects"}), nil
				case rb.Method == http.MethodGet && u.Path == "/gecko/nav/default":
					return jsonResp(http.StatusOK, NavPageLayoutProps{
						HeaderMetadata: HeaderMetadata{Title: "Nav"},
					}), nil
				case rb.Method == http.MethodPut && u.Path == "/gecko/apps_page/default":
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "ACCEPTED"}), nil
				case rb.Method == http.MethodDelete && u.Path == "/gecko/file_summary/default":
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "DELETED"}), nil
				case rb.Method == http.MethodGet && u.Path == "/gecko/projects/HTAN_INT/BForePC":
					return jsonResp(http.StatusOK, ProjectConfig{Title: "Project"}), nil
				default:
					return jsonResp(http.StatusNotFound, ErrorResponse{
						Error: HTTPError{Code: 404, Message: "not found"},
					}), nil
				}
			},
		},
		Endpoint: "https://example.org",
	}

	types, err := client.ListConfigTypes(context.Background())
	if err != nil {
		t.Fatalf("ListConfigTypes failed: %v", err)
	}
	if len(types) != 6 || types[0] != ConfigTypeExplorer || types[3] != ConfigTypeAppsPage || types[4] != ConfigTypeProject || types[5] != ConfigTypeProjects {
		t.Fatalf("unexpected config types: %+v", types)
	}

	var nav NavPageLayoutProps
	if err := client.GetConfig(context.Background(), ConfigTypeNav, "default", &nav); err != nil {
		t.Fatalf("GetConfig(nav) failed: %v", err)
	}
	if nav.HeaderMetadata.Title != "Nav" {
		t.Fatalf("unexpected nav config: %+v", nav)
	}

	if _, err := client.PutConfig(context.Background(), ConfigTypeAppsPage, "default", AppsConfig{}); err != nil {
		t.Fatalf("PutConfig(apps_page) failed: %v", err)
	}

	if _, err := client.DeleteConfig(context.Background(), ConfigTypeFileSummary, "default"); err != nil {
		t.Fatalf("DeleteConfig(file_summary) failed: %v", err)
	}

	project, err := GetTypedConfig[ProjectConfig](context.Background(), client, ConfigTypeProjects, "HTAN_INT/BForePC")
	if err != nil {
		t.Fatalf("GetTypedConfig(project) failed: %v", err)
	}
	if project.Title != "Project" {
		t.Fatalf("unexpected project config: %+v", project)
	}
}

func TestGeckoClientAppCardAndHealthOperations(t *testing.T) {
	client := &Client{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}

				switch {
				case rb.Method == http.MethodGet && u.Path == "/gecko/apps_page/appcard/PROG-PROJ":
					return jsonResp(http.StatusOK, AppCard{
						Title:       "Portal",
						Description: "Project portal",
						Icon:        "beaker",
						Href:        "/portal",
						Perms:       "PROG-PROJ",
					}), nil
				case rb.Method == http.MethodPost && u.Path == "/gecko/apps_page/appcard/PROG-PROJ":
					var payload AppCard
					if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
						t.Fatalf("decode app card payload: %v", err)
					}
					if payload.Perms != "PROG-PROJ" || payload.Title != "Portal" {
						t.Fatalf("unexpected app card payload: %+v", payload)
					}
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "ACCEPTED"}), nil
				case rb.Method == http.MethodDelete && u.Path == "/gecko/apps_page/appcard/PROG-PROJ":
					return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "DELETED"}), nil
				case rb.Method == http.MethodGet && u.Path == "/gecko/health":
					return jsonResp(http.StatusOK, "Healthy"), nil
				default:
					return jsonResp(http.StatusNotFound, ErrorResponse{
						Error: HTTPError{Type: ErrorTypeNotFound, Code: 404, Message: "not found"},
					}), nil
				}
			},
		},
		Endpoint: "https://example.org",
	}

	card, err := GetAppCard(context.Background(), client, "PROG-PROJ")
	if err != nil {
		t.Fatalf("GetAppCard failed: %v", err)
	}
	if card.Perms != "PROG-PROJ" || card.Title != "Portal" {
		t.Fatalf("unexpected app card: %+v", card)
	}

	upsertResp, err := UpsertAppCard(context.Background(), client, "PROG-PROJ", *card)
	if err != nil {
		t.Fatalf("UpsertAppCard failed: %v", err)
	}
	if upsertResp.Message != "ACCEPTED" {
		t.Fatalf("unexpected upsert response: %+v", upsertResp)
	}

	deleteResp, err := DeleteAppCard(context.Background(), client, "PROG-PROJ")
	if err != nil {
		t.Fatalf("DeleteAppCard failed: %v", err)
	}
	if deleteResp.Message != "DELETED" {
		t.Fatalf("unexpected delete response: %+v", deleteResp)
	}

	health, err := client.HealthCheck(context.Background())
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if health != "Healthy" {
		t.Fatalf("unexpected health response: %q", health)
	}
}

func TestGeckoClientPutProjectConfigValidatesRepository(t *testing.T) {
	oldValidator := validateProjectRepoURL
	validateProjectRepoURL = func(ctx context.Context, raw string, token string) (string, error) {
		if raw != "https://github.com/calypr/gecko" {
			t.Fatalf("unexpected repo URL passed to validator: %q", raw)
		}
		return "github.com/calypr/gecko", nil
	}
	t.Cleanup(func() {
		validateProjectRepoURL = oldValidator
	})

	client := &Client{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}
				if rb.Method != http.MethodPut || u.Path != "/gecko/projects/Org/Project" {
					return jsonResp(http.StatusNotFound, nil), nil
				}
				var payload ProjectConfig
				if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
					t.Fatalf("decode project payload: %v", err)
				}
				if payload.SrcRepo != "github.com/calypr/gecko" {
					t.Fatalf("expected normalized repo, got %q", payload.SrcRepo)
				}
				return jsonResp(http.StatusOK, StatusResponse{Code: 200, Message: "ACCEPTED"}), nil
			},
		},
		Endpoint: "https://example.org",
	}

	if _, err := PutProjectConfig(context.Background(), client, "Org/Project", ProjectConfig{
		Title:        "Project",
		ContactEmail: "user@example.org",
		SrcRepo:      "https://github.com/calypr/gecko",
		OrgTitle:     "Org",
		Description:  "Desc",
		ProjectTitle: "Project",
		IconName:     "beaker",
	}); err != nil {
		t.Fatalf("PutProjectConfig failed: %v", err)
	}
}

func TestGeckoClientReturnsStructuredErrors(t *testing.T) {
	client := &Client{
		RequestInterface: &fakeRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				return jsonResp(http.StatusNotFound, ErrorResponse{
					Error: HTTPError{
						Type:    ErrorTypeConfigNotFound,
						Code:    404,
						Message: "no config found with configId: missing of type: explorer",
						Details: map[string]any{"config_type": "explorer", "config_id": "missing"},
					},
				}), nil
			},
		},
		Endpoint: "https://example.org",
	}

	_, err := GetExplorerConfig(context.Background(), client, "missing")
	if err == nil || err.Error() != "get config: status 404: no config found with configId: missing of type: explorer" {
		t.Fatalf("unexpected error: %v", err)
	}

	var statusErr *request.HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("expected request.HTTPStatusError, got %T", err)
	}
	if statusErr.Type != string(ErrorTypeConfigNotFound) {
		t.Fatalf("unexpected error type: %q", statusErr.Type)
	}
	if statusErr.Details["config_id"] != "missing" {
		t.Fatalf("unexpected error details: %+v", statusErr.Details)
	}
}

func TestNewClientBuildsEndpoint(t *testing.T) {
	req := &request.Request{
		RetryClient: &retryablehttp.Client{HTTPClient: &http.Client{}},
	}
	client := NewClient(req, &conf.Credential{APIEndpoint: "https://example.org/base"}).(*Client)
	if got := client.configListURL(ConfigTypeExplorer); got != "https://example.org/base/gecko/explorer/list" {
		t.Fatalf("unexpected full URL: %s", got)
	}
	if got := client.configItemURL(ConfigTypeProjects, "Org/Project"); got != "https://example.org/base/gecko/projects/Org/Project" {
		t.Fatalf("unexpected project URL: %s", got)
	}
}
