package config

// Built-in values used when neither an environment variable nor the database sets one.
// They are applied here, after the database fallback, and never as viper defaults: a
// viper default makes the variable read as set by the environment, which would shadow
// the value stored by the setup wizard or System settings.
const (
	defaultSMTPPort       = 587
	defaultSMTPFromName   = "Notifuse"
	defaultSMTPBridgePort = 587
)

// resolveSMTPConfig builds the system SMTP config: a non-empty environment variable wins
// field by field, the database fills the rest once installed, then the built-in defaults.
func resolveSMTPConfig(env EnvValues, ss *SystemSettings, isInstalled bool) SMTPConfig {
	c := SMTPConfig{
		Host:         env.SMTPHost,
		Port:         env.SMTPPort,
		Username:     env.SMTPUsername,
		Password:     env.SMTPPassword,
		FromEmail:    env.SMTPFromEmail,
		FromName:     env.SMTPFromName,
		UseTLS:       env.SMTPUseTLS != "false", // on unless explicitly set to false
		EHLOHostname: env.SMTPEHLOHostname,
	}

	if isInstalled && ss != nil {
		if c.Host == "" {
			c.Host = ss.SMTPHost
		}
		if c.Port == 0 {
			c.Port = ss.SMTPPort
		}
		if c.Username == "" {
			c.Username = ss.SMTPUsername
		}
		if c.Password == "" {
			c.Password = ss.SMTPPassword
		}
		if c.FromEmail == "" {
			c.FromEmail = ss.SMTPFromEmail
		}
		if c.FromName == "" {
			c.FromName = ss.SMTPFromName
		}
		if env.SMTPUseTLS == "" {
			c.UseTLS = ss.SMTPUseTLS
		}
		if c.EHLOHostname == "" {
			c.EHLOHostname = ss.SMTPEHLOHostname
		}
	}

	if c.Port == 0 {
		c.Port = defaultSMTPPort
	}
	if c.FromName == "" {
		c.FromName = defaultSMTPFromName
	}
	return c
}

// resolveSMTPBridgeConfig builds the SMTP bridge config with the same precedence as
// resolveSMTPConfig, except for the port and the TLS mode, which come from the
// environment only. Unset, the TLS mode is resolved from the certificates later in Load.
func resolveSMTPBridgeConfig(env EnvValues, ss *SystemSettings, isInstalled bool) SMTPBridgeConfig {
	c := SMTPBridgeConfig{
		Enabled:       env.SMTPBridgeEnabled == "true",
		Port:          env.SMTPBridgePort,
		Host:          "0.0.0.0",
		Domain:        env.SMTPBridgeDomain,
		TLSCertBase64: env.SMTPBridgeTLSCertBase64,
		TLSKeyBase64:  env.SMTPBridgeTLSKeyBase64,
		TLSMode:       env.SMTPBridgeTLSMode,
	}

	if isInstalled && ss != nil {
		// Only use the stored value when the env var is not set at all.
		if env.SMTPBridgeEnabled == "" {
			c.Enabled = ss.SMTPBridgeEnabled
		}
		if c.Domain == "" {
			c.Domain = ss.SMTPBridgeDomain
		}
		// The port is deliberately not read from the database: compose publishes the
		// container port from SMTP_BRIDGE_PORT, so a stored port would move the bridge
		// off the published one. It used to be shadowed by a viper default; keep that.
		if c.TLSCertBase64 == "" {
			c.TLSCertBase64 = ss.SMTPBridgeTLSCertBase64
		}
		if c.TLSKeyBase64 == "" {
			c.TLSKeyBase64 = ss.SMTPBridgeTLSKeyBase64
		}
	} else if c.Domain == "" {
		// First run: the bridge greets as localhost until a domain is configured.
		c.Domain = "localhost"
	}

	if c.Port == 0 {
		c.Port = defaultSMTPBridgePort
	}
	return c
}
