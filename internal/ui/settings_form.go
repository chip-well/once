package ui

import (
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/basecamp/amar/internal/docker"
)

type settingsFormField int

const (
	settingsFieldImage settingsFormField = iota
	settingsFieldHostname
	settingsFieldTLS
	settingsFieldSMTPServer
	settingsFieldSMTPPort
	settingsFieldSMTPUsername
	settingsFieldSMTPPassword
	settingsFieldSMTPFrom
	settingsFieldSaveButton
	settingsFieldCancelButton
	settingsFieldCount
)

type SettingsFormSubmitMsg struct {
	Settings docker.ApplicationSettings
}

type SettingsFormCancelMsg struct{}

type SettingsForm struct {
	width, height     int
	focused           settingsFormField
	settings          docker.ApplicationSettings
	imageInput        textinput.Model
	hostnameInput     textinput.Model
	smtpServerInput   textinput.Model
	smtpPortInput     textinput.Model
	smtpUsernameInput textinput.Model
	smtpPasswordInput textinput.Model
	smtpFromInput     textinput.Model
}

func NewSettingsForm(settings docker.ApplicationSettings) SettingsForm {
	image := textinput.New()
	image.Placeholder = "user/repo:tag"
	image.Prompt = ""
	image.CharLimit = 256
	image.SetValue(settings.Image)
	image.Focus()

	hostname := textinput.New()
	hostname.Placeholder = "app.example.com"
	hostname.Prompt = ""
	hostname.CharLimit = 256
	hostname.SetValue(settings.Host)

	smtpServer := textinput.New()
	smtpServer.Placeholder = "smtp.example.com"
	smtpServer.Prompt = ""
	smtpServer.CharLimit = 256
	smtpServer.SetValue(settings.SMTP.Server)

	smtpPort := textinput.New()
	smtpPort.Placeholder = "587"
	smtpPort.Prompt = ""
	smtpPort.CharLimit = 5
	smtpPort.SetValue(settings.SMTP.Port)

	smtpUsername := textinput.New()
	smtpUsername.Placeholder = "user@example.com"
	smtpUsername.Prompt = ""
	smtpUsername.CharLimit = 256
	smtpUsername.SetValue(settings.SMTP.Username)

	smtpPassword := textinput.New()
	smtpPassword.Placeholder = "password"
	smtpPassword.Prompt = ""
	smtpPassword.CharLimit = 256
	smtpPassword.EchoMode = textinput.EchoPassword
	smtpPassword.SetValue(settings.SMTP.Password)

	smtpFrom := textinput.New()
	smtpFrom.Placeholder = "noreply@example.com"
	smtpFrom.Prompt = ""
	smtpFrom.CharLimit = 256
	smtpFrom.SetValue(settings.SMTP.From)

	return SettingsForm{
		focused:           settingsFieldImage,
		settings:          settings,
		imageInput:        image,
		hostnameInput:     hostname,
		smtpServerInput:   smtpServer,
		smtpPortInput:     smtpPort,
		smtpUsernameInput: smtpUsername,
		smtpPasswordInput: smtpPassword,
		smtpFromInput:     smtpFrom,
	}
}

func (m SettingsForm) Init() tea.Cmd {
	return nil
}

