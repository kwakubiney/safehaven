package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kwakubiney/safehaven/app"
	"github.com/kwakubiney/safehaven/config"
)

func main() {
	var config config.Config
	flag.StringVar(&config.ClientTunIP, "tc", "192.168.1.100/24", "client tun device ip")
	flag.StringVar(&config.ServerTunIP, "ts", "192.168.1.102/24", "server tun device ip")
	flag.StringVar(&config.ServerAddress, "s", "138.197.32.138:3000", "server address")
	flag.StringVar(&config.LocalAddress, "l", "3000", "local address")
	flag.StringVar(&config.TunName, "tname", "tun0", "tun interface name")
	flag.BoolVar(&config.Global, "g", false, "global")
	flag.StringVar(&config.DestinationAddress, "d", "10.108.0.2", "destination host/network address")
	flag.BoolVar(&config.ServerMode, "srv", false, "server mode")

	flag.Parse()

	app := app.NewApp(&config)

	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-interruptChan
		fmt.Println("\nShutting down gracefully...")
		os.Exit(0)
	}()

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
