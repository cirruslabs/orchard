package config

import (
	"fmt"
)

type Config struct {
	DefaultContext string             `yaml:"default-context,omitempty"`
	Contexts       map[string]Context `yaml:"contexts,omitempty"`
}

func (config *Config) SetContext(name string, context Context) {
	config.Contexts[name] = context

	if config.DefaultContext == "" {
		config.DefaultContext = name
	}
}

func (config *Config) RetrieveContext(name string) (Context, bool) {
	context, ok := config.Contexts[name]

	return context, ok
}

func (config *Config) RetrieveDefaultContext() (Context, bool) {
	if config.DefaultContext == "" {
		return Context{}, false
	}

	return config.RetrieveContext(config.DefaultContext)
}

func (config *Config) DeleteContext(name string) error {
	_, exists := config.Contexts[name]
	if !exists {
		return fmt.Errorf("%w: no such context: %q", ErrConfigConflict, name)
	}

	delete(config.Contexts, name)

	if config.DefaultContext == name {
		config.DefaultContext = ""

		for name := range config.Contexts {
			config.DefaultContext = name
			break
		}
	}

	return nil
}
