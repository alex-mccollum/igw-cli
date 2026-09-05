package testgateway

import "github.com/alex-mccollum/igw-cli/internal/moduleprofile"

func ProfileConfig(image, docker, name string) (Config, error) {
	profile, err := moduleprofile.Select(name)
	if err != nil {
		return Config{}, err
	}
	return Config{Image: image, Docker: docker, Modules: profile.EnabledModules}, nil
}

// ValidateModuleProfile checks the stable inventory before acceptance tests
// provision credentials or exercise mutations in their disposable Gateway.
func (s *Session) ValidateModuleProfile() error {
	profile, err := moduleprofile.FromWhitelist(s.Modules)
	if err != nil {
		return err
	}
	return profile.ValidateInventory(s.ModuleInventory)
}
