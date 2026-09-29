package config

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Size is a number of bytes that the config carries in a readable form such as
// "1MB" or "500KB".
type Size int64

var units = []struct {
	suffix string
	scale  int64
}{
	{"KIB", 1 << 10},
	{"MIB", 1 << 20},
	{"GIB", 1 << 30},
	{"TIB", 1 << 40},
	{"KB", 1 << 10},
	{"MB", 1 << 20},
	{"GB", 1 << 30},
	{"TB", 1 << 40},
	{"K", 1 << 10},
	{"M", 1 << 20},
	{"G", 1 << 30},
	{"T", 1 << 40},
	{"B", 1},
}

// ParseSize reads a plain number of bytes.
func ParseSize(text string) (Size, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return 0, nil
	}

	upper := strings.ToUpper(trimmed)

	for _, unit := range units {
		digits, found := strings.CutSuffix(upper, unit.suffix)
		if !found {
			continue
		}

		return scaled(strings.TrimSpace(digits), unit.scale, text)
	}

	return scaled(upper, 1, text)
}

func scaled(digits string, scale int64, original string) (Size, error) {
	value, err := strconv.ParseFloat(digits, 64)
	if err != nil {
		return 0, fmt.Errorf("bad size %q, want something like 1MB, 500KB or 0", original)
	}
	if value < 0 {
		return 0, fmt.Errorf("bad size %q, a size cannot be negative", original)
	}

	return Size(value * float64(scale)), nil
}

func (s Size) Bytes() int64 {
	return int64(s)
}

func (s Size) String() string {
	if s == 0 {
		return "0"
	}

	for _, unit := range []struct {
		suffix string
		scale  int64
	}{
		{"TB", 1 << 40},
		{"GB", 1 << 30},
		{"MB", 1 << 20},
		{"KB", 1 << 10},
	} {
		if int64(s)%unit.scale == 0 {
			return strconv.FormatInt(int64(s)/unit.scale, 10) + unit.suffix
		}
	}

	return strconv.FormatInt(int64(s), 10) + "B"
}

func (s Size) MarshalYAML() (any, error) {
	return s.String(), nil
}

func (s *Size) UnmarshalYAML(node *yaml.Node) error {
	parsed, err := ParseSize(node.Value)
	if err != nil {
		return err
	}

	*s = parsed

	return nil
}

func (s Size) MarshalJSON() ([]byte, error) {
	return []byte(`"` + s.String() + `"`), nil
}
