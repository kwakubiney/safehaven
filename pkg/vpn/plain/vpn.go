package plain

import (
	"context"
	"fmt"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/kwakubiney/safehaven/utils"
	cmap "github.com/orcaman/concurrent-map/v2"
	"github.com/songgao/water"
	"github.com/vishvananda/netlink"
	"log"
	"net"
	"strconv"
	"sync"
)

type PlainVPN struct {
	config    *config.Config
	tunDevice *water.Interface
	conn      net.Conn
	connMap   cmap.ConcurrentMap[string, net.Addr]
	wg        *sync.WaitGroup
}

func NewPlainVPN(config *config.Config) vpn.VPNService {
	log.Println("Initializing SafeHaven VPN service...")
	var wg = &sync.WaitGroup{}
	return &PlainVPN{
		config: config,
		wg:     wg,
	}
}

func (p *PlainVPN) Start(ctx context.Context) error {
	if p.config.ServerMode {
		log.Println("Starting VPN in server mode...")
		return p.startServer(ctx)
	}
	log.Println("Starting VPN in client mode...")
	return p.startClient(ctx)
}

func (p *PlainVPN) Stop() error {
	p.tunDevice.Close()
	p.conn.Close()
	log.Println("VPN service shutdown complete")
	return nil
}

func (p *PlainVPN) startClient(ctx context.Context) error {
	log.Println("Setting up TUN interface...")
	err := p.setTunOnDevice()
	if err != nil {
		return err
	}
	log.Printf("TUN interface %s created successfully", p.config.TunName)

	log.Println("Configuring TUN IP address...")
	err = p.assignIPToTun()
	if err != nil {
		return err
	}
	log.Printf("TUN interface IP configured: %s", p.config.ClientTunIP)

	log.Println("Setting up network routes...")
	err = p.createRoutes()
	if err != nil {
		return err
	}
	if p.config.Global {
		log.Println("Global routing enabled - all traffic will go through VPN")
	} else {
		log.Printf("Route to %s configured through VPN", p.config.DestinationAddress)
	}

	packet := make([]byte, 65535)
	log.Printf("Connecting to VPN server at %s...", p.config.ServerAddress)
	clientConn, err := net.Dial("udp", p.config.ServerAddress)
	p.conn = clientConn

	if err != nil {
		return err
	}
	log.Println("Connected to VPN server successfully")

	p.wg.Add(1)
	//receive
	go func() {
		defer p.wg.Done()
		log.Println("Started receive handler")
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Exiting loop...")
				return
			default:
				packet := make([]byte, 65535)
				n, err := clientConn.Read(packet)
				if err != nil {
					log.Printf("Error receiving data: %v", err)
					continue
				}
				_, err = p.tunDevice.Write(packet[:n])
				if err != nil {
					log.Printf("Error writing to TUN: %v", err)
					continue
				}
			}
		}
	}()

	//send
	p.wg.Add(1)
	log.Println("Started send handler")
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Exiting loop...")
				return
			default:
				n, err := p.tunDevice.Read(packet)
				if err != nil {
					log.Printf("Error reading from TUN: %v", err)
					break
				}

				_, err = clientConn.Write(packet[:n])
				if err != nil {
					log.Printf("Error sending data: %v", err)
					continue
				}
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("VPN client shutting down...")
	return nil
}

