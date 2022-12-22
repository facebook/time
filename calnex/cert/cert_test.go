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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Test certificate generated with:
//
// $ openssl req -x509 -nodes -newkey rsa:2048 -keyout privkey.pem -out cert.pem -sha256 -days 365 \
//	-subj '/CN=testhost.invalid' -reqexts SAN -extensions SAN \
//	-config <(printf '[req]\ndistinguished_name = dn\n[dn]\nCN=testhost.invalid\n[SAN]\nsubjectAltName=DNS:testhost.invalid')
//
// Validity
//   Not Before: Feb  3 11:25:46 2022 GMT
//   Not After : Feb  3 11:25:46 2023 GMT

const testCertificate = `-----BEGIN CERTIFICATE-----
MIIC4zCCAcugAwIBAgIUdGVC+4vEBWjB87q9k/NLtwF2wGswDQYJKoZIhvcNAQEL
BQAwGzEZMBcGA1UEAwwQdGVzdGhvc3QuaW52YWxpZDAeFw0yMjAyMDMxMTI1NDZa
Fw0yMzAyMDMxMTI1NDZaMBsxGTAXBgNVBAMMEHRlc3Rob3N0LmludmFsaWQwggEi
MA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC6CnsEElO640Xk9H7SPHPHXXQS
9Oo4pGf3uNMc9qXIA8PgNktskXMTh020vtV1d2RMRjbAvUmfK8RPsy7OyZvFMFv0
ncppC8BLrx2by7e60XXRlH6mpqA3X5H6y2UDaRhy4rl5xXn+Ppn+SjJTP91gYsDH
ryau4vgxM4QnHAb8PuDenTZdlHHLZxrPTET4NOll1SmDB/qPGva4/eZMGdosS7FW
KqyE1Loh4dygERM7Xrwlu2hNTJHd4BviKRF9wWV52iMv4uTIaSJHAGVd2YVg5fG+
tBQqqd9snLSydq8aay1wrr2DEo9fsFHR+kJUD9vcICdog/ge0eV4C9Gdrf6XAgMB
AAGjHzAdMBsGA1UdEQQUMBKCEHRlc3Rob3N0LmludmFsaWQwDQYJKoZIhvcNAQEL
BQADggEBABnfLyaYMNjJ7CqK3BwRYaHIYnVyKF8gnyzQzeH0jtGWyNe1rtmZLNiB
cDmK7BCNXfdZrAMBfNC3ku/wuISlqN3IWpdU0IBQiRLx2aZDMjTW+Tn1vjEA8bfy
lkye6unj1dXBjXt1sI4NhXgBsAvNISg7dh7AK9/rHefxvMXW3PLozvkwWV+lZjhe
u3CyT1d1MZdJzqts1t0eJTju1uZgsWp0SMXivabV6XiHQQQaYLSYsSMLxZxoAgOQ
Ml59Q4pF/AOnbPpnv0wdPYWXlGaFWSZ1d3Ch67ALbDIp6IA2NjSCcYORltaF+Ogr
kgEAiwyM3UboyEtLXDPk9VwqzCglAj0=
-----END CERTIFICATE-----
`

