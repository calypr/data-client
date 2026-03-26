package localclient

import (
	"github.com/calypr/data-client/conf"
	"github.com/calypr/data-client/credentials"
	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/request"
	"github.com/calypr/data-client/transfer"
	localsigner "github.com/calypr/data-client/transfer/signer/local"
)

// LocalInterface is the local-mode top-level facade.
// It mirrors the Gen3 facade shape for indexing and transfer operations.
type LocalInterface interface {
	request.RequestInterface
	Logger() *logs.Gen3Logger
	Credentials() credentials.Reader
	DRSClient() drs.ServerClient
}

type LocalClient struct {
	request.RequestInterface

	credential *conf.Credential
	creds      credentials.Reader
	logger     *logs.Gen3Logger
	server     drs.ServerClient
}

func NewLocalInterface(profile string, logger *logs.Gen3Logger) (LocalInterface, error) {
	config := conf.NewConfigure(logger.Logger)
	cred, err := config.Load(profile)
	if err != nil {
		return nil, err
	}
	return NewLocalInterfaceFromCredential(cred, logger), nil
}

func NewLocalInterfaceFromCredential(cred *conf.Credential, logger *logs.Gen3Logger) LocalInterface {
	config := conf.NewConfigure(logger.Logger)
	req := request.NewRequestInterface(logger, cred, config)
	dc := drs.NewLocalDrsClient(req, cred.APIEndpoint, logger.Logger)
	tb := transfer.New(req, logger, localsigner.New(cred.APIEndpoint, req, dc))

	return &LocalClient{
		RequestInterface: req,
		credential:       cred,
		creds:            &staticCredentials{cred: cred},
		logger:           logger,
		server:           drs.ComposeServerClient(dc, tb),
	}
}

type staticCredentials struct {
	cred *conf.Credential
}

func (c *staticCredentials) Current() *conf.Credential { return c.cred }

func (l *LocalClient) Logger() *logs.Gen3Logger        { return l.logger }
func (l *LocalClient) Credentials() credentials.Reader { return l.creds }
func (l *LocalClient) DRSClient() drs.ServerClient     { return l.server }
