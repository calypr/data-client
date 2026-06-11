package sower

import (
	"context"
	"net/http"
	"net/url"

	"github.com/calypr/calypr-cli/request"
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
	return request.JoinURL(sc.Endpoint, path)
}

func (sc *SowerClient) DispatchJob(ctx context.Context, name string, args *DispatchArgs) (*StatusResp, error) {
	body := JobArgs{
		Action: name,
		Input:  *args,
	}

	rb, err := request.NewJSON(sc.RequestInterface, http.MethodPost, sc.fullURL(sowerDispatch), body)
	if err != nil {
		return nil, err
	}

	statusResp := &StatusResp{}
	if err := request.DoJSON(
		ctx,
		sc.RequestInterface,
		rb,
		statusResp,
		request.WithAction("sower dispatch failed"),
		request.WithExpectedStatus(http.StatusOK),
	); err != nil {
		return nil, err
	}
	return statusResp, nil
}

func (sc *SowerClient) Status(ctx context.Context, uid string) (*StatusResp, error) {
	u, _ := url.Parse(sc.fullURL(sowerStatus))
	q := u.Query()
	q.Add("UID", uid)
	u.RawQuery = q.Encode()

	statusResp := &StatusResp{}
	if err := request.DoJSON(
		ctx,
		sc.RequestInterface,
		sc.New(http.MethodGet, u.String()),
		statusResp,
		request.WithAction("sower status failed"),
		request.WithExpectedStatus(http.StatusOK),
	); err != nil {
		return nil, err
	}
	return statusResp, nil
}

func (sc *SowerClient) Output(ctx context.Context, uid string) (*OutputResp, error) {
	u, _ := url.Parse(sc.fullURL(sowerJobOutput))
	q := u.Query()
	q.Add("UID", uid)
	u.RawQuery = q.Encode()

	var outputResp OutputResp
	if err := request.DoJSON(
		ctx,
		sc.RequestInterface,
		sc.New(http.MethodGet, u.String()),
		&outputResp,
		request.WithAction("sower output failed"),
		request.WithExpectedStatus(http.StatusOK),
	); err != nil {
		return nil, err
	}
	return &outputResp, nil
}

func (sc *SowerClient) List(ctx context.Context) ([]StatusResp, error) {
	var listResp []StatusResp
	if err := request.DoJSON(
		ctx,
		sc.RequestInterface,
		sc.New(http.MethodGet, sc.fullURL(sowerList)),
		&listResp,
		request.WithAction("sower list failed"),
		request.WithExpectedStatus(http.StatusOK),
	); err != nil {
		return nil, err
	}
	return listResp, nil
}
