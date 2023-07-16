package app

import (
	"log"
	"net"
	"os/exec"

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

	packet := make([]byte, 1500)
	clientConn, err := net.Dial("udp", app.Config.ServerAddress+":3000")

	if err != nil {
		return err
	}
	defer clientConn.Close()
	for {
		n, err := vpnClient.TunInterface.Read(packet)
		log.Println("from tun:", packet)
		if err != nil {
			log.Println(err)
			break
		}

		n, err = clientConn.Write(packet[:n])
		log.Println("written to udp:", n)
		if err != nil {
			log.Println(err)
			continue
		}
	}
	go func() {
		for {
			packet := make([]byte, 1500)
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

	return nil
}

func (app *App) AssignIPToTun() error {
	tunLink, err := netlink.LinkByName(app.Config.ClientTunName)
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

	/*	NB: The global path is supposed to route ALL traffic to TUN

		I use 0.0.0.0/1 and 128.0.0.0/1 as destination addresses specifically because I want to
		override the default route without modifying or removing the existing one
		See: https://serverfault.com/questions/1100250/what-is-the-difference-between-0-0-0-0-0-and-0-0-0-0-1
		& answer https://serverfault.com/a/1100354
	*/
	if !app.Config.ServerMode {
		if app.Config.Global {
			routeFirstHalfOfAllDestToTun := exec.Command("ip", "route", "add", "0.0.0.0/1", "dev", app.Config.ClientTunName)
			_, err = routeFirstHalfOfAllDestToTun.Output()
			if err != nil {
				return err
			}

			routeSecondHalfOfAllDestToTun := exec.Command("ip", "route", "add", "128.0.0.0/1", "dev", app.Config.ClientTunName)
			_, err = routeSecondHalfOfAllDestToTun.Output()
			if err != nil {
				return err
			}
		} else {
			routeTrafficToDestinationThroughTun := exec.Command("ip", "route", "add", app.Config.DestinationAddress, "dev", app.Config.ClientTunName)
			_, err = routeTrafficToDestinationThroughTun.Output()
			if err != nil {
				return err
			}
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

	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 3000})
	if err != nil {
		return err
	}
	vpnServer.UDPConn = serverConn

	defer serverConn.Close()
	go func() {
		for {
			packet := make([]byte, 1500)

			n, clientAddr, err := serverConn.ReadFrom(packet)
			log.Println("clientAddr:", clientAddr)
			if err != nil {
				log.Println(err)
				continue
			}
			log.Println("packet from udp client:", packet[:n])
			sourceIPAddress := utils.ResolveSourceIPAddressFromRawPacket(packet)
			log.Println("sourceIPAddress:", sourceIPAddress)
			vpnServer.ConnMap.Set(sourceIPAddress, clientAddr)
			log.Println("conn map:", vpnServer.ConnMap)
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
		log.Println("connection cache:", vpnServer.ConnMap)
		destinationUDPAddress, ok := vpnServer.ConnMap.Get(destinationIPAddress)
		log.Println("sending back to client at address:", destinationIPAddress, destinationUDPAddress)
		if ok{
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
