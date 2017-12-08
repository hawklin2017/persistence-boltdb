package main

import (
	"github.com/VolantMQ/persistence-boltdb"
	vlPlugin "github.com/VolantMQ/plugin"
)

type persistencePlugin struct {
	vlPlugin.Base
}

var _ vlPlugin.Provider = (*persistencePlugin)(nil)
var _ vlPlugin.Info = (*persistencePlugin)(nil)

// Plugin symbol
var Plugin persistencePlugin

func init() {
	Plugin.V = "0.0.1"
	Plugin.N = "PERSISTENCE_BOLTDB_PLUGIN"
}

func (pl *persistencePlugin) Init(c interface{}) (interface{}, error) {
	return boltdb.New(c)
}

func (pl *persistencePlugin) Info() vlPlugin.Info {
	return pl
}

func main() {
	panic("this is a plugin, build it as a plugin")
}
