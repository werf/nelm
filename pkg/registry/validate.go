package registry

func (c *Config) Validate() error {
	if c.Host == "" {
		return ErrNameEmpty
	}

	return nil
}
