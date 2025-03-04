package wg

import (
	"encoding/base64"
	"encoding/hex"
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
		fmt.Sprintf("(%s) ", "server "+tunnelName),
	)

	wgDevice := device.NewDevice(tunDevice, conn.NewDefaultBind(), logger)
	w.wgDevice = wgDevice

	hexEncodedClientPublicKey, hexEncodedServerPrivateKey, err :=
		convertPublicAndPrivateKeyToHex(w.config.WireGuardConfig.ClientPublicKey,
			w.config.WireGuardConfig.ServerPrivateKey)

	ipcRequest := fmt.Sprintf(`
private_key=%s
listen_port=%s
`,
		hexEncodedServerPrivateKey,
		w.config.LocalAddress,
	)

	// Add client configuration (if available)
	if w.config.WireGuardConfig.ServerAllowedIPs != "" {
		ipcRequest += fmt.Sprintf(`
public_key=%s
allowed_ip=%s
endpoint=%s
`,
			hexEncodedClientPublicKey,                 // Client's public key
			w.config.WireGuardConfig.ServerAllowedIPs, // Allowed IPs for the client
			w.config.ClientTunIP,
		)
	}

	fmt.Println("IPC Request [server]: ", ipcRequest)

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
		fmt.Sprintf("(%s) ", "client "+tunnelName),
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
		allowed_ip=%s
`,
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
