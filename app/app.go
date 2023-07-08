package app

import (
	"log"
	"os/exec"

	"github.com/kwakubiney/safehaven/client"
	"github.com/kwakubiney/safehaven/config"
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
	log.Printf("ip address add %s dev %s", app.Config.ClientTunIP, app.Config.ClientTunName)
	assignIPtoTunCommand := exec.Command("ip", "address", "add", app.Config.ClientTunIP, "dev", app.Config.ClientTunName)
	ipToTunCommandResp, err := assignIPtoTunCommand.Output()
	if err != nil {
		return err
	}
	log.Println(ipToTunCommandResp)

	log.Printf("ip link set dev %s up", app.Config.ClientTunName)
	tunUpCommand := exec.Command("ip", "link", "set", "dev", app.Config.ClientTunName, "up")
	tunUpCommandResp, err := tunUpCommand.Output()
	if err != nil {
		return err
	}
	log.Println((tunUpCommandResp))
	return nil
}
