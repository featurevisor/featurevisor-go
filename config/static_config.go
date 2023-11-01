package config

import (
	"fmt"
	"io"
	"net/http"
	"sync"
)

type StaticConfigManager struct {
	datafileURL string
	datafile    []byte
	client      *http.Client
	config      Config
	lock        sync.Mutex
	logger      interface{}
}

func (configManager *StaticConfigManager) GetConfig() (Config, error) {
	configManager.lock.Lock()
	defer configManager.lock.Unlock()

	return configManager.config, nil
}

func (configManager *StaticConfigManager) Sync() error {
	configManager.lock.Lock()
	defer configManager.lock.Unlock()

	url := configManager.datafileURL
	datafile, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("error while loading datafile: %w", err)
	}

	// Read the body of the response into a byte array
	defer datafile.Body.Close()
	body, err := io.ReadAll(datafile.Body)
	if err != nil {
		return fmt.Errorf("error while reading datafile: %w", err)
	}

	configManager.datafile = body
	return nil
}

func NewStaticConfigManager(datafileURL string) *StaticConfigManager {
	return &StaticConfigManager{
		datafileURL: datafileURL,
		client:      http.DefaultClient,
	}
}
