package main

import (
	"github.com/VolantMQ/persistence-boltdb"
	"github.com/VolantMQ/plugin"
)

type info struct {
	version string
	name    string
	desc    string
}

type persistencePlugin struct {
	info
}

var _ plugin.Provider = (*persistencePlugin)(nil)
var _ plugin.Info = (*persistencePlugin)(nil)

// Plugin symbol
var Plugin persistencePlugin

func init() {
	Plugin.version = "0.0.1"
	Plugin.name = "PERSISTENCE_BOLTDB_PLUGIN"
}

func (pl *info) Version() string {
	return pl.version
}

func (pl *info) Name() string {
	return pl.name
}

func (pl *info) Desc() string {
	return pl.desc
}

func (pl *persistencePlugin) Init(c interface{}) (interface{}, error) {
	return boltdb.New(c)
}

func (pl *persistencePlugin) Info() plugin.Info {
	return &pl.info
}
