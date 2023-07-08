package app

import (
	"log"

	"github.com/kwakubiney/safehaven/client"
	"github.com/kwakubiney/safehaven/config"
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
	err := vpnClient.SetTunOnClient()
	if err != nil {
		return err
	}
	err = app.AssignIPToTun()
	if err != nil {
		return err
	}

	packet := make([]byte, 1500)
	for {
		n, err := vpnClient.TunInterface.Read(packet)
		if err != nil {
			log.Fatal(err)
		}
		log.Printf("Packet Received On %s Interface: % x\n", app.Config.ClientTunName, packet[:n])
	}
}


func (app *App) AssignIPToTun() error {
	tunLink, err := netlink.LinkByName(app.Config.ClientTunName)
	if err != nil {
		return err
	}

	parsedTunIPAddress, err := netlink.ParseAddr(app.Config.ClientTunIP)
	if err != nil{
		return err
	}

	netlink.AddrAdd(tunLink, parsedTunIPAddress)

	return nil
}
