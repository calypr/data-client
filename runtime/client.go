package runtime

import (
	"fmt"
	"strings"

	"github.com/calypr/data-client/drs"
	"github.com/calypr/data-client/g3client"
	"github.com/calypr/data-client/localclient"
	"github.com/calypr/data-client/logs"
	"github.com/calypr/data-client/transfer"
)

// Client composes metadata and transfer concerns for a selected runtime mode.
type Client struct {
	g3  g3client.Gen3Interface
	drs drs.ServerClient
}

func New(profile string, mode string, logger *logs.Gen3Logger) (*Client, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "gen3":
		g3, err := g3client.NewGen3Interface(profile, logger)
		if err != nil {
			return nil, err
		}
		return &Client{
			g3:  g3,
			drs: g3.DRSClient(),
		}, nil
	case "drs":
		lc, err := localclient.NewLocalInterface(profile, logger)
		if err != nil {
			return nil, err
		}
		return &Client{
			g3:  nil,
			drs: lc.DRSClient(),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported backend mode %q", mode)
	}
}

func (c *Client) Gen3() g3client.Gen3Interface { return c.g3 }
func (c *Client) DRS() drs.ServerClient        { return c.drs }
func (c *Client) Transfer() transfer.Backend   { return c.drs }
