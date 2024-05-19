package app

import (
	"log"
	"net"
	"os/exec"
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

/*
NB: The global path is supposed to route ALL traffic to TUN

	I use 0.0.0.0/1 and 128.0.0.0/1 as destination addresses specifically because I want to
	override the default route without modifying or removing the existing one
	See: https://serverfault.com/questions/1100250/what-is-the-difference-between-0-0-0-0-0-and-0-0-0-0-1
	& answer https://serverfault.com/a/1100354
*/
func (app *App) CreateRoutes() error {
	if !app.Config.ServerMode {
		if app.Config.Global {
			routeFirstHalfOfAllDestToTun := exec.Command("sudo", "ip", "route", "add", "0.0.0.0/1", "dev", app.Config.TunName)
			_, err := routeFirstHalfOfAllDestToTun.Output()
			if err != nil {
				return err
			}

			routeSecondHalfOfAllDestToTun := exec.Command("sudo", "ip", "route", "add", "128.0.0.0/1", "dev", app.Config.TunName)
			_, err = routeSecondHalfOfAllDestToTun.Output()
			if err != nil {
				return err
			}
		} else {
			routeTrafficToDestinationThroughTun := exec.Command("sudo", "ip", "route", "add", app.Config.DestinationAddress, "dev", app.Config.TunName)
			_, err := routeTrafficToDestinationThroughTun.Output()
			if err != nil {
				return err
			}
		}
	} else {
		routeReplyBackToClient := exec.Command("sudo", "ip", "route", "add",
			utils.RemoveCIDRSuffix(app.Config.ClientTunIP, "/"),
			"dev", app.Config.TunName)
		_, err := routeReplyBackToClient.Output()
		if err != nil {
			return err
		}
	}
	return nil
}
