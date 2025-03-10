package wg

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/kwakubiney/safehaven/utils"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"log"
	"net"
	"strconv"
)

type WireGuardVPN struct {
	config     *config.Config
	wgDevice   *device.Device
	tunDevice  tun.Device
	privateKey wgtypes.Key
	publicKey  wgtypes.Key
}

func NewWireGuardVPN(config *config.Config) vpn.VPNService {
	log.Println("Initializing SafeHaven WireGuard VPN service...")
	return &WireGuardVPN{
		config: config,
	}
}

func (w *WireGuardVPN) Start(ctx context.Context) error {
	log.Println("Setting up WireGuard TUN device...")
	tunDevice, err := tun.CreateTUN(w.config.TunName, 1500)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %w", err)
	}
	log.Printf("TUN device %s created successfully", w.config.TunName)

	w.tunDevice = tunDevice
	w.tunDevice.Events()

	err = w.assignIPToTun()
	if err != nil {
		return fmt.Errorf("failed to assign IP to TUN device: %w", err)
	}
	log.Printf("TUN interface IP configured: %s", w.config.ServerMode)

	err = w.createRoutes()
	if err != nil {
		return fmt.Errorf("failed to create routes: %w", err)
	}
	log.Println("Network routes configured successfully")

	if w.config.ServerMode {
		log.Println("Starting VPN in server mode...")
		err = w.setupWireGuardServer()
	} else {
		log.Println("Starting VPN in client mode...")
		err = w.setupWireGuardClient()
	}
	if err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to setup WireGuard: %w", err)
	}
	log.Println("SafeHaven VPN started successfully")
	// Wait for context cancellation to initiate shutdown
	<-ctx.Done()
	log.Println("Context cancelled, initiating WireGuard VPN shutdown...")
	return nil
}

func (w *WireGuardVPN) Stop() error {
	w.tunDevice.Close()
	return nil
}

func (w *WireGuardVPN) setupWireGuardServer() error {

	tunnelName, err := w.tunDevice.Name()
	if err != nil {
		return fmt.Errorf("failed to get tunnel device name: %w", err)
	}
	logger := device.NewLogger(
		device.LogLevelVerbose,
		fmt.Sprintf("(%s) ", "server "+tunnelName),
	)

	wgDevice := device.NewDevice(w.tunDevice, conn.NewDefaultBind(), logger)
	w.wgDevice = wgDevice

	hexEncodedClientPublicKey, hexEncodedServerPrivateKey, err :=
		convertPublicAndPrivateKeyToHex(w.config.WireGuardConfig.ClientPublicKey,
			w.config.WireGuardConfig.ServerPrivateKey)

	ipcRequest := fmt.Sprintf(`private_key=%s
listen_port=%s
public_key=%s
allowed_ip=%s
`,
		hexEncodedServerPrivateKey,
		w.config.LocalAddress,
		hexEncodedClientPublicKey,                 // Client's public key
		w.config.WireGuardConfig.ServerAllowedIPs, // Allowed IPs for the client)
	)

	logger.Verbosef("IPC Request [server]: %s", ipcRequest)

	if err := wgDevice.IpcSet(ipcRequest); err != nil {
		return fmt.Errorf("failed to configure WireGuard server: %w", err)
	}

	//get ipc
	ipc, err := wgDevice.IpcGet()
	if err != nil {
		return fmt.Errorf("failed to get ipc: %w", err)
	}
	//log ipc
	logger.Verbosef("IPC Response [server]: %s", ipc)

	err = wgDevice.Up()
	if err != nil {
		return fmt.Errorf("failed to bring up WireGuard server: %w", err)
	}
	return nil
}

