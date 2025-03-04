package vpn

type VPNService interface {
	Start() error
	Stop() error
}

type VPNConfig interface {
	GetTunName() string
	IsServerMode() bool
}
