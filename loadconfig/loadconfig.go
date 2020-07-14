// package loadconfig contains functions for loading a config file and parsing it fully into a *shared.Shared.
// This package exists for multiple use, in the main function as well as the srvsighupreload SIGHUP reloader.
package loadconfig

import (
	"errors"

	"github.com/rob05c/traffic_router/config"
	"github.com/rob05c/traffic_router/crconfig"
	"github.com/rob05c/traffic_router/czf"
	"github.com/rob05c/traffic_router/shared"
)

func LoadConfig(path string) (*shared.Shared, error) {
	cfg, err := config.LoadConfig(path)
	if err != nil {
		return nil, errors.New("loading config file '" + path + "': " + err.Error())
	}

	czfRaw, err := czf.LoadCZF(cfg.CZFPath)
	if err != nil {
		return nil, errors.New("loading czf file '" + cfg.CZFPath + "': " + err.Error())
	}

	crc, err := crconfig.LoadCRConfig(cfg.CRConfigPath)
	if err != nil {
		return nil, errors.New("loading CRConfig file '" + cfg.CRConfigPath + "': " + err.Error())
	}

	crs, err := crconfig.LoadCRStates(cfg.CRStatesPath)
	if err != nil {
		return nil, errors.New("loading CRStates file '" + cfg.CRStatesPath + "': " + err.Error())
	}

	// fmt.Printf("DEBUG crc.config '%v': %+v\n", cfg.CRConfigPath, crc.Config)

	czfParsedNets, err := czf.ParseCZNets(czfRaw.CoverageZones)
	if err != nil {
		return nil, errors.New("parsing czf networks '" + cfg.CZFPath + "': " + err.Error())
	}

	parsedCZF := &czf.ParsedCZF{Revision: czfRaw.Revision, CustomerName: czfRaw.CustomerName, CoverageZones: czfParsedNets}

	certs, err := config.LoadCerts(cfg.CertDir)
	if err != nil {
		return nil, errors.New("loading certificates from dir '" + cfg.CertDir + "': " + err.Error())
	}

	sharedPtr := shared.NewShared(parsedCZF, crc, crs, certs)
	if sharedPtr == nil {
		return nil, errors.New("fatal error creating Shared object, see log for details.")
	}
	return sharedPtr, nil
}
