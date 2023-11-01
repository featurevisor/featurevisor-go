package config

// Config represents a parsed datafile config.
type Config interface {
	GetDatafile() []byte
	GetRevision() string
	// ...rest of the methods
}

// ConfigManager represents a configuration manager that reads and holds datafile config.
type ConfigManager interface {
	GetConfig() (Config, error)
	Sync() error
}
