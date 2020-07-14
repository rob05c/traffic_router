package config

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	CZFPath      string `json:"czf_path"`
	CRConfigPath string `json:"crconfig_path"`
	CRStatesPath string `json:"crstates_path"`
	// CertDir is the directory of HTTPS Certificates.
	// Certificates must be key and cert file pairs, named fqdn.crt and fqdn.key.
	// Wildcard certificates should use a # for the wildcard (since * is not a valid filename on some operating systems). For example, '#.example.net.crt'.
	CertDir string `json:"cert_dir"`
}

func LoadConfig(path string) (Config, error) {
	fi, err := os.Open(path)
	if err != nil {
		return Config{}, errors.New("loading file: " + err.Error())
	}
	defer fi.Close()
	cfg := Config{}
	if err := json.NewDecoder(fi).Decode(&cfg); err != nil {
		return Config{}, errors.New("decoding: " + err.Error())
	}
	return cfg, nil
}

// LoadCerts loads the certificates in certDir.
// Returns a map[fqdn]cert
// TODO move to a different package?
// TODO make bad certs not a fatal error, to prevent
// TODO use tls.BuildNameToCertificate instead of the file name, to find the FQDN?
func LoadCerts(certDir string) (map[string]*tls.Certificate, error) {
	files, err := ioutil.ReadDir(certDir)
	if err != nil {
		return nil, errors.New("reading directory: " + err.Error())
	}

	keys := []string{}

	certs := map[string]*tls.Certificate{}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".key") {
			keys = append(keys, file.Name())
		}
	}

	for _, key := range keys {
		lastDot := strings.LastIndex(key, ".")
		keyPrefix := key[:lastDot]
		keyPath := filepath.Join(certDir, key)
		certPath := filepath.Join(certDir, keyPrefix+".crt")
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			fmt.Println("ERROR loading certificate '" + keyPrefix + "' - found file(s) but failed to load: " + err.Error())
			continue
		}
		fqdn := strings.Replace(keyPrefix, "#", "*", -1) // # is used for * in the file name
		certs[fqdn] = &cert
	}
	return certs, nil
}

//LoadX509KeyPair(certFile, keyFile string) (Certificate, error)
