package wg

import (
	"encoding/json"
	"fmt"
	"os"
)

// WireGuardConfig represents the WireGuard configuration
type WireGuardConfig struct {
	ClientPrivateKey string `json:"client_private_key"`
	ClientPublicKey  string `json:"client_public_key"`
	ServerPrivateKey string `json:"server_private_key"`
	ServerPublicKey  string `json:"server_public_key"`
	ServerAllowedIPs string `json:"server_allowed_ips"`
}

// loadWireGuardConfig loads the WireGuard configuration from a JSON file
func LoadWireGuardConfig(filepath string) (*WireGuardConfig, error) {
	file, err := os.ReadFile(filepath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config WireGuardConfig
	if err := json.Unmarshal(file, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}
	return &config, nil
}
