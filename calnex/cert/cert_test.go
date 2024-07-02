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
//	openssl req -x509 -nodes -sha256 -new -key "root.key" -out "root.crt" -days 731 \
//	  -subj "/CN=Custom Root" \
//	  -addext "keyUsage = critical, keyCertSign" \
//	  -addext "basicConstraints = critical, CA:TRUE, pathlen:0" \
//	  -addext "subjectKeyIdentifier = hash"
const rootCA = `-----BEGIN CERTIFICATE-----
MIIEIDCCAoigAwIBAgIUH6gyij3LwI01yP1WRsITIm4op7YwDQYJKoZIhvcNAQEL
BQAwFjEUMBIGA1UEAwwLQ3VzdG9tIFJvb3QwHhcNMjQwNzAxMTMyOTQ2WhcNMjYw
NzAyMTMyOTQ2WjAWMRQwEgYDVQQDDAtDdXN0b20gUm9vdDCCAaIwDQYJKoZIhvcN
AQEBBQADggGPADCCAYoCggGBAL7TbX59+OpBLmzs3orQbBcP1uN9qzaoMRBaJuVj
f1hn8jhdip2pNnZTyTmVgNDvur3v5AzaQQK1TxHyCbrmoe4I6/tV5YShlXmFx24S
gXGyVfzshnnNSoIup9knT18VTbYUFfA8CAVIafTlNB20fN+YDv7Ah/SjQ+ZZn+IO
m+1i9yeJ0aVwzSIvz2re3ei9OoN78aQXoF3Xk833XWiftCCGxZtbTP94sMGn4WUU
4SmJMaQEIEXD3gLaP+CMOAdEw2FJdJ1OQV5IShaOAq2qhKAwCNWMt3WUaYAWeOhf
sGGyzWgdsoQ9cze/l3lMk6mHhSOh/nktaUPntSJaGucZjLKtfu6vXn04sepdpsSV
HgKEGXXcPMN9qStlaJf010pnuKYU556K7IhGCEHPQ/WpxaN468cDZOxpE0XgsMe7
llvmqhFxvMVvnFpTcbjmW+DgdhaHG9kicVmi012b7AdQ7H6Z3X2psSMgzONhIRcV
1S38tVTMowTVrHE6fIDapGTkjwIDAQABo2YwZDAfBgNVHSMEGDAWgBQ717pUZqNd
eylniXMuPxwOL1gvUDAOBgNVHQ8BAf8EBAMCAgQwEgYDVR0TAQH/BAgwBgEB/wIB
ADAdBgNVHQ4EFgQUO9e6VGajXXspZ4lzLj8cDi9YL1AwDQYJKoZIhvcNAQELBQAD
ggGBAKOIW9W0slLA8Ib3lU/WdwhDaSWQe0VdDFHnMB77i59Xf5j/npSMEwna2N1T
1IfLiFEpf6WXSounPJH2+Sy7+030MrthSqCrGriuxV3c3VE6/tFW4WsQ9hu5z8V9
ZuaS5W96jBaIViDPDybvZ+kLurRL+c0x23EqxrxAy+8HcO+QScREf1nGB2EYNv+n
kEaPrj/XPXeh7y2oSutrEr9QzREbXSJj2eeNKR4LrlqOd4Z71UTYAktfVY+EM0uB
8PGqihkTU2uNdPKUD6nBOdFM0OfZSL/mOtsBFkpz+eM+aMDX21hE7zxZ+ap5870X
s47ClO3BDs16Rvpcby0UmT7bt+mkwuRxAm6RcZScYD6Ym7wyAS+YGb/WOwXZUUL7
eP6DjPCyn1NBPBhssB9Ll5xZ+ALfQiaOiufkkO2bGlqmCt9Tlvg5alPL42m3HMNh
q/rA1ZVNsR6wtE/GSbW2vCtF07P24HQBsfKm1ZII0MeZKWlfEyLSKZ87w84wZvhv
pZKEFw==
-----END CERTIFICATE-----
`

