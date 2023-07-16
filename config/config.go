package config

type Config struct {
	ClientTunIP        string
	ServerAddress      string
	ServerPort         string
	TunName            string
	ServerTunIP        string
	LocalAddress       string
	DestinationAddress string
	//this to denote routing all traffic
	Global     bool
	ServerMode bool
}
