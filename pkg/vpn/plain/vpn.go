package plain

import (
	"fmt"
	"github.com/kwakubiney/safehaven/client"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/kwakubiney/safehaven/server"
	"github.com/kwakubiney/safehaven/utils"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"log"
	"net"
	"strconv"
)

type PlainVPN struct {
	config    *config.Config
	tunDevice *water.Interface
	conn      net.Conn
}

func NewPlainVPN(config *config.Config) vpn.VPNService {
	return &PlainVPN{
		config: config,
	}
}

func (p *PlainVPN) Start() error {
	if p.config.ServerMode {
		return p.startServer()
	}
	return p.startClient()
}

func (p *PlainVPN) Stop() error {
	p.conn.Close()
	p.tunDevice.Close()
	return nil
}

func (p *PlainVPN) startClient() error {
	vpnClient := client.NewClient(p.config)
	err := vpnClient.SetTunOnDevice()
	if err != nil {
		return err
	}
	err = p.assignIPToTun()
	if err != nil {
		return err
	}

	err = p.createRoutes()
	if err != nil {
		return err
	}

	packet := make([]byte, 65535)
	clientConn, err := net.Dial("udp", p.config.ServerAddress)

	if err != nil {
		return err
	}

	//receive
	go func() {
		for {
			packet := make([]byte, 65535)
			n, err := clientConn.Read(packet)
			if err != nil {
				log.Println(err)
				continue
			}
			_, err = vpnClient.TunInterface.Write(packet[:n])
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}()

	//send
	for {
		n, err := vpnClient.TunInterface.Read(packet)
		if err != nil {
			log.Println(err)
			break
		}

		_, err = clientConn.Write(packet[:n])
		if err != nil {
			log.Println(err)
			continue
		}
	}
	return nil
}

func (p *PlainVPN) startServer() error {
	vpnServer := server.NewServer(p.config)
	err := vpnServer.SetTunOnDevice()
	if err != nil {
		return err
	}
	vpnServer.ConnMap = cmap.New[net.Addr]()
	err = p.assignIPToTun()
	if err != nil {
		return err
	}

	err = p.createRoutes()
	if err != nil {
		return err
	}

	localAddress, _ := strconv.Atoi(p.config.LocalAddress)
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localAddress})
	if err != nil {
		return err
	}
	defer serverConn.Close()
	defer vpnServer.TunInterface.Close()
	go func() {
		for {
			packet := make([]byte, 65535)

			n, clientAddr, err := serverConn.ReadFrom(packet)
			if err != nil {
				log.Println(err)
				continue
			}
			sourceIPAddress := utils.ResolveSourceIPAddressFromRawPacket(packet)
			vpnServer.ConnMap.Set(sourceIPAddress, clientAddr)
			_, err = vpnServer.TunInterface.Write(packet[:n])
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}()

	for {
		packet := make([]byte, 1500)
		n, err := vpnServer.TunInterface.Read(packet)
		if err != nil {
			log.Println(err)
			break
		}
		destinationIPAddress := utils.ResolveDestinationIPAddressFromRawPacket(packet)
		destinationUDPAddress, ok := vpnServer.ConnMap.Get(destinationIPAddress)
		if ok {
			_, err = serverConn.WriteToUDP(packet[:n], destinationUDPAddress.(*net.UDPAddr))
			if err != nil {
				log.Println(err)
				continue
			}
			vpnServer.ConnMap.Remove(destinationIPAddress)
		}
	}
	return nil
}

func (p *PlainVPN) assignIPToTun() error {
	if !p.config.ServerMode {
		tunLink, err := netlink.LinkByName(p.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(p.config.ClientTunIP)
		if err != nil {
			return err
		}

		err = netlink.AddrAdd(tunLink, parsedTunIPAddress)
		if err != nil {
			return err
		}

		err = netlink.LinkSetUp(tunLink)
		if err != nil {
			return err
		}
	} else {
		tunLink, err := netlink.LinkByName(p.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(p.config.ServerTunIP)
		if err != nil {
			return err
		}

		err = netlink.AddrAdd(tunLink, parsedTunIPAddress)
		if err != nil {
			return err
		}

		err = netlink.LinkSetUp(tunLink)
		if err != nil {
			return err
		}
	}
	return nil
}

func (p *PlainVPN) createRoutes() error {
	// Get the TUN interface by name
	link, err := netlink.LinkByName(p.config.TunName)
	if err != nil {
		return fmt.Errorf("failed to get TUN interface %s: %w", p.config.TunName, err)
	}

	if !p.config.ServerMode {
		if p.config.Global {
			// Add default route (0.0.0.0/0) with lower metric to override existing default route
			defaultDst, _ := netlink.ParseIPNet("0.0.0.0/0")
			defaultRoute := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       defaultDst,
				Priority:  50,
			}

			if err := netlink.RouteAdd(defaultRoute); err != nil {
				return fmt.Errorf("failed to add default route with lower metric: %w", err)
			}
		} else {
			// Add route for specific destination through TUN
			dst, err := netlink.ParseIPNet(p.config.DestinationAddress)
			if err != nil {
				return fmt.Errorf("invalid destination address %s: %w", p.config.DestinationAddress, err)
			}

			route := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       dst,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("failed to add route for %s: %w", p.config.DestinationAddress, err)
			}
		}
	} else {
		// Server mode: Add route to reply back to client
		// Parse client IP without CIDR suffix
		clientIP := utils.RemoveCIDRSuffix(p.config.ClientTunIP, "/")

		// Create a /32 network for the single IP
		dst, err := netlink.ParseIPNet(clientIP + "/32")
		if err != nil {
			return fmt.Errorf("invalid client IP %s: %w", clientIP, err)
		}

		route := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       dst,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return fmt.Errorf("failed to add route for client %s: %w", clientIP, err)
		}
	}

	return nil
}
