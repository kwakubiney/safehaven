package main

import (
	"flag"
	"fmt"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/kwakubiney/safehaven/pkg/vpn/plain"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kwakubiney/safehaven/config"
	wg2 "github.com/kwakubiney/safehaven/pkg/vpn/wg"
	"github.com/kwakubiney/safehaven/wg"
)

func setupConfig() (*config.Config, error) {
	cfg := &config.Config{}

	// Basic VPN flags
	flag.StringVar(&cfg.ClientTunIP, "tc", "192.168.1.100/24", "client tun device ip")
	flag.StringVar(&cfg.ServerTunIP, "ts", "192.168.1.102/24", "server tun device ip")
	flag.StringVar(&cfg.ServerAddress, "s", "138.197.32.138:3000", "server address")
	flag.StringVar(&cfg.LocalAddress, "l", "3000", "local address")
	flag.StringVar(&cfg.TunName, "tname", "tun0", "tun interface name")
	flag.BoolVar(&cfg.Global, "g", false, "global")
	flag.StringVar(&cfg.DestinationAddress, "d", "10.108.0.2", "destination host/network address")
	flag.BoolVar(&cfg.ServerMode, "srv", false, "server mode")
	wgConfigPath := flag.String("wg", "", "Path to WireGuard configuration file (JSON)")

	flag.Parse()

	if *wgConfigPath != "" {
		wgConfig, err := wg.LoadWireGuardConfig(*wgConfigPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load wg config: %w", err)
		}
		cfg.WireGuardConfig = wgConfig
	}

	return cfg, nil
}

func main() {
	cfg, err := setupConfig()
	if err != nil {
		log.Fatal(err)
	}

	var vpnService vpn.VPNService
	if cfg.WireGuardConfig != nil {
		vpnService = wg2.NewWireGuardVPN(cfg)
	} else {
		vpnService = plain.NewPlainVPN(cfg)
	}

	// Setup graceful shutdown
	interruptChan := make(chan os.Signal, 1)
	signal.Notify(interruptChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-interruptChan
		fmt.Println("\nShutting down gracefully...")
		vpnService.Stop()
		os.Exit(0)
	}()

	if err := vpnService.Start(); err != nil {
		log.Fatal(err)
	}
}
