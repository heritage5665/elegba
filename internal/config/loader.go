package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadConfig(filePath string) (*Config, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 10<<20 {
		return nil, fmt.Errorf("configuration exceeds 10 MB limit")
	}

	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse configuration: %w", err)
	}
	raw, err = interpolate(raw)
	if err != nil {
		return nil, err
	}
	data, err = yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}

	var cfg Config
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode configuration: %w", err)
	}
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)(?::-([^}]*))?\}`)

func interpolate(value any) (any, error) {
	switch value := value.(type) {
	case string:
		var interpolationError error
		result := envPattern.ReplaceAllStringFunc(value, func(match string) string {
			if interpolationError != nil {
				return match
			}
			parts := envPattern.FindStringSubmatch(match)
			if env, ok := os.LookupEnv(parts[1]); ok && env != "" {
				return env
			}
			if strings.Contains(match, ":-") {
				return parts[2]
			}
			interpolationError = fmt.Errorf("environment variable %s is not set", parts[1])
			return match
		})
		return result, interpolationError
	case map[string]any:
		for key, item := range value {
			result, err := interpolate(item)
			if err != nil {
				return nil, err
			}
			value[key] = result
		}
		return value, nil
	case map[any]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("configuration keys must be strings")
			}
			converted, err := interpolate(item)
			if err != nil {
				return nil, err
			}
			result[stringKey] = converted
		}
		return result, nil
	case []any:
		for i, item := range value {
			result, err := interpolate(item)
			if err != nil {
				return nil, err
			}
			value[i] = result
		}
		return value, nil
	default:
		return value, nil
	}
}
