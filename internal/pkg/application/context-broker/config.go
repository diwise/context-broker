package contextbroker

import (
	"io"

	yaml "gopkg.in/yaml.v2"
)

type EntityInfo struct {
	IDPattern string `yaml:"idPattern"`
	Type      string `yaml:"type"`
}

type RegistrationInfo struct {
	Entities []EntityInfo `yaml:"entities"`
}

type ContextSourceConfig struct {
	Endpoint    string             `yaml:"endpoint"`
	Information []RegistrationInfo `yaml:"information"`
}

type Tenant struct {
	ID             string                `yaml:"id"`
	Name           string                `yaml:"name"`
	ContextSources []ContextSourceConfig `yaml:"contextSources"`
}

type Config struct {
	Tenants []Tenant `yaml:"tenants"`
}

func LoadConfiguration(data io.Reader) (*Config, error) {

	buf, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}

	cfg := &Config{}
	err = yaml.Unmarshal(buf, &cfg)

	return cfg, err
}
