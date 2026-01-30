package docker

import "github.com/docker/docker/api/types/container"

const ContainerLogMaxSize = "10m"

func ContainerLogConfig() container.LogConfig {
	return container.LogConfig{
		Type: "json-file",
		Config: map[string]string{
			"max-size": ContainerLogMaxSize,
			"max-file": "1",
		},
	}
}
