package docker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNameFromImageRef(t *testing.T) {
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire:main"))
	assert.Equal(t, "once-campfire", NameFromImageRef("ghcr.io/basecamp/once-campfire"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx:latest"))
	assert.Equal(t, "nginx", NameFromImageRef("nginx"))
}

func TestBuildEnvWithSMTP(t *testing.T) {
	settings := ApplicationSettings{
		SMTP: SMTPSettings{
			Server:   "smtp.example.com",
			Port:     "587",
			Username: "user@example.com",
			Password: "secret",
		},
	}

	env := settings.BuildEnv("test-secret-key")

	assert.Contains(t, env, "SMTP_ADDRESS=smtp.example.com")
	assert.Contains(t, env, "SMTP_PORT=587")
	assert.Contains(t, env, "SMTP_USERNAME=user@example.com")
	assert.Contains(t, env, "SMTP_PASSWORD=secret")
}

func TestBuildEnvWithoutSMTP(t *testing.T) {
	settings := ApplicationSettings{}

	env := settings.BuildEnv("test-secret-key")

	for _, e := range env {
		assert.NotContains(t, e, "SMTP_")
	}
}
