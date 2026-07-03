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
	"crypto/x509"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generated with
// openssl genrsa -out "root.key" 3072
//
//		openssl req -x509 -nodes -sha256 -new -key "root.key" -out "root.crt" -days 7310 \
//		  -subj "/CN=Custom Root" \
//		  -addext "keyUsage = critical, keyCertSign" \
//		  -addext "basicConstraints = critical, CA:TRUE, pathlen:0" \
//		  -addext "subjectKeyIdentifier = hash"
//	   Validity
//	           Not Before: Jul  3 12:49:00 2026 GMT
//	           Not After : Jul  8 12:49:00 2046 GMT
const rootCA = `-----BEGIN CERTIFICATE-----
MIIEIDCCAoigAwIBAgIUKHP7CqmIKOtIBsKReRXaaWEaUwEwDQYJKoZIhvcNAQEL
BQAwFjEUMBIGA1UEAwwLQ3VzdG9tIFJvb3QwHhcNMjYwNzAzMTI0OTAwWhcNNDYw
NzA4MTI0OTAwWjAWMRQwEgYDVQQDDAtDdXN0b20gUm9vdDCCAaIwDQYJKoZIhvcN
AQEBBQADggGPADCCAYoCggGBAMmooOlJnVGLTCdo7SWkyDRTeWmxis8ns8wTEiQ6
1/IlSu1R6+wM5J/KYHcWXsAOjBBOj00GEQ/TeMwjLhOOZU0Y2zCDduvmWsKeOMjz
UgvuA7frGL/6ruPG9Ko4a2omS8IfAU/OnWARBM5d8wslVIPsirFY6v892qj4i3j3
OMFMWrDyiCiu+/nltYy2r+Ix2cmIHgddlHDbS7tzZEmlad48VftfAApWQohRVRnb
FMpBVhuVPfTNjlKLrOsyJi2YqAUMGk3C/KBXXg27OQ+FfH+1vtwzDBjqsSD8EvIh
kugz71ZcVX9RmnZnaaGlHRM4gaC26t7G2d2yLadJwjBEjStk3Yd1vwnDejKbzd2P
dux1zWr7n/YF13G/FuNsud7dabNSuh7KjhyPp7SgM9asFd2hsnf6yYW1KwfYRLJi
LqYuM/qKBjq/gPudii6TptYnnhyruBsAt8umsUM7h+AHYWsrTFZhU2AqmY4rFNi2
CAM1R6EORsi5RG8ij3N33iXgnwIDAQABo2YwZDAfBgNVHSMEGDAWgBQ0j7rdTIqB
YIkkfFCTcBcPtjqhEzAOBgNVHQ8BAf8EBAMCAgQwEgYDVR0TAQH/BAgwBgEB/wIB
ADAdBgNVHQ4EFgQUNI+63UyKgWCJJHxQk3AXD7Y6oRMwDQYJKoZIhvcNAQELBQAD
ggGBAC4ELWWd/W88S0f0kRworl78b8AQcK+DUkNTFw05kV3+zY0m3mdPHpkZB8C2
uAcKDlpazMuc6o2Gz+POQ6jDVRqf26wt7lRLghxfG6AbVrITJfI42OjwMxPD8rgy
ofUx9/jKZiU7btzfyluRbh5lwvVw0/PmKzwltT/7FV1+0EY8+TKLVbzyzq7BOyyG
OtwnGEcasVT5BmHO7FOv6LDtHxaKrVvoNaYsXC1svc8EugLFaImCSDXqlCF16Ggp
5gVJZAsEccGyLBkTf0mIOR4Kw85UB3LmEIenfoyT6LGOg7KV+ndc1Py2i2k+t83J
YK2Z+uEl16x5eRNKHLYq1QmiXedbUsjCK+7w6AoTqp08Ny5vRTe8sOpTRmr3UpBG
IG00q0bXI0adHBRtxLN+2pM4bxqo6rjSZSXTkFyLYhMCF+zA5RUxjdWX8+1fAckI
zDydQVA4wWgcP43ZsW9S+wvbxEevfPC787j+WcT4uC8QNTiMDdesJ9bBJFEumbRg
1qBRBQ==
-----END CERTIFICATE-----
`

