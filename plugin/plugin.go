package main

import (
	"github.com/VolantMQ/persistence-boltdb"
	"github.com/VolantMQ/plugin"
)

type persistencePlugin struct {
	plugin.Base
}

var _ plugin.Provider = (*persistencePlugin)(nil)
var _ plugin.Info = (*persistencePlugin)(nil)

// Plugin symbol
var Plugin persistencePlugin

func init() {
	Plugin.V = "0.0.1"
	Plugin.N = "PERSISTENCE_BOLTDB_PLUGIN"
}

func (pl *persistencePlugin) Init(c interface{}) (interface{}, error) {
	return boltdb.New(c)
}

func (pl *persistencePlugin) Info() plugin.Info {
	return pl
}

func main() {
	panic("this is a plugin, build it as a plugin")
}