func (p *PlainVPN) startServer(ctx context.Context) error {
	log.Println("Setting up TUN interface...")
	err := p.setTunOnDevice()
	if err != nil {
		return err
	}
	log.Printf("TUN interface %s created successfully", p.config.TunName)

	p.connMap = cmap.New[net.Addr]()

	log.Println("Configuring TUN IP address...")
	err = p.assignIPToTun()
	if err != nil {
		return err
	}
	log.Printf("TUN interface IP configured: %s", p.config.ServerTunIP)

	log.Println("Setting up network routes...")
	err = p.createRoutes()
	if err != nil {
		return err
	}
	log.Printf("Route to client (%s) configured", utils.RemoveCIDRSuffix(p.config.ClientTunIP, "/"))

	localAddress, _ := strconv.Atoi(p.config.LocalAddress)
	log.Printf("Starting UDP server on port %d...", localAddress)
	serverConn, err := net.ListenUDP("udp", &net.UDPAddr{Port: localAddress})
	p.conn = serverConn
	if err != nil {
		return err
	}
	log.Printf("UDP server listening on port %d", localAddress)
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		log.Println("Started client receive handler")
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Exiting loop...")
				return
			default:
				packet := make([]byte, 65535)
				n, clientAddr, err := serverConn.ReadFrom(packet)
				if err != nil {
					log.Printf("Error receiving from client: %v", err)
					continue
				}
				sourceIPAddress := utils.ResolveSourceIPAddressFromRawPacket(packet)

				p.connMap.Set(sourceIPAddress, clientAddr)

				_, err = p.tunDevice.Write(packet[:n])
				if err != nil {
					log.Printf("Error writing to TUN: %v", err)
					continue
				}
			}
		}
	}()

	log.Println("Started client send handler")
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for {
			select {
			case <-ctx.Done():
				fmt.Println("Exiting loop...")
				return
			default:
				packet := make([]byte, 1500)
				n, err := p.tunDevice.Read(packet)
				if err != nil {
					log.Printf("Error reading from TUN: %v", err)
					break
				}

				destinationIPAddress := utils.ResolveDestinationIPAddressFromRawPacket(packet)
				destinationUDPAddress, ok := p.connMap.Get(destinationIPAddress)
				if ok {
					_, err = serverConn.WriteToUDP(packet[:n], destinationUDPAddress.(*net.UDPAddr))
					if err != nil {
						log.Printf("Error sending to client %s: %v", destinationUDPAddress.String(), err)
						continue
					}

					p.connMap.Remove(destinationIPAddress)
				}
			}
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("VPN server shutting down...")
	return nil
}

func (p *PlainVPN) assignIPToTun() error {
	if !p.config.ServerMode {
		tunLink, err := netlink.LinkByName(p.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(p.config.ClientTunIP)
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
		log.Printf("TUN interface %s is up with IP %s", p.config.TunName, p.config.ClientTunIP)
	} else {
		tunLink, err := netlink.LinkByName(p.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(p.config.ServerTunIP)
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
		log.Printf("TUN interface %s is up with IP %s", p.config.TunName, p.config.ServerTunIP)
	}
	return nil
}

func (p *PlainVPN) createRoutes() error {
	// Get the TUN interface by name
	link, err := netlink.LinkByName(p.config.TunName)
	if err != nil {
		return fmt.Errorf("failed to get TUN interface %s: %w", p.config.TunName, err)
	}

	if !p.config.ServerMode {
		if p.config.Global {
			// Add default route (0.0.0.0/0) with lower metric to override existing default route
			defaultDst, _ := netlink.ParseIPNet("0.0.0.0/0")
			defaultRoute := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       defaultDst,
				Priority:  50,
			}

			if err := netlink.RouteAdd(defaultRoute); err != nil {
				return fmt.Errorf("failed to add default route with lower metric: %w", err)
			}
			log.Println("Added global route - all traffic will go through the VPN")
		} else {
			// Add route for specific destination through TUN
			dst, err := netlink.ParseIPNet(p.config.DestinationAddress)
			if err != nil {
				return fmt.Errorf("invalid destination address %s: %w", p.config.DestinationAddress, err)
			}

			route := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       dst,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("failed to add route for %s: %w", p.config.DestinationAddress, err)
			}
			log.Printf("Added route for %s through the VPN", p.config.DestinationAddress)
		}
	} else {
		// Server mode: Add route to reply back to client
		// Parse client IP without CIDR suffix
		clientIP := utils.RemoveCIDRSuffix(p.config.ClientTunIP, "/")

		// Create a /32 network for the single IP
		dst, err := netlink.ParseIPNet(clientIP + "/32")
		if err != nil {
			return fmt.Errorf("invalid client IP %s: %w", clientIP, err)
		}

		route := &netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       dst,
		}
		if err := netlink.RouteAdd(route); err != nil {
			return fmt.Errorf("failed to add route for client %s: %w", clientIP, err)
		}
		log.Printf("Added route for client %s", clientIP)
	}

	return nil
}

func (p *PlainVPN) setTunOnDevice() error {
	log.Printf("Creating TUN interface %s...", p.config.TunName)
	ifce, err := water.New(water.Config{DeviceType: water.TUN,
		PlatformSpecificParams: water.PlatformSpecificParams{Name: p.config.TunName},
	})
	if err != nil {
		return err
	}
	p.tunDevice = ifce
	return nil
}
