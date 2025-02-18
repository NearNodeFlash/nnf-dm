/*
 * Copyright 2025 Hewlett Packard Enterprise Development LP
 * Other additional copyright holders may be indicated within.
 *
 * The entirety of this work is licensed under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 *
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package main

import (
	"bytes"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"

	"github.com/golang-jwt/jwt/v5"
)

func createKeyForTokens() ([]byte, []byte, error) {
	keyType := "EC PRIVATE KEY"
	privateKey, err := ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
	if err != nil {
		return []byte{}, []byte{}, fmt.Errorf("failure from GenerateKey: %w", err)
	}
	privBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return []byte{}, []byte{}, fmt.Errorf("failure from MarshalPKCS8PrivateKey: %w", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: keyType, Bytes: privBytes})
	if pemKey == nil {
		return []byte{}, []byte{}, errors.New("unable to PEM-encode signing key for token")
	}
	return privBytes, pemKey, nil
}

func createTokenFromKey(key []byte, method jwt.SigningMethod) (string, error) {
	token := jwt.NewWithClaims(method,
		jwt.MapClaims{
			"sub": "user-container",
			"iat": time.Now().Unix(),
		})

	tokenString, err := token.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("failure from SignedString: %w", err)
	}
	return tokenString, nil
}

func verifyToken(tokenString string, key []byte, method jwt.SigningMethod) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != method.Alg() {
			return nil, errors.New("token verification failed: unexpected signing method for token")
		}
		return key, nil
	})
	if err != nil {
		return fmt.Errorf("token verification failed: parse failed: %w", err)
	}
	if !token.Valid {
		return errors.New("token verification failed: invalid token")
	}
	return nil
}

func main() {

	tokenKeyFile := flag.String("tokenkey", "token_key.pem", "Output filel for the token key in PEM form.")
	tokenFile := flag.String("token", "token", "Output file for the token.")
	flag.Parse()

	privKey, pemKey, err := createKeyForTokens()
	if err != nil {
		fmt.Printf("unable to create a signing key: %s\n", err.Error())
		os.Exit(1)
	}

	signingMethod := jwt.SigningMethodHS256
	tokenString, err := createTokenFromKey(privKey, signingMethod)
	if err != nil {
		fmt.Printf("unable to create token: %s\n", err.Error())
		os.Exit(1)
	}

	err = verifyToken(tokenString, privKey, signingMethod)
	if err != nil {
		fmt.Printf("verify failed: %s\n", err.Error())
		os.Exit(1)
	}

	if err := os.WriteFile(*tokenKeyFile, pemKey, 0600); err != nil {
		fmt.Printf("unable to write file %s: %s\n", *tokenKeyFile, err.Error())
		os.Exit(1)
	}

	if err := os.WriteFile(*tokenFile, []byte(tokenString), 0600); err != nil {
		fmt.Printf("unable to write file %s: %s\n", *tokenFile, err.Error())
		os.Exit(1)
	}

	// Read the key and token from their files and verify that we can still
	// make sense of them.

	inKey, err := os.ReadFile(*tokenKeyFile)
	if err != nil {
		fmt.Printf("unable to read back the key file: %s\n", err.Error())
		os.Exit(1)
	}
	keyBlock, _ := pem.Decode(inKey)
	if !bytes.Equal(privKey, keyBlock.Bytes) {
		fmt.Printf("key block does not match private key\n")
		os.Exit(1)
	}

	tokenStr2, err := os.ReadFile(*tokenFile)
	if err != nil {
		fmt.Printf("unable to read back the token: %s\n", err.Error())
		os.Exit(1)
	}

	err = verifyToken(string(tokenStr2), keyBlock.Bytes, signingMethod)
	if err != nil {
		fmt.Printf("verify 2 failed: %s\n", err.Error())
		os.Exit(1)
	}

	os.Exit(0)
}
