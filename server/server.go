package server

import (
	"github.com/kwakubiney/safehaven/config"
	"github.com/songgao/water"
)

type Server struct {
	Config       *config.Config
	TunInterface *water.Interface
}

func NewServer(config *config.Config) *Server {
	return &Server{
		Config: config,
	}
}

func (server *Server) SetTunOnDevice() error {
	ifce, err := water.New(water.Config{DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{Name: server.Config.ClientTunName},
	})
	if err != nil {
		return err
	}
	server.TunInterface = ifce
	return nil
}
