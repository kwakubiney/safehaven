package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/kwakubiney/safehaven/pkg/vpn/plain"
	wg2 "github.com/kwakubiney/safehaven/pkg/vpn/wg"
	"github.com/kwakubiney/safehaven/wg"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
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

	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		sig := <-signalChan
		log.Printf("Received signal %v: initiating graceful shutdown", sig)

		cancel()

		stopDone := make(chan struct{})

		go func() {
			// Block additional signals during shutdown
			signal.Stop(signalChan)

			if err := vpnService.Stop(); err != nil {
				log.Printf("Error during shutdown: %v", err)
			}

			close(stopDone)
		}()

		select {
		case <-time.After(6 * time.Second):
			log.Printf("Shutdown timed out")
		case <-stopDone:
			log.Printf("VPN service stopped successfully")
		}

		os.Exit(0)
	}()

	// Start VPN service
	if err := vpnService.Start(ctx); err != nil {
		log.Fatalf("Failed to start VPN service: %v", err)
	}
}