// @lint-ignore PRIVATEKEY insecure-private-key-storage
const testKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQC6CnsEElO640Xk
9H7SPHPHXXQS9Oo4pGf3uNMc9qXIA8PgNktskXMTh020vtV1d2RMRjbAvUmfK8RP
sy7OyZvFMFv0ncppC8BLrx2by7e60XXRlH6mpqA3X5H6y2UDaRhy4rl5xXn+Ppn+
SjJTP91gYsDHryau4vgxM4QnHAb8PuDenTZdlHHLZxrPTET4NOll1SmDB/qPGva4
/eZMGdosS7FWKqyE1Loh4dygERM7Xrwlu2hNTJHd4BviKRF9wWV52iMv4uTIaSJH
AGVd2YVg5fG+tBQqqd9snLSydq8aay1wrr2DEo9fsFHR+kJUD9vcICdog/ge0eV4
C9Gdrf6XAgMBAAECggEAUUrVGCFd/vLijrobVIhf2wTF/KaSVi/Y0lErxqMsK6sh
gy6WZJll3GmqFcmxgoOqCv4/XJcZvXilbmIQmQFVlKOd+tScJqyg2TFq0bIB1ZtD
TVICyZVTuv6CzkDkIcphiYnym/gjZ2o5ZflL5j6o4D4mmNq7H35ED1PAckp37u5X
d/MmWeE+axoOgaU2A7bcMdT8EkYBdeOb+7WTkel/F8tbPKc8rpp4fw5zsRFOREqy
EyaeUj2rsqhBOcPhFelaXAQ7AaF2U1BeEnLZzRRUIhdTA76NsE5varmDeoBdDIlf
EacAPXzQuqYsHjlzDXQuMZ44sVhrvTLH0vKrFbMUAQKBgQDtMAjSVgIlyCaELJmv
/E3HnNYXAEpkpgiNppeW/WNn141h1nIxiOU67Jyu7QXMfUssjLCFUfoe7jpt2RPi
O+QPLI7IasTB5dN4hPnJDnfJy/+zpTWA1vqkR9ULAd7DSq7EyaQH3qRIRwEI0/cK
/HLLBLRiSImOtJ0sqpzzfah2AQKBgQDIy/C5Vrv2EL8l8CkmPgnM07jfFF0z083j
83/yG+Zqc+cFSstt2nuNCGKy7cbHbhGAZu42qBPhz0KGLJSb/Xb1sItOs/Mg+Vsy
CHXw0M5z7YxUC5/MuDnLpiTRw/CdIISxSR0yhcSRkS5O7vZb1YYqfZx4NFvzIYqK
bUaM4ThklwKBgQCDKjcWqj2RyzeRjGCJM8uHgbHbEmwRcMf2HZRjCUk5mbgzzLVl
s0Cg70xOaAD27qrtvfe4IndhN3jUWmFmkJwzz/490t1wJLpnQZIon3ma/Ncw70HB
OCFvS9ICvkwET36KkL/HIlZTKgDmcuGBD84jezyNxXNcmYD5vHgDJxBMAQKBgQDD
25F55u0+TgV1BvXMVJUQkq/wAJgMtptMrrXtPWOaEGWWFueoxoTfAv/q0d2jp2ww
17Wh4H5MMvMLly55nVlMuyCW6xXK4w8eFXydIb9O+rV3QUNk14mgZ/XgGgR370Ef
AFcXcb1T083ctl/dIcBVb+KQqVnLJLtS3NYFEqYEDwKBgHcvU054ijoHE/uWKePW
V4DNschu4iFvyVqgaqd/wLTBTxWIN4ig8yw3EIRnb9yB00/YakxkiDGNML/E2pGz
6WpaadzzKqu84iQGXrD0V/Wv+Di/Mhnossfaj3K1shlHatHP1LPTQwtTxK8uzxeg
1J0Y06oQ8xagjQtehln0flmO
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

	// Invalid hostname
	testTime, _ := time.Parse(time.RFC3339, "2022-02-03T12:00:00Z")
	err = bundle.Verify("invalid.testhost", testTime)
	require.Equal(t, ErrBundleNoCertForHost, err)

	// Not yet valid
	testTime, _ = time.Parse(time.RFC3339, "2022-02-01T12:00:00Z")
	err = bundle.Verify("testhost.invalid", testTime)
	require.Equal(t, ErrCertNotYetValid, err)

	// Expired
	testTime, _ = time.Parse(time.RFC3339, "2023-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", testTime)
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

	testTime, _ := time.Parse(time.RFC3339, "2022-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", testTime)
	require.NoError(t, err)
}

func TestMissingDataVerify(t *testing.T) {
	bundle, err := Parse([]byte(testCertificate))
	require.NoError(t, err)

	testTime, _ := time.Parse(time.RFC3339, "2022-02-03T12:00:00Z")
	err = bundle.Verify("testhost.invalid", testTime)
	require.Equal(t, ErrBundleNoPrivKey, err)

	bundle, err = Parse([]byte(testKey))
	require.NoError(t, err)

	err = bundle.Verify("testhost.invalid", testTime)
	require.Equal(t, ErrBundleNoCerts, err)
}

func TestMultipleKeysParse(t *testing.T) {
	_, err := Parse([]byte(testKey + testKey))
	require.Equal(t, ErrMultiplePrivKeys, err)
}
