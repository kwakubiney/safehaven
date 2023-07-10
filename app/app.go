package app

import (
	"log"
	"os/exec"

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

	/*
		I use 0.0.0.0/1 and 128.0.0.0/1 as destination addresses specifically because I want to
		override the default route without modifying or removing the existing one
		See: https://serverfault.com/questions/1100250/what-is-the-difference-between-0-0-0-0-0-and-0-0-0-0-1
		& answer https://serverfault.com/a/1100354
	*/

	//TODO: Figure out how to use netlink to achieve this
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

	return nil
}
