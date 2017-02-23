package main

// Birdwatcher Configuration

import (
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/imdario/mergo"

	"github.com/ecix/birdwatcher/bird"
	"github.com/ecix/birdwatcher/endpoints"
)

type Config struct {
	Server endpoints.ServerConfig

	Ratelimit bird.RateLimitConfig
	Status    bird.StatusConfig
	Bird      bird.BirdConfig
	Bird6     bird.BirdConfig
}

// Try to load configfiles as specified in the files
// list. For example:
//
//    ./etc/birdwatcher/birdwatcher.conf
//    /etc/birdwatcher/birdwatcher.conf
//    ./etc/birdwatcher/birdwatcher.local.conf
//
//
func LoadConfigs(configFiles []string) (*Config, error) {
	config := &Config{}
	hasConfig := false
	var confError error

	for _, filename := range configFiles {
		tmp := &Config{}
		_, err := toml.DecodeFile(filename, tmp)
		if err != nil {
			continue
		} else {
			hasConfig = true
			// Merge configs
			if err := mergo.Merge(config, tmp); err != nil {
				return nil, err
			}
		}
	}

	if !hasConfig {
		confError = fmt.Errorf("Could not load any config file")
	}

	return config, confError
}

func ConfigOptions(filename string) []string {
	return []string{
		strings.Join([]string{"/", filename}, ""),
		strings.Join([]string{"./", filename}, ""),
		strings.Replace(filename, ".conf", ".local.conf", 1),
	}
}
