package server

import (
	"net"

	"github.com/kwakubiney/safehaven/config"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/songgao/water"
)

type Server struct {
	Config       *config.Config
	TunInterface *water.Interface
	UDPConn      *net.UDPConn
	ConnMap      cmap.ConcurrentMap[string, net.Addr]
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
