package keygen

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

func GenerateSSHKeyPair() (*KeyPair, error) {
	// Generate Ed25519 key pair
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	// Encode private key
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	privateKeyPEM := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	}
	privateKeyPEMBytes := pem.EncodeToMemory(privateKeyPEM)

	// Encode public key
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return nil, err
	}
	publicKeyBytes := ssh.MarshalAuthorizedKey(sshPublicKey)

	return &KeyPair{
		PrivateKey: privateKeyPEMBytes,
		PublicKey:  publicKeyBytes,
	}, nil
}