// Test certificate generated with:
//
// openssl genrsa -out "testhost.invalid.key" 2048
// openssl req -sha256 -new -key "testhost.invalid.key" -out "testhost.invalid.csr" \
// -subj "/CN=*.testhost.invalid" \
// -reqexts SAN -config <(echo "[SAN]\nsubjectAltName=DNS:testhost.invalid,DNS:*.testhost.invalid\n")
//
//	openssl x509 -req -sha256 -in "testhost.invalid.csr" -out "testhost.invalid.crt" -days 7300 \
//	  -CAkey "root.key" -CA "root.crt" -CAcreateserial -extfile <(cat <<END
//	    subjectAltName = DNS:testhost.invalid,DNS:*.testhost.invalid
//	    keyUsage = critical, digitalSignature, keyEncipherment
//	    extendedKeyUsage = serverAuth
//	    basicConstraints = CA:FALSE
//	    authorityKeyIdentifier = keyid:always
//	    subjectKeyIdentifier = none
//
// END
// )
//
//	Validity
//	    Not Before: Jul  3 13:09:23 2026 GMT
//	    Not After : Jun 28 13:09:23 2046 GMT
const testCertificate = `-----BEGIN CERTIFICATE-----
MIIDxzCCAi+gAwIBAgIUTiSlTNXHhhTXgZ3tUqrNXHWkX+EwDQYJKoZIhvcNAQEL
BQAwFjEUMBIGA1UEAwwLQ3VzdG9tIFJvb3QwHhcNMjYwNzAzMTMwOTIzWhcNNDYw
NjI4MTMwOTIzWjAdMRswGQYDVQQDDBIqLnRlc3Rob3N0LmludmFsaWQwggEiMA0G
CSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQDiEBmt/VTTgM7WoPy6dDBZb1FoCrDk
Gzx2aWF/CVkgPnMM8Keiqf71j2/+ptqJsrjePamDCU5TO7K80IZABggveOOE3uhy
B3u1Ey+owZvKdwS3CVQrW5JXGLaLtcaiIM4VfuIND60J28GqRVrhZnWb0r/Y6zUi
0pr6jTlvvF8BKzWiiRtR3LtkhES90IzQKCqM6ssRLFZsg8sr7CYCB+Kr5hjD6kHv
L9+9IsSbTxSLW3v6SfZd9K+q4fNyWmoevgdDFlJeER62mXaAGdBsaCjmas4q7xZs
fyvs8DN0jbpRqP60XmpjmPLcF30snNdNUa6ecnKUgpt9wog5peQiOZZnAgMBAAGj
gYUwgYIwLwYDVR0RBCgwJoIQdGVzdGhvc3QuaW52YWxpZIISKi50ZXN0aG9zdC5p
bnZhbGlkMA4GA1UdDwEB/wQEAwIFoDATBgNVHSUEDDAKBggrBgEFBQcDATAJBgNV
HRMEAjAAMB8GA1UdIwQYMBaAFDSPut1MioFgiSR8UJNwFw+2OqETMA0GCSqGSIb3
DQEBCwUAA4IBgQCOU0q/lYJHyy/mZOGgxhUvn//LsXhFJSHH8/6vDiPsoqNnCKl7
SW/VURpKgd3jZ33gGGH0Pb/upZNQexfyIQLRzu5buvynMNtDG/bm0xe80hqgIbe4
VrnVuDnOnHb5FpGKkIsSZY8YzD8baJfZ1AC2h/4ouvvxVTPnpJzsYMQ+f/r5nvBZ
z3A1FwM088qURuStSwwqExjayGMl5O66oxDwnEvVfP0HLJVhzVpOeqbwpkJZw+37
7XUbOR+wc8tSugc1qAr5awgt1s8uRlxdEjdA/SQ69jRewcoGbu+qt1W2B/gWZmxA
AsWMJIMmn5ODfEUpFX5XutII2/e+eP5KDdSrzJ31hGuje3o5ZgZ/UXpp1RKEcr0L
7N9LUHPOIIaJvYgO2G5k4Ls+6+CwpZiRW17blXlG1Ad/BzHndxREMxpe+8hLvxV3
ZncHqoFxHThMDOJxv6/EPLZptG9HVUsjQh0KhRCwWEC++fyMLzXUiO17uElZt0GE
kEhmJtPVJj8bhb0=
-----END CERTIFICATE-----
`

