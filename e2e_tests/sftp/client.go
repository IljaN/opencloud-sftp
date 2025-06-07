//go:build e2e

package sftp

import (
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type Config struct {
	Address        string `env:"ADDR" envDefault:"127.0.1.1:2222"`
	PrivateKeyPath string `env:"KEY_PATH"`
}

type Client struct {
	*sftp.Client
	SSHClient *ssh.Client
	Config    Config
}

func (c *Client) Close() error {
	if c.Client != nil {
		if err := c.Client.Close(); err != nil {
			return err
		}
	}

	if c.SSHClient != nil {
		if err := c.SSHClient.Close(); err != nil {
			return err
		}
	}

	return nil
}
