package config

import (
	"time"

	"gopkg.in/yaml.v3"
)

// Duration wraps time.Duration so that YAML carries a human-readable string
// such as "10s" or "750ms".
type Duration time.Duration

func (d Duration) Unwrap() time.Duration {
	return time.Duration(d)
}

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d Duration) MarshalYAML() (any, error) {
	return time.Duration(d).String(), nil
}

func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	v, err := time.ParseDuration(node.Value)
	if err != nil {
		return err
	}

	*d = Duration(v)

	return nil
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Duration(d).String() + `"`), nil
}
