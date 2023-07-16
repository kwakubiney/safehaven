package main

import (
	"flag"
	"log"

	"github.com/kwakubiney/safehaven/app"
	"github.com/kwakubiney/safehaven/config"
)

func main() {
	var config config.Config
	flag.StringVar(&config.ClientTunIP, "t", "192.168.1.100/24", "client tun device ip")
	flag.StringVar(&config.ServerAddress, "s", "138.197.32.138", "server address")
	flag.StringVar(&config.LocalAddress, "l", "", "local address")
	flag.StringVar(&config.ClientTunName, "tname", "tun0", "tunname")
	flag.BoolVar(&config.Global, "g", false, "global")
	flag.StringVar(&config.DestinationAddress, "d", "10.108.0.2", "destination")
	flag.BoolVar(&config.ServerMode, "srv", false, "server mode")

	flag.Parse()

	app := app.NewApp(&config)

	if !config.ServerMode {
		err := app.StartVPNClient()
		if err != nil {
			log.Fatal(err)
		}
	} else {
		err := app.StartVPNServer()
		if err != nil {
			log.Fatal(err)
		}
	}
}
