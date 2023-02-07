package config

import (
	"encoding/base64"
	"gopkg.in/yaml.v3"
)

type Base64 []byte

func (b64 Base64) MarshalYAML() (interface{}, error) {
	return base64.StdEncoding.EncodeToString(b64), nil
}

func (b64 *Base64) UnmarshalYAML(value *yaml.Node) error {
	result, err := base64.StdEncoding.DecodeString(value.Value)
	if err != nil {
		return err
	}

	*b64 = result

	return nil
}