// @lint-ignore PRIVATEKEY insecure-private-key-storage
const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDiEBmt/VTTgM7W
oPy6dDBZb1FoCrDkGzx2aWF/CVkgPnMM8Keiqf71j2/+ptqJsrjePamDCU5TO7K8
0IZABggveOOE3uhyB3u1Ey+owZvKdwS3CVQrW5JXGLaLtcaiIM4VfuIND60J28Gq
RVrhZnWb0r/Y6zUi0pr6jTlvvF8BKzWiiRtR3LtkhES90IzQKCqM6ssRLFZsg8sr
7CYCB+Kr5hjD6kHvL9+9IsSbTxSLW3v6SfZd9K+q4fNyWmoevgdDFlJeER62mXaA
GdBsaCjmas4q7xZsfyvs8DN0jbpRqP60XmpjmPLcF30snNdNUa6ecnKUgpt9wog5
peQiOZZnAgMBAAECggEAS8AaYsd/S6ofZSMn3LC/XNCk5iii8qS/w2v3fBKdV2Ul
t0HS4np1UUKhxCKUG00Ujn/6E8skAFcCQyvauIxs5L9s+eKZ4E/qn5gQwcsykYsF
PPI2zpqONHo2/STJrR0yAVj1lWvZz3JgeFZqKBplsXPSznSuZv9MaEW3Z94mtaRs
R2+fBscFruiilseceBIOdH7K9tJ+3qG1qNGCoiQWVN01b2KQsTIeyH/hxoGCh5HD
tfKZ/E7fVsPvAx56qfiklKbRjOwq7Fy+OzokY7cwBhHVycv/U47qZYYmUpjE72qQ
9XvVLVlEP+bMY710pTZiGRz84cYfqHz1YE7Fo6MT9QKBgQDypmvODXHrIQ/w9W/F
+LLm8onyepQG2losP/Y0fqTz/+UnoX9EuwxhgsvdPQqyeH5i/DyhyvKJPqNtXGcm
0rYFOmWCA5CupD/SJSRdXHzk5VZnGU7/hwVTobicJY6AYkPKyl+hCFZ58Y4WwbIN
v3jh4FI29QqE+Am8IKE3oDBuRQKBgQDugA8HJNxBxvZjQj8zaONp3hS678SVPr91
0tfVS4QsZFoGAvXxcDvzSIBLrjie15yuloUfGnHKT6VNiprZxRmxAyeWcu1Rlmbs
pfsUHXy4+fD11A9SZ/54b8Sfbku1neqpq2vcqigpIbXLIWD/243W+lK7e59zGnA8
7nAyDhKCuwKBgQC7dfTdaKe01oMhTgx/LsbQA1qteSO5M6Hsg7GrFphLZUvdVTgk
mjlTcCAdmNYV0V8bC/GvsUG05C6QA44xgSJcYaQgUK7LLVuc91Ljyds3XzJkTjoo
0WA9HzincaBo8QGcvsIof2+HoCV80UHEu0MhhhMeICtzVMj4jWDfv6MK3QKBgAQ4
sAtoU522b9YB7ixyxtOw4p0McWZS3gCv4rIbzBMdE5rXopLLccQ0nFC6nLXzCwrs
Dx8l0K3MCxj8QxFns7S2YZUAI8M17kxyA6evfe2oPuObBUpoHND06X4I7b4hNW4b
YqVdPai8uAMIbDcbI7+SXrSC06et6B6r+cBpD1rRAoGATP7AdyFkeRB51gFdRp8y
U7/7WDwHtjg8wHf1KbkbXJ8mjyOWrRbT2Rn1SEBOHvYMSZY3189tyQcZH4nKwHGv
xeCrDMNL1M4RShGpZrcJo1nx60rVL4wX1pSXyOWAoyKGyTl8Yjs5fd6C0VOU51k2
Vqlm3rrOLjJVbS2Qwx42h/Q=
-----END PRIVATE KEY-----
`

// Taken from https://letsencrypt.org/certs/isrgrootx1.pem
const letsEncryptX1 = `-----BEGIN CERTIFICATE-----
MIIFazCCA1OgAwIBAgIRAIIQz7DSQONZRGPgu2OCiwAwDQYJKoZIhvcNAQELBQAw
TzELMAkGA1UEBhMCVVMxKTAnBgNVBAoTIEludGVybmV0IFNlY3VyaXR5IFJlc2Vh
cmNoIEdyb3VwMRUwEwYDVQQDEwxJU1JHIFJvb3QgWDEwHhcNMTUwNjA0MTEwNDM4
WhcNMzUwNjA0MTEwNDM4WjBPMQswCQYDVQQGEwJVUzEpMCcGA1UEChMgSW50ZXJu
ZXQgU2VjdXJpdHkgUmVzZWFyY2ggR3JvdXAxFTATBgNVBAMTDElTUkcgUm9vdCBY
MTCCAiIwDQYJKoZIhvcNAQEBBQADggIPADCCAgoCggIBAK3oJHP0FDfzm54rVygc
h77ct984kIxuPOZXoHj3dcKi/vVqbvYATyjb3miGbESTtrFj/RQSa78f0uoxmyF+
0TM8ukj13Xnfs7j/EvEhmkvBioZxaUpmZmyPfjxwv60pIgbz5MDmgK7iS4+3mX6U
A5/TR5d8mUgjU+g4rk8Kb4Mu0UlXjIB0ttov0DiNewNwIRt18jA8+o+u3dpjq+sW
T8KOEUt+zwvo/7V3LvSye0rgTBIlDHCNAymg4VMk7BPZ7hm/ELNKjD+Jo2FR3qyH
B5T0Y3HsLuJvW5iB4YlcNHlsdu87kGJ55tukmi8mxdAQ4Q7e2RCOFvu396j3x+UC
B5iPNgiV5+I3lg02dZ77DnKxHZu8A/lJBdiB3QW0KtZB6awBdpUKD9jf1b0SHzUv
KBds0pjBqAlkd25HN7rOrFleaJ1/ctaJxQZBKT5ZPt0m9STJEadao0xAH0ahmbWn
OlFuhjuefXKnEgV4We0+UXgVCwOPjdAvBbI+e0ocS3MFEvzG6uBQE3xDk3SzynTn
jh8BCNAw1FtxNrQHusEwMFxIt4I7mKZ9YIqioymCzLq9gwQbooMDQaHWBfEbwrbw
qHyGO0aoSCqI3Haadr8faqU9GY/rOPNk3sgrDQoo//fb4hVC1CLQJ13hef4Y53CI
rU7m2Ys6xt0nUW7/vGT1M0NPAgMBAAGjQjBAMA4GA1UdDwEB/wQEAwIBBjAPBgNV
HRMBAf8EBTADAQH/MB0GA1UdDgQWBBR5tFnme7bl5AFzgAiIyBpY9umbbjANBgkq
hkiG9w0BAQsFAAOCAgEAVR9YqbyyqFDQDLHYGmkgJykIrGF1XIpu+ILlaS/V9lZL
ubhzEFnTIZd+50xx+7LSYK05qAvqFyFWhfFQDlnrzuBZ6brJFe+GnY+EgPbk6ZGQ
3BebYhtF8GaV0nxvwuo77x/Py9auJ/GpsMiu/X1+mvoiBOv/2X/qkSsisRcOj/KK
NFtY2PwByVS5uCbMiogziUwthDyC3+6WVwW6LLv3xLfHTjuCvjHIInNzktHCgKQ5
ORAzI4JMPJ+GslWYHb4phowim57iaztXOoJwTdwJx4nLCgdNbOhdjsnvzqvHu7Ur
TkXWStAmzOVyyghqpZXjFaH3pO3JLF+l+/+sKAIuvtd7u+Nxe5AW0wdeRlN8NwdC
jNPElpzVmbUq4JUagEiuTDkHzsxHpFKVK7q4+63SM1N95R1NbdWhscdCb+ZAJzVc
oyi3B43njTOQ5yOf+1CceWxG1bQVs5ZufpsMljq4Ui0/1lvh+wjChP4kqKOJ2qxq
4RgqsahDYVvTH9w7jXbyLeiNdd8XM2w9U/t7y0Ff/9yi0GE44Za4rF2LN9d11TPA
mRGunUHBcnWEvgJBQl9nJEiU0Zsnvgc/ubhPgXRR4Xq37Z0j4r7g1SgEEzwxA57d
emyPxgcYxn/eR44/KJ4EBs+lVDR3veyJm+kXQ99b21/+jh5Xos1AnX5iItreGCc=
-----END CERTIFICATE-----
`

func TestBadParse(t *testing.T) {
	_, err := Parse([]byte("I am not a valid cert"))

	require.Equal(t, ErrFailedToParsePEM, err)
}

func TestBadVerify(t *testing.T) {
	bundle, err := Parse([]byte(testCertificate + testKey))
	require.NoError(t, err)

	// nil bundle
	var emptyBundle *Bundle = nil
	err = emptyBundle.Verify("testhost.invalid", x509.VerifyOptions{})
	require.Equal(t, ErrBundleNil, err)

	// CA signed by unknown authority
	testTime, _ := time.Parse(time.RFC3339, "2025-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Error(t, err)

	// Invalid hostname
	testTime, _ = time.Parse(time.RFC3339, "2025-02-03T12:00:00Z")
	err = bundle.Verify("invalid.testhost", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrBundleNoCertForHost, err)

	// Not yet valid
	testTime, _ = time.Parse(time.RFC3339, "2022-02-01T12:00:00Z")
	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrCertNotYetValid, err)

	// Expired
	testTime, _ = time.Parse(time.RFC3339, "2047-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrCertExpired, err)
}

func TestCompare(t *testing.T) {
	bundle, err := Parse([]byte(testCertificate))
	require.NoError(t, err)

	bundleDupe, err := Parse([]byte(testCertificate))
	require.NoError(t, err)

	require.True(t, bundle.Equals(bundleDupe))

	bundleLE, err := Parse([]byte(letsEncryptX1))
	require.NoError(t, err)

	require.False(t, bundle.Equals(bundleLE))

	bundleWithChain, err := Parse([]byte(testCertificate + letsEncryptX1))
	require.NoError(t, err)

	require.False(t, bundleWithChain.Equals(bundleLE))

	bundleWithChainDupe, err := Parse([]byte(testCertificate + letsEncryptX1))
	require.NoError(t, err)

	require.True(t, bundleWithChain.Equals(bundleWithChainDupe))
}

func TestGoodParse(t *testing.T) {
	_, err := Parse([]byte(testCertificate + testKey))
	require.NoError(t, err)
}

func TestGoodVerify(t *testing.T) {
	bundle, err := Parse([]byte(testCertificate + testKey))
	require.NoError(t, err)

	cp := x509.NewCertPool()
	cp.AppendCertsFromPEM([]byte(rootCA))

	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{Roots: cp})
	require.NoError(t, err)
}

func TestMissingDataVerify(t *testing.T) {
	bundle, err := Parse([]byte(testCertificate))
	require.NoError(t, err)

	testTime, _ := time.Parse(time.RFC3339, "2022-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrBundleNoPrivKey, err)

	bundle, err = Parse([]byte(testKey))
	require.NoError(t, err)

	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrBundleNoCerts, err)
}

func TestMultipleKeysParse(t *testing.T) {
	_, err := Parse([]byte(testKey + testKey))
	require.Equal(t, ErrMultiplePrivKeys, err)
}
