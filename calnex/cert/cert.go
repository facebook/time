/*
Copyright (c) Facebook, Inc. and its affiliates.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cert

import (
	"bytes"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"
)

// This are the errors that can be raised by certificate parsing and validation
var (
	// ErrBundleNil indicates a nil bundle was supplied
	ErrBundleNil = errors.New("bundle is nil")
	// ErrBundleNoCertForHost indicates that there was no certificate for the given hostname
	ErrBundleNoCertForHost = errors.New("bundle has no certificate for supplied host")
	// ErrBundleNoCerts indicates that the supplied bundle contains no certificates
	ErrBundleNoCerts = errors.New("bundle has no certificates")
	// ErrBundleNoPrivKey indicates that the supplied bundle contains no private key
	ErrBundleNoPrivKey = errors.New("bundle has no private key")
	// ErrCertExpired indicates the supplied certificate is no longer valid
	ErrCertExpired = errors.New("certificate has expired")
	// ErrCertNotYetValid indicates that the valid data for the certificate has not been reached
	ErrCertNotYetValid = errors.New("certificate is not yet valid")
	// ErrFailedToParsePEM indicates the supplied data could not be parsed as valid PEM data
	ErrFailedToParsePEM = errors.New("failed to parse certificate PEM")
	// ErrMultiplePrivKeys indicates that more than one private key was found in the supplied PEM data
	ErrMultiplePrivKeys = errors.New("multiple private keys in PEM")
	// ErrOnlyRSA indicates a non-RSA private key was provided
	ErrOnlyRSA = errors.New("only RSA private keys are supported")
	// ErrUnsupportedPEMBlock indicates an unknown PEM block type was encountered in the supplied PEM data
	ErrUnsupportedPEMBlock = errors.New("unsupported PEM block")
)

// Bundle holds details of a key + its certificates
type Bundle struct {
	Certs   []*x509.Certificate
	PrivKey *rsa.PrivateKey
}

// Fetch connects to the remote host specification (hostname + TCP port) and retrieves
// the remote TLS certificate, returning it as a Bundle.
func Fetch(host string) (*Bundle, error) {
	conf := &tls.Config{
		InsecureSkipVerify: true,
	}

	conn, err := tls.Dial("tcp", host, conf)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	bundle := &Bundle{
		Certs: conn.ConnectionState().PeerCertificates,
	}

	return bundle, nil
}

// Parse parses the supplied byte array to check it is a valid x509 PEM file.
// If this is successful the certificate bundle (key + any certs) is returned.
// If not an error is returned.
func Parse(data []byte) (*Bundle, error) {
	bundle := &Bundle{}

	for len(data) > 0 {
		var block *pem.Block

		block, data = pem.Decode(data)
		if block == nil {
			return nil, ErrFailedToParsePEM
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			bundle.Certs = append(bundle.Certs, cert)
		} else if strings.Contains(block.Type, "PRIVATE KEY") {
			if bundle.PrivKey != nil {
				return nil, ErrMultiplePrivKeys
			}

			var key *rsa.PrivateKey
			var err error

			switch block.Type {
			case "RSA PRIVATE KEY":
				key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
				if err != nil {
					return nil, err
				}
			case "PRIVATE KEY":
				tmpKey, err := x509.ParsePKCS8PrivateKey(block.Bytes)
				if err != nil {
					return nil, err
				}

				ok := false
				key, ok = tmpKey.(*rsa.PrivateKey)
				if !ok {
					return nil, ErrOnlyRSA
				}
			default:
				return nil, ErrUnsupportedPEMBlock
			}

			bundle.PrivKey = key
		} else {
			return nil, ErrUnsupportedPEMBlock
		}
	}

	return bundle, nil
}

// Equals compares two bundles and returns true if they both contain exactly the same
// certificates.
func (b *Bundle) Equals(other *Bundle) bool {
	otherCerts := other.Certs
	for _, cert := range b.Certs {
		found := false
		for i, otherCert := range otherCerts {
			if bytes.Equal(cert.Raw, otherCert.Raw) {
				// It's ok to re-order this cert list to delete an element
				if len(otherCerts) > 1 {
					otherCerts[i] = otherCerts[len(otherCerts)-1]
					otherCerts = otherCerts[:len(otherCerts)-1]
				} else {
					otherCerts = nil
				}
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// We've walked through all of b.Certs; if there's nothing left from other.Certs
	// then we're equal
	return len(otherCerts) == 0
}

// Verify verifies the supplied bundle checking that:
//   - We have a private key
//   - We have a cert
//   - One of the cert CNs matches the supplied hostname
//   - The cert validity date before and ends after the supplied time.
//   - Other validations provided by x509 lib, inc. unknown authority
//
// Returns an error if any of these are not satisfied.
func (b *Bundle) Verify(hostname string, opts x509.VerifyOptions) error {
	t := opts.CurrentTime
	if t.IsZero() {
		t = time.Now()
	}

	if b == nil {
		return ErrBundleNil
	}

	if b.PrivKey == nil {
		return ErrBundleNoPrivKey
	}

	if len(b.Certs) == 0 {
		return ErrBundleNoCerts
	}

	// Confirm one of the certs is for our hostname
	var hostCert *x509.Certificate
	for _, cert := range b.Certs {
		err := cert.VerifyHostname(hostname)
		if err == nil {
			hostCert = cert
		}
	}

	if hostCert == nil {
		return ErrBundleNoCertForHost
	}

	if hostCert.NotBefore.After(t) {
		return ErrCertNotYetValid
	}

	if hostCert.NotAfter.Before(t) {
		return ErrCertExpired
	}

	_, err := hostCert.Verify(opts)

	return err
}
