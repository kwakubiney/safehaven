package app

import (
	"github.com/kwakubiney/safehaven/client"
	"github.com/kwakubiney/safehaven/config"
)

type App struct{
	Config *config.Config
}

func NewApp(config *config.Config) App{
	return App{
		Config: config,
	}
}

//Run this function on application startup
func (app *App) StartVPN() error{
	vpnClient := client.NewClient(app.Config)
	err := vpnClient.SetTunOnClient()
	if err != nil{
		return err
	}
	return nil	
}