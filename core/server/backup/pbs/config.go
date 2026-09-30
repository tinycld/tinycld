package pbs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	gopbs "github.com/osshield/gopbs/pbs"
)

type Config struct {
	Server      string `json:"server"` // "host", "host:port" or "https://host:port"
	Fingerprint string `json:"fingerprint"`
	Datastore   string `json:"datastore"`
	Namespace   string `json:"namespace"`
	AuthID      string `json:"auth_id"`
	Secret      string `json:"secret"`
	Key         string `json:"key"`       // PBS key file JSON; "" ⇒ no encryption
	BackupID    string `json:"backup_id"` // set by the caller
}

func ParseConfig(raw json.RawMessage) (Config, error) {
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, errors.New("backup: the PBS settings are not valid JSON")
	}
	for _, field := range []struct{ name, value string }{
		{"server", c.Server}, {"datastore", c.Datastore}, {"auth_id", c.AuthID},
		{"secret", c.Secret}, {"backup_id", c.BackupID},
	} {
		if strings.TrimSpace(field.value) == "" {
			return c, fmt.Errorf("backup: the PBS setting %q is required", field.name)
		}
	}
	if !strings.Contains(c.AuthID, "!") {
		return c, errors.New("backup: the PBS auth ID must be an API token (user@realm!name)")
	}
	return c, nil
}

// baseURL accepts "host", "host:port" or a full https URL.
func (c Config) baseURL() string {
	s := strings.TrimSuffix(strings.TrimSpace(c.Server), "/")
	if !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return s
	}
	if u.Port() == "" {
		u.Host = net.JoinHostPort(u.Hostname(), "8007")
	}
	return u.Scheme + "://" + u.Host
}

// Host is the only part of the config that may be logged or stored in a row.
func (c Config) Host() string {
	u, err := url.Parse(c.baseURL())
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func (c Config) crypt() (*gopbs.CryptConfig, error) {
	if strings.TrimSpace(c.Key) == "" {
		return nil, nil
	}
	info, err := gopbs.LoadKeyFile([]byte(c.Key), nil)
	if err != nil {
		// The key is never quoted.
		return nil, errors.New("backup: the PBS encryption key could not be read")
	}
	return info.CryptConfig(gopbs.CryptModeEncrypt), nil
}

// GenerateKey returns a new unprotected PBS key file. The admin must keep a
// copy: without it the backups cannot be read.
func GenerateKey() (string, error) {
	key, err := gopbs.GenerateEncryptionKey()
	if err != nil {
		return "", err
	}
	raw, err := gopbs.CreateKeyFile(key, nil, gopbs.KDFNone, "tinycld")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
