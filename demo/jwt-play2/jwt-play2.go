package main

import (
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

func verifyToken(tokenString string, key []byte) error {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != "HS256" {
			return nil, errors.New("unexpected signing method")
		}
		return key, nil
	})

	if err != nil {
		return err
	}

	if !token.Valid {
		return fmt.Errorf("invalid token")
	}

	return nil
}

func main() {

	fmt.Println("t1")
	inKey, err := os.ReadFile("/tmp/key.pem")
	if err != nil {
		fmt.Printf("unable to read back the key file: %s\n", err.Error())
		os.Exit(1)
	}

	inToken, err := os.ReadFile("/tmp/token")
	if err != nil {
		fmt.Printf("unable to read back the token file: %s\n", err.Error())
		os.Exit(1)
	}
	tokenString := strings.TrimSuffix(string(inToken), "\n")

	fmt.Printf("tokenString: (%s)\n", tokenString)

	keyBlock, _ := pem.Decode(inKey)
	if keyBlock == nil {
		fmt.Printf("unable to decode PEM block\n")
		os.Exit(1)
	}
	derStr := base64.StdEncoding.EncodeToString(keyBlock.Bytes)
	fmt.Printf("der (%s)\n", derStr)

	keyFname := "copy-offload-key.der"
	if err := os.WriteFile(keyFname, []byte(derStr), 0600); err != nil {
		fmt.Printf("unable to write file %s: %s\n", keyFname, err.Error())
		os.Exit(1)
	}

	fail := false
	err = verifyToken(tokenString, keyBlock.Bytes)
	if err != nil {
		fmt.Printf("verify failed with keyBlock.Bytes: %s\n", err.Error())
		fail = true
	}

	// This should never work--the base64-encoded bytes cannot just be cast
	// to []byte.
	err = verifyToken(tokenString, []byte(derStr))
	if err == nil {
		fmt.Println("FAIL: []byte(derStr) should not have succeeded.")
		fail = true
	}

	if fail {
		os.Exit(1)
	}
	fmt.Printf("good\n")
}