func (m SettingsForm) Update(msg tea.Msg) (SettingsForm, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		inputWidth := min(m.width-4, 60)
		m.imageInput.SetWidth(inputWidth)
		m.hostnameInput.SetWidth(inputWidth)
		m.smtpServerInput.SetWidth(inputWidth)
		m.smtpPortInput.SetWidth(inputWidth)
		m.smtpUsernameInput.SetWidth(inputWidth)
		m.smtpPasswordInput.SetWidth(inputWidth)
		m.smtpFromInput.SetWidth(inputWidth)

	case tea.KeyMsg:
		switch {
		case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
			return m.focusNext()
		case key.Matches(msg, key.NewBinding(key.WithKeys("shift+tab"))):
			return m.focusPrev()
		case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
			return m.handleEnter()
		case key.Matches(msg, key.NewBinding(key.WithKeys("space"))) && m.focused == settingsFieldTLS:
			if !docker.IsLocalhost(m.settings.Host) {
				m.settings.DisableTLS = !m.settings.DisableTLS
			}
			return m, nil
		}
	}

	switch m.focused {
	case settingsFieldImage:
		var cmd tea.Cmd
		m.imageInput, cmd = m.imageInput.Update(msg)
		m.settings.Image = m.imageInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldHostname:
		var cmd tea.Cmd
		m.hostnameInput, cmd = m.hostnameInput.Update(msg)
		m.settings.Host = m.hostnameInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldSMTPServer:
		var cmd tea.Cmd
		m.smtpServerInput, cmd = m.smtpServerInput.Update(msg)
		m.settings.SMTP.Server = m.smtpServerInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldSMTPPort:
		var cmd tea.Cmd
		m.smtpPortInput, cmd = m.smtpPortInput.Update(msg)
		m.settings.SMTP.Port = m.smtpPortInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldSMTPUsername:
		var cmd tea.Cmd
		m.smtpUsernameInput, cmd = m.smtpUsernameInput.Update(msg)
		m.settings.SMTP.Username = m.smtpUsernameInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldSMTPPassword:
		var cmd tea.Cmd
		m.smtpPasswordInput, cmd = m.smtpPasswordInput.Update(msg)
		m.settings.SMTP.Password = m.smtpPasswordInput.Value()
		cmds = append(cmds, cmd)
	case settingsFieldSMTPFrom:
		var cmd tea.Cmd
		m.smtpFromInput, cmd = m.smtpFromInput.Update(msg)
		m.settings.SMTP.From = m.smtpFromInput.Value()
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m SettingsForm) View() string {
	imageLabel := Styles.Label.Render("Image")
	imageField := Styles.Focus(Styles.Input, m.focused == settingsFieldImage).
		Render(m.imageInput.View())

	hostnameLabel := Styles.Label.Render("Hostname")
	hostnameField := Styles.Focus(Styles.Input, m.focused == settingsFieldHostname).
		Render(m.hostnameInput.View())

	tlsLabel := Styles.Label.Render("TLS")
	var tlsText string
	if docker.IsLocalhost(m.settings.Host) {
		tlsText = "Not available for localhost"
	} else if m.settings.TLSEnabled() {
		tlsText = "[x] Enabled"
	} else {
		tlsText = "[ ] Enabled"
	}
	tlsField := Styles.Focus(Styles.Input, m.focused == settingsFieldTLS).
		Render(tlsText)

	smtpServerLabel := Styles.Label.Render("SMTP Server")
	smtpServerField := Styles.Focus(Styles.Input, m.focused == settingsFieldSMTPServer).
		Render(m.smtpServerInput.View())

	smtpPortLabel := Styles.Label.Render("SMTP Port")
	smtpPortField := Styles.Focus(Styles.Input, m.focused == settingsFieldSMTPPort).
		Render(m.smtpPortInput.View())

	smtpUsernameLabel := Styles.Label.Render("SMTP Username")
	smtpUsernameField := Styles.Focus(Styles.Input, m.focused == settingsFieldSMTPUsername).
		Render(m.smtpUsernameInput.View())

	smtpPasswordLabel := Styles.Label.Render("SMTP Password")
	smtpPasswordField := Styles.Focus(Styles.Input, m.focused == settingsFieldSMTPPassword).
		Render(m.smtpPasswordInput.View())

	smtpFromLabel := Styles.Label.Render("SMTP From")
	smtpFromField := Styles.Focus(Styles.Input, m.focused == settingsFieldSMTPFrom).
		Render(m.smtpFromInput.View())

	saveButton := Styles.Focus(Styles.ButtonPrimary, m.focused == settingsFieldSaveButton).
		Render("Save")
	cancelButton := Styles.Focus(Styles.Button, m.focused == settingsFieldCancelButton).
		Render("Cancel")

	buttons := lipgloss.JoinHorizontal(lipgloss.Center, saveButton, cancelButton)

	form := lipgloss.JoinVertical(lipgloss.Left,
		imageLabel,
		imageField,
		hostnameLabel,
		hostnameField,
		tlsLabel,
		tlsField,
		"",
		smtpServerLabel,
		smtpServerField,
		smtpPortLabel,
		smtpPortField,
		smtpUsernameLabel,
		smtpUsernameField,
		smtpPasswordLabel,
		smtpPasswordField,
		smtpFromLabel,
		smtpFromField,
		"",
		buttons,
	)

	return form
}

// Private

func (m SettingsForm) focusNext() (SettingsForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused + 1) % settingsFieldCount
	return m.focusCurrent()
}

func (m SettingsForm) focusPrev() (SettingsForm, tea.Cmd) {
	m.blurCurrent()
	m.focused = (m.focused - 1 + settingsFieldCount) % settingsFieldCount
	return m.focusCurrent()
}

func (m *SettingsForm) blurCurrent() {
	switch m.focused {
	case settingsFieldImage:
		m.imageInput.Blur()
	case settingsFieldHostname:
		m.hostnameInput.Blur()
	case settingsFieldSMTPServer:
		m.smtpServerInput.Blur()
	case settingsFieldSMTPPort:
		m.smtpPortInput.Blur()
	case settingsFieldSMTPUsername:
		m.smtpUsernameInput.Blur()
	case settingsFieldSMTPPassword:
		m.smtpPasswordInput.Blur()
	case settingsFieldSMTPFrom:
		m.smtpFromInput.Blur()
	}
}

func (m SettingsForm) focusCurrent() (SettingsForm, tea.Cmd) {
	var cmd tea.Cmd
	switch m.focused {
	case settingsFieldImage:
		cmd = m.imageInput.Focus()
	case settingsFieldHostname:
		cmd = m.hostnameInput.Focus()
	case settingsFieldSMTPServer:
		cmd = m.smtpServerInput.Focus()
	case settingsFieldSMTPPort:
		cmd = m.smtpPortInput.Focus()
	case settingsFieldSMTPUsername:
		cmd = m.smtpUsernameInput.Focus()
	case settingsFieldSMTPPassword:
		cmd = m.smtpPasswordInput.Focus()
	case settingsFieldSMTPFrom:
		cmd = m.smtpFromInput.Focus()
	}
	return m, cmd
}

func (m SettingsForm) handleEnter() (SettingsForm, tea.Cmd) {
	switch m.focused {
	case settingsFieldImage, settingsFieldHostname, settingsFieldTLS,
		settingsFieldSMTPServer, settingsFieldSMTPPort, settingsFieldSMTPUsername, settingsFieldSMTPPassword, settingsFieldSMTPFrom:
		return m.focusNext()
	case settingsFieldSaveButton:
		return m.submitForm()
	case settingsFieldCancelButton:
		return m, func() tea.Msg { return SettingsFormCancelMsg{} }
	}
	return m, nil
}

func (m SettingsForm) submitForm() (SettingsForm, tea.Cmd) {
	return m, func() tea.Msg {
		return SettingsFormSubmitMsg{Settings: m.settings}
	}
}
