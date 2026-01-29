package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/basecamp/amar/internal/docker"
)

func TestSettingsForm_InitialState_NonLocalhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "app.example.com",
		DisableTLS: false,
	}
	form := NewSettingsForm(settings)

	assert.Equal(t, settingsFieldImage, form.focused)
	assert.Equal(t, "nginx:latest", form.imageInput.Value())
	assert.Equal(t, "app.example.com", form.hostnameInput.Value())
	assert.True(t, form.settings.TLSEnabled())
}

func TestSettingsForm_InitialState_Localhost(t *testing.T) {
	settings := docker.ApplicationSettings{
		Image:      "nginx:latest",
		Host:       "chat.localhost",
		DisableTLS: false,
	}
	form := NewSettingsForm(settings)

	assert.Equal(t, "chat.localhost", form.hostnameInput.Value())
	assert.False(t, form.settings.TLSEnabled(), "TLS should be disabled for localhost even when DisableTLS is false")
}

func TestSettingsForm_TabNavigation(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.Equal(t, settingsFieldImage, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldHostname, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSMTPServer, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSMTPPort, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSMTPUsername, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSMTPPassword, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSMTPFrom, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldSaveButton, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldCancelButton, form.focused)

	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldImage, form.focused)
}

func TestSettingsForm_ShiftTabNavigation(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})

	form = settingsPressShiftTab(form)
	assert.Equal(t, settingsFieldCancelButton, form.focused)

	form = settingsPressShiftTab(form)
	assert.Equal(t, settingsFieldSaveButton, form.focused)
}

func TestSettingsForm_SpaceTogglesTLS(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.settings.TLSEnabled())

	// Tab twice to get to TLS field (Image -> Hostname -> TLS)
	form = settingsPressTab(form)
	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressSpace(form)
	assert.False(t, form.settings.TLSEnabled())

	form = settingsPressSpace(form)
	assert.True(t, form.settings.TLSEnabled())
}

func TestSettingsForm_SpaceDoesNotToggleTLSForLocalhost(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "chat.localhost"})
	assert.False(t, form.settings.TLSEnabled())

	// Tab twice to get to TLS field
	form = settingsPressTab(form)
	form = settingsPressTab(form)
	assert.Equal(t, settingsFieldTLS, form.focused)

	form = settingsPressSpace(form)
	assert.False(t, form.settings.TLSEnabled(), "TLS should remain disabled for localhost")
}

func TestSettingsForm_TLSShowsDisabledForLocalhost(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})
	assert.True(t, form.settings.TLSEnabled())

	// Tab to hostname field and change to localhost
	form = settingsPressTab(form)
	form = settingsTypeText(form, ".localhost")
	assert.False(t, form.settings.TLSEnabled(), "TLS should show as disabled for localhost")

	// Change back to non-localhost - TLS preference is preserved
	form = settingsClearAndType(form, "app.example.com")
	assert.True(t, form.settings.TLSEnabled(), "TLS preference should be preserved")
}

func TestSettingsForm_Submit(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{
		Name:  "myapp",
		Image: "nginx:latest",
		Host:  "app.example.com",
	})

	form.focused = settingsFieldSaveButton
	_, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	submitMsg, ok := msg.(SettingsFormSubmitMsg)
	require.True(t, ok, "expected SettingsFormSubmitMsg, got %T", msg)
	assert.Equal(t, "myapp", submitMsg.Settings.Name)
	assert.Equal(t, "nginx:latest", submitMsg.Settings.Image)
	assert.Equal(t, "app.example.com", submitMsg.Settings.Host)
	assert.False(t, submitMsg.Settings.DisableTLS)
}

func TestSettingsForm_Cancel(t *testing.T) {
	form := NewSettingsForm(docker.ApplicationSettings{Host: "app.example.com"})

	form.focused = settingsFieldCancelButton
	_, cmd := form.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)

	msg := cmd()
	_, ok := msg.(SettingsFormCancelMsg)
	assert.True(t, ok, "expected SettingsFormCancelMsg, got %T", msg)
}

// Helpers

func settingsTypeText(form SettingsForm, text string) SettingsForm {
	for _, r := range text {
		form, _ = form.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return form
}

func settingsClearAndType(form SettingsForm, text string) SettingsForm {
	form.hostnameInput.SetValue("")
	form.settings.Host = ""
	return settingsTypeText(form, text)
}

func settingsPressTab(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	return form
}

func settingsPressShiftTab(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	return form
}

func settingsPressSpace(form SettingsForm) SettingsForm {
	form, _ = form.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	return form
}
