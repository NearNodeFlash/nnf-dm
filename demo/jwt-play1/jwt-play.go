package main

import (
	"bytes"
	"encoding/pem"
	"errors"
	"fmt"
	"os"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func createKey() ([]byte, error) {

	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return []byte{}, err
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return []byte{}, err
	}
	return privBytes, nil
}

func createToken(key []byte) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256,
		jwt.MapClaims{
			"service": "copy-offload-api",
			"uid":     uuid.New().String(),
		})

	tokenString, err := token.SignedString(key)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func verifyToken(tokenString string, key []byte) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != "HS256" {
			return nil, errors.New("unexpected signing method")
		}
		return key, nil
	})

	if err != nil {
		return fmt.Errorf("token parse failed: %w", err)
	}

	if !token.Valid {
		return errors.New("invalid token")
	}

	return nil
}

func main() {
	privKey, err := createKey()
	if err != nil {
		fmt.Printf("unable to create a signing key: %s\n", err.Error())
		os.Exit(1)
	}
	tokenString, err := createToken(privKey)
	if err != nil {
		fmt.Printf("unable to create token: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Printf("tokenString: %s\n", tokenString)

	err = verifyToken(tokenString, privKey)
	if err != nil {
		fmt.Printf("verify failed: %s\n", err.Error())
		os.Exit(1)
	}

	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privKey})
	if pemKey == nil {
		fmt.Printf("unable to encode signing key\n")
		os.Exit(1)
	}
	keyFname := "copy-offload-key.pem"
	if err := os.WriteFile(keyFname, pemKey, 0600); err != nil {
		fmt.Printf("unable to write file %s: %s\n", keyFname, err.Error())
		os.Exit(1)
	}

	tokFname := "copy-offload-token"
	if err := os.WriteFile(tokFname, []byte(tokenString), 0600); err != nil {
		fmt.Printf("unable to write file %s: %s\n", tokFname, err.Error())
		os.Exit(1)
	}

	inKey, err := os.ReadFile(keyFname)
	if err != nil {
		fmt.Printf("unable to read back the key file: %s\n", err.Error())
		os.Exit(1)
	}
	keyBlock, _ := pem.Decode(inKey)
	//fmt.Printf("keybytes: %s\n", string(keyBlock.Bytes))
	if !bytes.Equal(privKey, keyBlock.Bytes) {
		fmt.Printf("key block does not match private key\n")
		os.Exit(1)
	}

	tokenStr2, err := os.ReadFile(tokFname)
	if err != nil {
		fmt.Printf("unable to read back the token: %s\n", err.Error())
		os.Exit(1)
	}

	err = verifyToken(string(tokenStr2), keyBlock.Bytes)
	if err != nil {
		fmt.Printf("verify 2 failed: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Printf("good\n")
}
