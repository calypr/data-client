package sower

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/calypr/data-client/request"
)

const (
	sowerDispatch  = "/job/dispatch"
	sowerStatus    = "/job/status"
	sowerList      = "/job/list"
	sowerJobOutput = "/job/output"
)

type SowerInterface interface {
	DispatchJob(ctx context.Context, name string, args *DispatchArgs) (*StatusResp, error)
	Status(ctx context.Context, uid string) (*StatusResp, error)
	List(ctx context.Context) ([]StatusResp, error)
	Output(ctx context.Context, uid string) (*OutputResp, error)
}

type SowerClient struct {
	request.RequestInterface
	Endpoint string
}

func NewSowerClient(req request.RequestInterface, endpoint string) *SowerClient {
	return &SowerClient{
		RequestInterface: req,
		Endpoint:         endpoint,
	}
}

func (sc *SowerClient) fullURL(path string) string {
	u, _ := url.Parse(sc.Endpoint)
	u.Path = path
	return u.String()
}

func (sc *SowerClient) DispatchJob(ctx context.Context, name string, args *DispatchArgs) (*StatusResp, error) {
	body := JobArgs{
		Action: name,
		Input:  *args,
	}

	rb := sc.New(http.MethodPost, sc.fullURL(sowerDispatch))
	rb, err := rb.WithJSONBody(body)
	if err != nil {
		return nil, err
	}

	resp, err := sc.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sower dispatch failed: %d %s", resp.StatusCode, string(b))
	}

	statusResp := &StatusResp{}
	err = json.NewDecoder(resp.Body).Decode(statusResp)
	if err != nil {
		return nil, err
	}
	return statusResp, nil
}

func (sc *SowerClient) Status(ctx context.Context, uid string) (*StatusResp, error) {
	u, _ := url.Parse(sc.fullURL(sowerStatus))
	q := u.Query()
	q.Add("UID", uid)
	u.RawQuery = q.Encode()

	rb := sc.New(http.MethodGet, u.String())
	resp, err := sc.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sower status failed: %d %s", resp.StatusCode, string(b))
	}

	statusResp := &StatusResp{}
	err = json.NewDecoder(resp.Body).Decode(statusResp)
	if err != nil {
		return nil, err
	}
	return statusResp, nil
}

func (sc *SowerClient) Output(ctx context.Context, uid string) (*OutputResp, error) {
	u, _ := url.Parse(sc.fullURL(sowerJobOutput))
	q := u.Query()
	q.Add("UID", uid)
	u.RawQuery = q.Encode()

	rb := sc.New(http.MethodGet, u.String())
	resp, err := sc.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sower output failed: %d %s", resp.StatusCode, string(b))
	}

	var outputResp OutputResp
	err = json.NewDecoder(resp.Body).Decode(&outputResp)
	if err != nil {
		return nil, err
	}
	return &outputResp, nil
}

func (sc *SowerClient) List(ctx context.Context) ([]StatusResp, error) {
	rb := sc.New(http.MethodGet, sc.fullURL(sowerList))
	resp, err := sc.Do(ctx, rb)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("sower list failed: %d %s", resp.StatusCode, string(b))
	}

	var listResp []StatusResp
	err = json.NewDecoder(resp.Body).Decode(&listResp)
	if err != nil {
		return nil, err
	}
	return listResp, nil
}