func (w *WireGuardVPN) setupWireGuardClient() error {
	tunDeviceName, err := w.tunDevice.Name()
	if err != nil {
		return fmt.Errorf("failed to get tunnel device name: %w", err)
	}

	logger := device.NewLogger(
		device.LogLevelVerbose,
		fmt.Sprintf("(%s) ", "client "+tunDeviceName),
	)

	wgDevice := device.NewDevice(w.tunDevice, conn.NewDefaultBind(), logger)
	w.wgDevice = wgDevice

	host, portStr, err := net.SplitHostPort(w.config.ServerAddress)
	if err != nil {
		return fmt.Errorf("invalid server address format: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	hexEncodedServerPublicKey, hexEncodedClientPrivateKey, err :=
		convertPublicAndPrivateKeyToHex(w.config.WireGuardConfig.ServerPublicKey,
			w.config.WireGuardConfig.ClientPrivateKey)

	if err != nil {
		return fmt.Errorf("failed to convert public and private keys to hexadecimal: %w", err)
	}

	ipcRequest := fmt.Sprintf(`private_key=%s
listen_port=%s
public_key=%s
endpoint=%s:%d
allowed_ip=%s`,
		hexEncodedClientPrivateKey,
		w.config.LocalAddress,
		hexEncodedServerPublicKey,
		host, port,
		w.config.DestinationAddress,
	)

	fmt.Println("IPC Request [client]: ", ipcRequest)

	if err := wgDevice.IpcSet(ipcRequest); err != nil {
		return fmt.Errorf("failed to configure WireGuard client: %w", err)
	}

	err = wgDevice.Up()
	if err != nil {
		return fmt.Errorf("failed to bring up WireGuard client: %w", err)
	}

	//get ipc
	ipc, err := wgDevice.IpcGet()
	if err != nil {
		return fmt.Errorf("failed to get ipc: %w", err)
	}
	//log ipc
	logger.Verbosef("IPC Response [client]: %s", ipc)

	return nil
}

func (w *WireGuardVPN) assignIPToTun() error {
	if !w.config.ServerMode {
		tunLink, err := netlink.LinkByName(w.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(w.config.ClientTunIP)
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
		tunLink, err := netlink.LinkByName(w.config.TunName)
		if err != nil {
			return err
		}

		parsedTunIPAddress, err := netlink.ParseAddr(w.config.ServerTunIP)
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

func base64ToHex(base64Str string) (string, error) {
	// Decode Base64 to bytes
	data, err := base64.StdEncoding.DecodeString(base64Str)
	if err != nil {
		return "", err
	}

	// Encode bytes to hexadecimal
	hexStr := hex.EncodeToString(data)
	return hexStr, nil
}

func convertPublicAndPrivateKeyToHex(public string, private string) (string, string, error) {
	// Convert public key to hexadecimal
	publicKeyHex, err := base64ToHex(public)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert public key to hexadecimal: %w", err)
	}

	// Convert private key to hexadecimal
	privateKeyHex, err := base64ToHex(private)
	if err != nil {
		return "", "", fmt.Errorf("failed to convert private key to hexadecimal: %w", err)
	}

	return publicKeyHex, privateKeyHex, nil
}

func (w *WireGuardVPN) createRoutes() error {
	// Get the TUN interface by name
	link, err := netlink.LinkByName(w.config.TunName)
	if err != nil {
		return fmt.Errorf("failed to get TUN interface %s: %w", w.config.TunName, err)
	}

	if !w.config.ServerMode {
		if w.config.Global {
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
		} else {
			// Add route for specific destination through TUN
			dst, err := netlink.ParseIPNet(w.config.DestinationAddress)
			if err != nil {
				return fmt.Errorf("invalid destination address %s: %w", w.config.DestinationAddress, err)
			}

			route := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       dst,
			}
			if err := netlink.RouteAdd(route); err != nil {
				return fmt.Errorf("failed to add route for %s: %w", w.config.DestinationAddress, err)
			}
		}
	} else {
		// Server mode: Add route to reply back to client
		// Parse client IP without CIDR suffix
		clientIP := utils.RemoveCIDRSuffix(w.config.ClientTunIP, "/")

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
	}
	return nil
}
