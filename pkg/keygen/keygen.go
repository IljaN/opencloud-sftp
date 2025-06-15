package keygen

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"

	"golang.org/x/crypto/ssh"
)

type KeyPair struct {
	PrivateKey []byte
	PublicKey  []byte
}

type KeyType string

const (
	KeyTypeEd25519 KeyType = "ed25519"
	KeyTypeRSA     KeyType = "rsa"
)

func GenerateSSHKeyPair(keyType KeyType) (*KeyPair, error) {
	var privateKey interface{}
	var publicKey ssh.PublicKey
	var err error

	switch keyType {
	case KeyTypeEd25519:
		pub, priv, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, err
		}
		privateKey = priv
		publicKey, err = ssh.NewPublicKey(pub)
		if err != nil {
			return nil, err
		}
	case KeyTypeRSA:
		priv, err := rsa.GenerateKey(rand.Reader, 4096)
		if err != nil {
			return nil, err
		}
		privateKey = priv
		publicKey, err = ssh.NewPublicKey(&priv.PublicKey)
		if err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("unsupported key type")
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}

	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	publicKeyBytes := ssh.MarshalAuthorizedKey(publicKey)

	return &KeyPair{
		PrivateKey: privateKeyPEM,
		PublicKey:  publicKeyBytes,
	}, nil
}
