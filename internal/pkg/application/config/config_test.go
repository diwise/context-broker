package config

import (
	"bytes"
	"testing"

	"github.com/matryer/is"
)

func TestLoadConfig(t *testing.T) {
	is, config := setupConfigTest(t)

	is.Equal(len(config.Tenants), 1) // should have a single tenant
}

func TestLoadTenant(t *testing.T) {
	is, config := setupConfigTest(t)
	tenant := config.Tenants[0]

	is.Equal(tenant.ID, "default")
	is.Equal(tenant.Name, "Kommunen")
}

func TestLoadContextSource(t *testing.T) {
	is, config := setupConfigTest(t)
	tenant := config.Tenants[0]

	is.Equal(len(tenant.ContextSources), 1) // should find a single context source

	csource := tenant.ContextSources[0]
	is.Equal(csource.Endpoint, "http://lolcathost:1234")
	is.Equal(len(csource.Information), 1) // should find a single registration info
}

func TestLoadTemporalEndpoint(t *testing.T) {
	is, config := setupConfigTest(t)
	tenant := config.Tenants[0]

	is.Equal(len(tenant.ContextSources), 1) // should find a single context source

	csource := tenant.ContextSources[0]
	is.True(csource.Temporal.Enabled)
	is.Equal(csource.Temporal.Endpoint, "http://tempz:1337")
}

func TestLoadRegistrationInfo(t *testing.T) {
	is, config := setupConfigTest(t)
	csource := config.Tenants[0].ContextSources[0]
	reginfo := csource.Information[0]

	is.Equal(len(reginfo.Entities), 2) // should find two entity infos
	is.Equal(reginfo.Entities[0].Type, "Device")
	is.Equal(reginfo.Entities[1].Type, "DeviceModel")
}

func setupConfigTest(t *testing.T) (*is.I, *Config) {
	is := is.New(t)
	cfgData := bytes.NewBuffer([]byte(configFile))
	config, err := Load(cfgData)
	is.NoErr(err)

	return is, config
}

var configFile string = `
tenants:
  - id: default
    name: Kommunen
    notifications:      
      - endpoint: http://endpoint-01/v2/notify
      - endpoint: http://endpoint-02/v2/notify	
    contextSources:
    - endpoint: http://lolcathost:1234
      temporal:
        enabled: true
        endpoint: http://tempz:1337
      information:
      - entities:
        - idPattern: ^urn:ngsi-ld:Device:.+
          type: Device
        - idPattern: ^urn:ngsi-ld:DeviceModel:.+
          type: DeviceModel
`
