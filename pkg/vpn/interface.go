package vpn

import "context"

type VPNService interface {
	Start(ctx context.Context) error
	Stop() error
}