// Test certificate generated with:
//
// openssl genrsa -out "testhost.invalid.key" 2048
// openssl req -sha256 -new -key "testhost.invalid.key" -out "testhost.invalid.csr" \
// -subj "/CN=*.testhost.invalid" \
// -reqexts SAN -config <(echo "[SAN]\nsubjectAltName=DNS:testhost.invalid,DNS:*.testhost.invalid\n")
//
// openssl x509 -req -sha256 -in "testhost.invalid.csr" -out "testhost.invalid.crt" -days 731 \
//   -CAkey "root.key" -CA "root.crt" -CAcreateserial -extfile <(cat <<END
//     subjectAltName = DNS:testhost.invalid,DNS:*.testhost.invalid
//     keyUsage = critical, digitalSignature, keyEncipherment
//     extendedKeyUsage = serverAuth
//     basicConstraints = CA:FALSE
//     authorityKeyIdentifier = keyid:always
//     subjectKeyIdentifier = none
// END
// )
// Validity
// Not Before: Jul  1 13:29:47 2024 GMT
// Not After : Jul  2 13:29:47 2026 GMT

const testCertificate = `-----BEGIN CERTIFICATE-----
MIID3zCCAkegAwIBAgIUBATsW3bbqNNSzh4TU+FiOHl8QQUwDQYJKoZIhvcNAQEL
BQAwFjEUMBIGA1UEAwwLQ3VzdG9tIFJvb3QwHhcNMjQwNzAxMTMyOTQ3WhcNMjYw
NzAyMTMyOTQ3WjA1MRswGQYDVQQDDBIqLnRlc3Rob3N0LmludmFsaWQxFjAUBgNV
BAoMDU1ldGEgSUROIFRlc3QwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIB
AQDN9nk7srvl66/a1VUQ7A6yyYQdnKaKOMWHfWZS2Sl+ZDJqiCAm+LYX3b1ashG6
QM80TC/bKEqQujSFSaPAqQXj/9v3ETMibYDPbjk8iCAx7VEEy67aJ+ng93raGpUm
yFJOF+h0sHpbbqmyIM4PVlhUMmshUkTKZyQNWjicEp7wxpH1l7xO0ShRg2EtZnrr
hK6ukYzlTfAaAHWeUslVV6ppuZy7gobXI7dNyWIk/9CGjF/iqyNuGf/DrExPY2kE
v9LX84sLsYqn1W53u3QftMSWCTcqTAW5KS93RM5FSHAntBPGNXNGVrQJ1XOfpQhp
bviM4c5BgOGNJl88RONuOqo/AgMBAAGjgYUwgYIwLwYDVR0RBCgwJoIQdGVzdGhv
c3QuaW52YWxpZIISKi50ZXN0aG9zdC5pbnZhbGlkMA4GA1UdDwEB/wQEAwIFoDAT
BgNVHSUEDDAKBggrBgEFBQcDATAJBgNVHRMEAjAAMB8GA1UdIwQYMBaAFDvXulRm
o117KWeJcy4/HA4vWC9QMA0GCSqGSIb3DQEBCwUAA4IBgQBY8QiImao9+CXt4Rwl
TgnI2n8E7FiEg/nQhTOvgWqQesTeWNz5ctyqU+XZC5R1Nb0vEiKrC1RSLy7tZ7E3
zpVdAb6lhKKkpgW7bwYBx5fu4JoqAzlD8flibEr8/jlasRUYlT4nmpflmG9CMwjV
7PS+E2Vy2uBEJfZBUIRmUECRkRsNiLx8jNLpfAClIO1qzyIwLz66PjPtwwKBa/uA
gWnsc4uHNeMV2YNO8Sg6ULV3infFnrG3LSJLXGCP+O6HG3Da+kwNMaskJfXhP9q7
gdzmC+qaoLkpv5DNBh9LQX0QKi1zXNHammZJ9LhafRkpzyt9z03b3gaA8HaLSTsR
3aBpI7SnM6gLdhhMZf4q7jPvEOa/zenHPtA7D4n086C46Aao5xe938m9x8+0lmnn
1LjCIc7ze3M42pLhwJUjtzX0W5PBk9UphbsQObO30DN3AAMpEAugQ5W334Goz41u
r1GkDF/44gf4QxzJOGZuWPoKBr1Bcqkbn7C4BuJ2mbC68MU=
-----END CERTIFICATE-----
`

