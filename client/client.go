package client

import (
	"log"

	"github.com/kwakubiney/safehaven/config"
	"github.com/songgao/water"
)

type Client struct{
	Config *config.Config
	Tun  *water.Interface	
}

func NewClient(config *config.Config) *Client{
	return &Client{
		Config: config,
	}
}

//sets tun interface on client
func (client *Client) SetTunOnClient() error {
	ifce, err := water.New(water.Config{DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{Name: client.Config.ClientTunName},
	})
	log.Println(ifce)
	if err != nil{
		return err
	}
	client.Tun = ifce
	return nil	
}



