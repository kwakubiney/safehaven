package config

import "github.com/kwakubiney/safehaven/wg"

type Config struct {
	ClientTunIP        string
	ClientVPNIP        string
	ServerPublicKey    string
	ClientPrivateKey   string
	ServerVPNIP        string
	ServerAddress      string
	ServerPort         string
	TunName            string
	ServerTunIP        string
	LocalAddress       string
	DestinationAddress string
	WireGuardConfig    *wg.WireGuardConfig
	Global             bool
	ServerMode         bool
}
