package registry

import (
	"github.com/werf/3p-helm/pkg/registry"
)

func (c *Config) Install() error {
	err := c.Validate()
	if err != nil {
		return err
	}

	err = HelmRegistryClient.Login(
		c.Host,
		registry.LoginOptBasicAuth(c.Username, c.Password),
		registry.LoginOptInsecure(c.Insecure),
	)

	if err != nil {
		return NewLoginError(err)
	}

	return nil
}

func (c *Config) Uninstall() error {
	err := HelmRegistryClient.Logout(c.Host)

	if err != nil {
		return NewLogoutError(err)
	}

	return nil

}