// @lint-ignore PRIVATEKEY insecure-private-key-storage
const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvwIBADANBgkqhkiG9w0BAQEFAASCBKkwggSlAgEAAoIBAQDN9nk7srvl66/a
1VUQ7A6yyYQdnKaKOMWHfWZS2Sl+ZDJqiCAm+LYX3b1ashG6QM80TC/bKEqQujSF
SaPAqQXj/9v3ETMibYDPbjk8iCAx7VEEy67aJ+ng93raGpUmyFJOF+h0sHpbbqmy
IM4PVlhUMmshUkTKZyQNWjicEp7wxpH1l7xO0ShRg2EtZnrrhK6ukYzlTfAaAHWe
UslVV6ppuZy7gobXI7dNyWIk/9CGjF/iqyNuGf/DrExPY2kEv9LX84sLsYqn1W53
u3QftMSWCTcqTAW5KS93RM5FSHAntBPGNXNGVrQJ1XOfpQhpbviM4c5BgOGNJl88
RONuOqo/AgMBAAECggEAXpBNtVUo5DXENgtA1VYsoXXYjOgBpvDN8Jlow50lafyD
EVqSuJH0uRx79gpQDV34RKC+UDc9lRmJR7E52BlCtR4iVlu1SJdSTuriqKIvdfzp
9/O0wkEVJs85vq350Sakc2qSthDY/OXgUAKz2WLhhzbm7ROitfOJIABOgYojI5SV
E4aPDrxCGFMzyBnGhS2Q9m6nrT4PmbnnbIT2nZzBT9FYWmSC9FKmej5IS3AKGLwB
9ZpuWV1iqZOIgvDhRWntVr24Gn0O5EMTF7RynTV2QlbWuE8zCD1kSjnuicxRGAqf
OeMbeClkU9jOsVeyJmAzIlwAfs3YbFEz1yMKF4D90QKBgQDa/yLaJmMx/7sxNt+4
qATWjGMJO8SYggrqeFqqJU+gMGInsyGN6UlLko+kaSQF4kBbceDHb6o2XXlPtK3P
JFuEQQiV/CbztinyB0UNuJpB06dtfEW22O1cJqrgS5M/xwj1vIE4CRHZKQYzZQ9C
iXnnfXp0AiOZ2IuaXImrZoCwcwKBgQDww4yHudV2DP5qTGhy4LBfIq2rzY+QDaBa
g5pWzomJt5eJWgMruwxFEHNIjLSXb2lYq9TTCXKyNzwBF6kGVOM4WCwV+/JOGx7F
vGg5KUBKPC/9uIYikm9Gs3XMhmcUZqBV7g0B4soiWalwBkI7BtoeObqaSZZ+QZUI
jyemIjXoBQKBgQCltWTz2RQ6Ix3MEY+btFdk2PmfZQBPvibwYH2KPY1Q0wuSqrL7
JMj3TEEw0PYXFapJB5RklJQhav1+WGMkWIh/PI54n0ICK5b1spaH2WWv5a3M5LoD
r4V7sy6dZdJX8g1PlIHamtJMlgRBI3k2ibwadBIScgPqR7bq6JargXZjDQKBgQCU
avemc5hzPW9Yd+Grb3dKLkaBMibd1oiTQ61Q9eEzVEnGEgcCXjwiFxH6F0L8V2HJ
l6OKtLhPxFzpD3zSumGXykLjCn1ESNOfcZWOJy/Kk2/CKI4Hod2W5+omOnQwz1Ln
pee+0d9pbXxV4oXRfVfYah3uHo73JdaJgDYg49X3QQKBgQCan9EfdksF8OVi+5PP
7gktyg+giMFWnNFFE/Gi9VbyHRSQB/pU9LpdwzlPUSe6QQuKdL93kgWXYgVf3cwF
zKvIA65gXxPrpkPwWeuzjMIA9pwW9IJudsof6kaZ7NK+T6gBbt91WLVi+zWwoxci
p89kXDk+P1McDjmisyRqgRV49A==
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
	require.ErrorAs(t, err, &x509.UnknownAuthorityError{})

	// Invalid hostname
	testTime, _ = time.Parse(time.RFC3339, "2025-02-03T12:00:00Z")
	err = bundle.Verify("invalid.testhost", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrBundleNoCertForHost, err)

	// Not yet valid
	testTime, _ = time.Parse(time.RFC3339, "2022-02-01T12:00:00Z")
	err = bundle.Verify("testhost.invalid", x509.VerifyOptions{CurrentTime: testTime})
	require.Equal(t, ErrCertNotYetValid, err)

	// Expired
	testTime, _ = time.Parse(time.RFC3339, "2027-02-03T12:00:00Z")
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
