package request

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type ErrorMessageProvider interface {
	ErrorMessage() string
}

type ErrorTypeProvider interface {
	ErrorType() string
}

type ErrorDetailsProvider interface {
	ErrorDetails() map[string]any
}

type HTTPStatusError struct {
	Action     string
	StatusCode int
	Message    string
	Type       string
	Details    map[string]any
	RawBody    string
}

func (e *HTTPStatusError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("%s: status %d: %s", e.Action, e.StatusCode, e.Message)
	}
	if e.RawBody == "" {
		return fmt.Sprintf("%s: status %d", e.Action, e.StatusCode)
	}
	return fmt.Sprintf("%s: status %d, body: %s", e.Action, e.StatusCode, e.RawBody)
}

type JSONOption func(*jsonOptions)

type jsonOptions struct {
	action        string
	errorEnvelope ErrorMessageProvider
	expectedCodes map[int]struct{}
}

func WithAction(action string) JSONOption {
	return func(opts *jsonOptions) {
		opts.action = action
	}
}

func WithErrorEnvelope(envelope ErrorMessageProvider) JSONOption {
	return func(opts *jsonOptions) {
		opts.errorEnvelope = envelope
	}
}

func WithExpectedStatus(codes ...int) JSONOption {
	return func(opts *jsonOptions) {
		opts.expectedCodes = make(map[int]struct{}, len(codes))
		for _, code := range codes {
			opts.expectedCodes[code] = struct{}{}
		}
	}
}

func JoinURL(base string, parts ...string) string {
	u, _ := url.Parse(strings.TrimSpace(base))

	joined := make([]string, 0, len(parts)+1)
	if basePath := strings.Trim(u.Path, "/"); basePath != "" {
		joined = append(joined, basePath)
	}
	for _, part := range parts {
		if trimmed := strings.Trim(part, "/"); trimmed != "" {
			joined = append(joined, trimmed)
		}
	}

	if len(joined) == 0 {
		u.Path = ""
		return u.String()
	}

	u.Path = "/" + strings.Join(joined, "/")
	return u.String()
}

func NewJSON(req RequestInterface, method, url string, body any) (*RequestBuilder, error) {
	rb := req.New(method, url)
	if body == nil {
		return rb, nil
	}
	return rb.WithJSONBody(body)
}

func DoJSON(ctx context.Context, req RequestInterface, rb *RequestBuilder, out any, opts ...JSONOption) error {
	options := jsonOptions{
		action: "request failed",
	}
	for _, opt := range opts {
		opt(&options)
	}

	resp, err := req.Do(ctx, rb)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if !statusAccepted(resp.StatusCode, options.expectedCodes) {
		if options.errorEnvelope != nil {
			return StatusErrorJSON(resp, options.action, options.errorEnvelope)
		}
		return StatusError(resp, options.action)
	}

	if out == nil {
		return nil
	}
	return DecodeJSON(resp, out)
}

func DecodeJSON(resp *http.Response, out any) error {
	return json.NewDecoder(resp.Body).Decode(out)
}

func StatusError(resp *http.Response, action string) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	return statusErrorFromBody(resp.StatusCode, action, bodyBytes)
}

func StatusErrorJSON(resp *http.Response, action string, out ErrorMessageProvider) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(bodyBytes, out); err == nil {
		if message := strings.TrimSpace(out.ErrorMessage()); message != "" {
			statusErr := &HTTPStatusError{
				Action:     action,
				StatusCode: resp.StatusCode,
				Message:    message,
				RawBody:    strings.TrimSpace(string(bodyBytes)),
			}
			if typed, ok := out.(ErrorTypeProvider); ok {
				statusErr.Type = strings.TrimSpace(typed.ErrorType())
			}
			if detailed, ok := out.(ErrorDetailsProvider); ok {
				statusErr.Details = detailed.ErrorDetails()
			}
			return statusErr
		}
	}
	return statusErrorFromBody(resp.StatusCode, action, bodyBytes)
}

func statusAccepted(statusCode int, expectedCodes map[int]struct{}) bool {
	if len(expectedCodes) == 0 {
		return statusCode < http.StatusBadRequest
	}
	_, ok := expectedCodes[statusCode]
	return ok
}

func statusErrorFromBody(statusCode int, action string, bodyBytes []byte) error {
	body := strings.TrimSpace(string(bodyBytes))
	return &HTTPStatusError{
		Action:     action,
		StatusCode: statusCode,
		RawBody:    body,
	}
}
