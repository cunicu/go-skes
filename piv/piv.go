// SPDX-FileCopyrightText: 2023 Steffen Vogel <post@steffenvogel.de>
// SPDX-License-Identifier: Apache-2.0

package piv

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/go-piv/piv-go/piv"
)

func main() {
	if len(os.Args) < 2 {
		return
	}

	// List all smartcards connected to the system.
	cards, err := piv.Cards()
	if err != nil {
		panic(err)
	}

	// Find a YubiKey and open the reader.
	var yk *piv.YubiKey
	for _, card := range cards {
		if strings.Contains(strings.ToLower(card), "yubikey") {
			if yk, err = piv.Open(card); err != nil {
				panic(err)
			}
			break
		}
	}
	if yk == nil {
		panic("no yubikey found")
	}

	defer yk.Close()

	switch os.Args[1] {
	case "cert":
		cert(yk)
	case "decrypt":
		decrypt(yk)
	case "encrypt":
		encrypt(yk)
	}
}

func cert(yk *piv.YubiKey) {
	// sn, _ := yk.Serial()
	// fmt.Printf("Version: %d.%d.%d\n", yk.Version().Major, yk.Version().Minor, yk.Version().Patch)
	// fmt.Printf("Serial: %d\n", sn)

	crt, err := yk.Certificate(piv.SlotSignature)
	if err != nil {
		panic(err)
	}

	// Print the certificate
	// result, err := certinfo.CertificateText(crt)
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// fmt.Print(result)

	pemPayload, _ := x509.MarshalPKIXPublicKey(crt.PublicKey.(*rsa.PublicKey))

	pem.Encode(os.Stdout, &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pemPayload,
	})
}

func encrypt(yk *piv.YubiKey) {
	pt, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	crt, err := yk.Certificate(piv.SlotSignature)
	if err != nil {
		panic(err)
	}

	pk := crt.PublicKey.(*rsa.PublicKey)

	ct, err := rsa.EncryptPKCS1v15(rand.Reader, pk, pt)
	if err != nil {
		panic(err)
	}

	os.Stdout.Write(ct)
}

func decrypt(yk *piv.YubiKey) {
	ct, err := io.ReadAll(os.Stdin)
	if err != nil {
		panic(err)
	}

	crt, err := yk.Certificate(piv.SlotSignature)
	if err != nil {
		panic(err)
	}

	sk, err := yk.PrivateKey(piv.SlotSignature, crt.PublicKey, piv.KeyAuth{
		PIN: "111111",
	})
	if err != nil {
		panic(err)
	}

	dec, ok := sk.(crypto.Decrypter)
	if !ok {
		panic("no rsa key?")
	}

	pt, err := dec.Decrypt(rand.Reader, ct, nil)
	if err != nil {
		panic(err)
	}

	fmt.Print(string(pt))
}