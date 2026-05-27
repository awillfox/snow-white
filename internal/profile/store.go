package profile

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".snow_white", "profiles.yaml"), nil
}

func Load() ([]Profile, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []Profile{}, nil
	}
	if err != nil {
		return nil, err
	}
	var profiles []Profile
	if err := yaml.Unmarshal(data, &profiles); err != nil {
		return []Profile{}, nil // corrupt file → treat as empty
	}
	return profiles, nil
}

func Save(profiles []Profile) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := yaml.Marshal(profiles)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
