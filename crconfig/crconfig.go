package crconfig

import (
	"encoding/json"
	"errors"
	"github.com/apache/trafficcontrol/lib/go-tc"
	"os"
)

func LoadCRConfig(path string) (*tc.CRConfig, error) {
	fi, err := os.Open(path)
	if err != nil {
		return nil, errors.New("loading file: " + err.Error())
	}
	defer fi.Close()
	obj := tc.CRConfig{}
	if err := json.NewDecoder(fi).Decode(&obj); err != nil {
		return nil, errors.New("decoding: " + err.Error())
	}
	return &obj, nil
}

func LoadCRStates(path string) (*tc.CRStates, error) {
	// TODO put in its own file? Make abstract "JSON File Loader" taking interface{}?
	fi, err := os.Open(path)
	if err != nil {
		return nil, errors.New("loading file: " + err.Error())
	}
	defer fi.Close()
	obj := tc.CRStates{}
	if err := json.NewDecoder(fi).Decode(&obj); err != nil {
		return nil, errors.New("decoding: " + err.Error())
	}
	return &obj, nil
}
