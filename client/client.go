package client

import (
	"github.com/kwakubiney/safehaven/config"
	"github.com/songgao/water"
)

type Client struct {
	Config       *config.Config
	TunInterface *water.Interface
}

func NewClient(config *config.Config) *Client {
	return &Client{
		Config: config,
	}
}

func (client *Client) SetTunOnClient() error {
	ifce, err := water.New(water.Config{DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{Name: client.Config.ClientTunName},
	})
	if err != nil {
		return err
	}
	client.TunInterface = ifce
	return nil
}
