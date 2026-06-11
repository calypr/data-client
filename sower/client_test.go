package sower

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/calypr/calypr-cli/request"
	"github.com/hashicorp/go-retryablehttp"
)

type fakeSowerRequest struct {
	doFn func(rb *request.RequestBuilder) (*http.Response, error)
}

func (f *fakeSowerRequest) New(method, u string) *request.RequestBuilder {
	return &request.RequestBuilder{Method: method, Url: u, Headers: map[string]string{}}
}

func (f *fakeSowerRequest) Do(ctx context.Context, rb *request.RequestBuilder) (*http.Response, error) {
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

func TestSowerClientOperations(t *testing.T) {
	client := &SowerClient{
		RequestInterface: &fakeSowerRequest{
			doFn: func(rb *request.RequestBuilder) (*http.Response, error) {
				u, err := url.Parse(rb.Url)
				if err != nil {
					return nil, err
				}
				switch {
				case rb.Method == http.MethodPost && u.Path == sowerDispatch:
					var payload JobArgs
					if err := json.NewDecoder(rb.Body).Decode(&payload); err != nil {
						t.Fatalf("decode dispatch payload: %v", err)
					}
					if payload.Action != "dispatch" {
						t.Fatalf("unexpected dispatch action: %q", payload.Action)
					}
					if payload.Input.Profile != "profile-a" {
						t.Fatalf("unexpected profile: %q", payload.Input.Profile)
					}
					return jsonResp(http.StatusOK, StatusResp{Uid: "uid-1", Name: "job-1", Status: "running"}), nil
				case rb.Method == http.MethodGet && u.Path == sowerStatus:
					if got := u.Query().Get("UID"); got != "uid-1" {
						t.Fatalf("unexpected UID query: %q", got)
					}
					return jsonResp(http.StatusOK, StatusResp{Uid: "uid-1", Name: "job-1", Status: "complete"}), nil
				case rb.Method == http.MethodGet && u.Path == sowerJobOutput:
					if got := u.Query().Get("UID"); got != "uid-1" {
						t.Fatalf("unexpected UID query: %q", got)
					}
					return jsonResp(http.StatusOK, OutputResp{Output: "done"}), nil
				case rb.Method == http.MethodGet && u.Path == sowerList:
					return jsonResp(http.StatusOK, []StatusResp{{Uid: "uid-1", Name: "job-1", Status: "complete"}}), nil
				default:
					return jsonResp(http.StatusNotFound, nil), nil
				}
			},
		},
		Endpoint: "https://example.org",
	}

	status, err := client.DispatchJob(context.Background(), "dispatch", &DispatchArgs{
		Profile:     "profile-a",
		APIEndpoint: "https://example.org",
	})
	if err != nil {
		t.Fatalf("DispatchJob failed: %v", err)
	}
	if status.Uid != "uid-1" {
		t.Fatalf("unexpected dispatch result: %+v", status)
	}

	gotStatus, err := client.Status(context.Background(), "uid-1")
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}
	if gotStatus.Status != "complete" {
		t.Fatalf("unexpected status result: %+v", gotStatus)
	}

	output, err := client.Output(context.Background(), "uid-1")
	if err != nil {
		t.Fatalf("Output failed: %v", err)
	}
	if output.Output != "done" {
		t.Fatalf("unexpected output result: %+v", output)
	}

	list, err := client.List(context.Background())
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 || list[0].Uid != "uid-1" {
		t.Fatalf("unexpected list result: %+v", list)
	}
}

func TestNewSowerClientBuildsEndpoint(t *testing.T) {
	req := &request.Request{
		RetryClient: &retryablehttp.Client{HTTPClient: &http.Client{}},
	}
	client := NewSowerClient(req, "https://example.org")
	if client.fullURL(sowerDispatch) != "https://example.org/job/dispatch" {
		t.Fatalf("unexpected full URL: %s", client.fullURL(sowerDispatch))
	}
}
