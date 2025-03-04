package wg

import (
	"fmt"
	"github.com/kwakubiney/safehaven/config"
	"github.com/kwakubiney/safehaven/pkg/vpn"
	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
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
	return &WireGuardVPN{
		config: config,
	}
}

func (w *WireGuardVPN) Start() error {
	tunDevice, err := tun.CreateTUN(w.config.TunName, 1500)
	if err != nil {
		return fmt.Errorf("failed to create TUN device: %w", err)
	}

	w.tunDevice = tunDevice

	err = w.assignIPToTun()
	if err != nil {
		return fmt.Errorf("failed to assign IP to TUN device: %w", err)
	}

	if w.config.ServerMode {
		err = w.setupWireGuardServer(tunDevice)
	} else {
		err = w.setupWireGuardClient(tunDevice)
	}
	if err != nil {
		tunDevice.Close()
		return fmt.Errorf("failed to setup WireGuard: %w", err)
	}
	return nil
}

func (w *WireGuardVPN) Stop() error {
	w.tunDevice.Close()
	return nil
}

func (w *WireGuardVPN) setupWireGuardServer(tunDevice tun.Device) error {

	tunnelName, err := tunDevice.Name()
	if err != nil {
		return fmt.Errorf("failed to get tunnel device name: %w", err)
	}
	logger := device.NewLogger(
		device.LogLevelVerbose,
		fmt.Sprintf("(%s) ", tunnelName),
	)

	wgDevice := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)
	w.wgDevice = wgDevice

	ipcRequest := fmt.Sprintf(`private_key=%s
listen_port=%d
`,
		w.config.WireGuardConfig.ServerPrivateKey,
		w.config.WireGuardConfig.ServerListenPort,
	)

	// Add client configuration (if available)
	if w.config.WireGuardConfig.ServerAllowedIPs != "" {
		ipcRequest += fmt.Sprintf(`
public_key=%s
allowed_ip=%s
`,
			w.config.WireGuardConfig.ServerPublicKey,  // Client's public key
			w.config.WireGuardConfig.ServerAllowedIPs, // Allowed IPs for the client
		)
	}

	if err := wgDevice.IpcSet(ipcRequest); err != nil {
		return fmt.Errorf("failed to configure WireGuard server: %w", err)
	}

	wgDevice.Up()

	return nil
}

func (w *WireGuardVPN) setupWireGuardClient(tunDevice tun.Device) error {
	tunnelName, err := tunDevice.Name()
	if err != nil {
		return fmt.Errorf("failed to get tunnel device name: %w", err)
	}

	logger := device.NewLogger(
		device.LogLevelVerbose,
		fmt.Sprintf("(%s) ", tunnelName),
	)

	wgDevice := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)
	w.wgDevice = wgDevice

	host, portStr, err := net.SplitHostPort(w.config.ServerAddress)
	if err != nil {
		return fmt.Errorf("invalid server address format: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	ipcRequest := fmt.Sprintf(`private_key=%s
		listen_port=%d
		public_key=%s
		endpoint=%s:%d
		allowed_ip=%s
		persistent_keepalive_interval=%d
`,
		w.config.WireGuardConfig.ClientPrivateKey,
		w.config.WireGuardConfig.ClientListenPort,
		w.config.WireGuardConfig.ClientPublicKey,
		host, port,
		w.config.WireGuardConfig.ClientAllowedIPs,
		w.config.WireGuardConfig.PersistentKeepalive,
	)

	if err := wgDevice.IpcSet(ipcRequest); err != nil {
		return fmt.Errorf("failed to configure WireGuard client: %w", err)
	}
	wgDevice.Up()
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
