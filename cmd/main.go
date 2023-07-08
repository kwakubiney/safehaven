package main

import (
	"flag"
	"log"

	"github.com/kwakubiney/safehaven/app"
	"github.com/kwakubiney/safehaven/config"
)

func main() {
	var config config.Config
	flag.StringVar(&config.ClientTunIP, "tunaddr", "192.168.1.100/24", "client tun device ip")
	flag.StringVar(&config.ServerAddress, "saddr", "", "server address")
	flag.StringVar(&config.LocalAddress, "laddr", "", "local address")
	flag.StringVar(&config.ServerPort, "sport", "", "server port")
	flag.StringVar(&config.ClientTunName, "tun", "tun0", "tunname")
	flag.Parse()

	app := app.NewApp(&config)

	err := app.StartVPNClient()
	if err != nil {
		log.Fatal(err)
	}
}
