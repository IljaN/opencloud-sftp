//go:build e2e

package e2e_tests

import (
	"github.com/IljaN/opencloud-sftp/e2e_tests/gateway"
	"github.com/IljaN/opencloud-sftp/e2e_tests/sftp"
	extSftp "github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

func NewClientFactory(sftpCfg sftp.Config, gwCfg gateway.Config) *ClientFactory {
	return &ClientFactory{
		keyCache:  make(map[string]sftp.KeyPair),
		sftConfig: sftpCfg,
		gwConfig:  gwCfg,
	}
}

type ClientFactory struct {
	keyCache  map[string]sftp.KeyPair
	gwConfig  gateway.Config
	sftConfig sftp.Config
}

func (c *ClientFactory) NewClient(uid string) (*sftp.Client, error) {
	var keyPair *sftp.KeyPair
	var err error

	if kp, exists := c.keyCache[uid]; exists {
		keyPair = &kp
	} else {
		keyPair, err = sftp.GenerateSSHKeyPair()
		if err != nil {
			return nil, err
		}

		gwClient, err := gateway.NewClient(&c.gwConfig, uid)
		if err != nil {
			return nil, err
		}

		err = gwClient.DeployPublicKey(keyPair)
		if err != nil {
			return nil, err
		}

		c.keyCache[uid] = *keyPair

	}

	sshClientConfig := &ssh.ClientConfig{
		User: uid,
		Auth: []ssh.AuthMethod{
			authMethodFromKeyPair(keyPair),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	sshClient, err := ssh.Dial("tcp", c.sftConfig.Address, sshClientConfig)
	if err != nil {
		return nil, err
	}

	sftpClient, err := extSftp.NewClient(sshClient)
	if err != nil {
		return nil, err
	}

	return &sftp.Client{
		Client:    sftpClient,
		SSHClient: sshClient,
		Config:    c.sftConfig,
	}, nil

}

func authMethodFromKeyPair(kp *sftp.KeyPair) ssh.AuthMethod {
	signer, err := ssh.ParsePrivateKey(kp.PrivateKey)
	if err != nil {
		panic(err)
	}
	return ssh.PublicKeys(signer)
}
