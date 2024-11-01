package config

import (
	"errors"
	"fmt"
	"github.com/cirruslabs/orchard/internal/netconstants"
	"github.com/cirruslabs/orchard/internal/orchardhome"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
)

const configName = "orchard.yml"

type Handle struct {
	configPath string
}

func NewHandle() (*Handle, error) {
	orchardHomeDir, err := orchardhome.Path()
	if err != nil {
		return nil, err
	}

	return &Handle{
		configPath: filepath.Join(orchardHomeDir, configName),
	}, nil
}

func (handle *Handle) Config() (*Config, error) {
	config := Config{
		Contexts: map[string]Context{},
	}

	configBytes, err := os.ReadFile(handle.configPath)
	if err != nil {
		// Handle a case where the config file is not created yet
		if errors.Is(err, os.ErrNotExist) {
			return &Config{
				Contexts: map[string]Context{},
			}, nil
		}

		return nil, fmt.Errorf("%w: %v", ErrConfigReadFailed, err)
	}

	if err := yaml.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("%w: invalid YAML: %v", ErrConfigReadFailed, err)
	}

	return &config, nil
}

func (handle *Handle) SetConfig(config *Config) error {
	configBytes, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("%w: failed to marshal YAML: %v", ErrConfigWriteFailed, err)
	}

	if err := os.WriteFile(handle.configPath, configBytes, 0600); err != nil {
		return fmt.Errorf("%w: %v", ErrConfigWriteFailed, err)
	}

	return nil
}

func (handle *Handle) CreateContext(name string, context Context, force bool) error {
	unlock, err := handle.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	config, err := handle.Config()
	if err != nil {
		return err
	}

	_, exists := config.RetrieveContext(name)
	if exists && !force {
		return fmt.Errorf("%w: context %q already exists", ErrConfigConflict, name)
	}

	config.SetContext(name, context)

	return handle.SetConfig(config)
}

func (handle *Handle) DefaultContext() (Context, error) {
	unlock, err := handle.Lock()
	if err != nil {
		return Context{}, err
	}
	defer unlock()

	config, err := handle.Config()
	if err != nil {
		return Context{}, err
	}

	defaultContext, ok := config.RetrieveDefaultContext()
	if !ok {
		defaultContext = Context{
			URL: fmt.Sprintf("http://127.0.0.1:%d", netconstants.DefaultControllerPort),
		}

		config.SetContext("default", defaultContext)

		if err := handle.SetConfig(config); err != nil {
			return Context{}, err
		}
	}

	// Environment variable overrides
	if url, ok := os.LookupEnv(OrchardURL); ok {
		defaultContext.URL = url
	}
	if serviceAccountName, ok := os.LookupEnv(OrchardServiceAccountName); ok {
		defaultContext.ServiceAccountName = serviceAccountName
	}
	if serviceAccountToken, ok := os.LookupEnv(OrchardServiceAccountToken); ok {
		defaultContext.ServiceAccountToken = serviceAccountToken
	}

	return defaultContext, nil
}

func (handle *Handle) SetDefaultContext(name string) error {
	unlock, err := handle.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	config, err := handle.Config()
	if err != nil {
		return err
	}

	_, ok := config.RetrieveContext(name)
	if !ok {
		return fmt.Errorf("%w: no such context: %q", ErrConfigConflict, name)
	}

	config.DefaultContext = name

	return handle.SetConfig(config)
}

func (handle *Handle) DeleteContext(name string) error {
	unlock, err := handle.Lock()
	if err != nil {
		return err
	}
	defer unlock()

	config, err := handle.Config()
	if err != nil {
		return err
	}

	if err := config.DeleteContext(name); err != nil {
		return err
	}

	return handle.SetConfig(config)
}
