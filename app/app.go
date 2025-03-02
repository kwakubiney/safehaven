package app

import (
	"fmt"
	"log"
	"net"
	"strconv"

	"github.com/kwakubiney/safehaven/client"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/server"
	"github.com/kwakubiney/safehaven/utils"
	"github.com/orcaman/concurrent-map/v2"
	"github.com/vishvananda/netlink"
)

type App struct {
	Config *config.Config
}

func NewApp(config *config.Config) App {
	return App{
		Config: config,
	}
}

func (app *App) StartVPNClient() error {
	vpnClient := client.NewClient(app.Config)
	err := vpnClient.SetTunOnDevice()
	if err != nil {
		return err
	}
	err = app.AssignIPToTun()
	if err != nil {
		return err
	}

	err = app.CreateRoutes()
	if err != nil {
		return err
	}

	packet := make([]byte, 65535)
	clientConn, err := net.Dial("udp", app.Config.ServerAddress)

	if err != nil {
		return err
	}
	defer clientConn.Close()
	defer vpnClient.TunInterface.Close()

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

func (app *App) StartVPNServer() error {
	vpnServer := server.NewServer(app.Config)
	err := vpnServer.SetTunOnDevice()
	if err != nil {
		return err
	}
	vpnServer.ConnMap = cmap.New[net.Addr]()
	err = app.AssignIPToTun()
	if err != nil {
		return err
	}

	err = app.CreateRoutes()
	if err != nil {
		return err
	}

	localAddress, _ := strconv.Atoi(app.Config.LocalAddress)
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

func (app *App) AssignIPToTun() error {
	if !app.Config.ServerMode {
		tunLink, err := netlink.LinkByName(app.Config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(app.Config.ClientTunIP)
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
		tunLink, err := netlink.LinkByName(app.Config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(app.Config.ServerTunIP)
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

func (app *App) CreateRoutes() error {
	// Get the TUN interface by name
	link, err := netlink.LinkByName(app.Config.TunName)
	if err != nil {
		return fmt.Errorf("failed to get TUN interface %s: %w", app.Config.TunName, err)
	}

	if !app.Config.ServerMode {
		if app.Config.Global {
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
			dst, err := netlink.ParseIPNet(app.Config.DestinationAddress)
			if err != nil {
				return fmt.Errorf("invalid destination address %s: %w", app.Config.DestinationAddress, err)
			}

			route := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       dst,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("failed to add route for %s: %w", app.Config.DestinationAddress, err)
			}
		}
	} else {
		// Server mode: Add route to reply back to client
		// Parse client IP without CIDR suffix
		clientIP := utils.RemoveCIDRSuffix(app.Config.ClientTunIP, "/")

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
