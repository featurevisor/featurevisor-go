package instance

import "github.com/featurevisor/featurevisor-go/config"

type Factory struct {
	DatafileURL string

	logger interface{}
}

// Instance creates a new Featurevisor instance with passed datafile URL.
func (factory *Factory) NewInstance() (*Instance, error) {
	configManager := config.NewStaticConfigManager(factory.DatafileURL)

	instance := &Instance{
		ConfigManager: configManager,
	}
	return instance, nil
}
