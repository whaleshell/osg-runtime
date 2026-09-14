package proxy

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

const maxCachedLeafs = 256

// MitmCA is an ephemeral sandbox CA used for TLS terminate (HTTPS L7).
type MitmCA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	tlsCert tls.Certificate
	pem     []byte

	mu    sync.Mutex
	leafs map[string]*tls.Certificate
}

// GenerateMitmCA creates a new ephemeral CA.
func GenerateMitmCA() (*MitmCA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			Organization: []string{"osg"},
			CommonName:   "osg Sandbox Proxy CA",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &MitmCA{
		cert:    cert,
		key:     key,
		tlsCert: tlsCert,
		pem:     certPEM,
		leafs:   make(map[string]*tls.Certificate),
	}, nil
}

// CertPEM returns the CA certificate PEM.
func (c *MitmCA) CertPEM() []byte {
	if c == nil {
		return nil
	}
	return append([]byte(nil), c.pem...)
}

// WritePEM writes the CA certificate to path.
func (c *MitmCA) WritePEM(path string) error {
	if c == nil {
		return fmt.Errorf("mitm: nil ca")
	}
	return os.WriteFile(path, c.pem, 0o644)
}

// WriteBundle writes system CA roots (if found) plus the osg CA to path.
func (c *MitmCA) WriteBundle(path string) error {
	if c == nil {
		return fmt.Errorf("mitm: nil ca")
	}
	var b []byte
	for _, p := range systemCAPaths() {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		b = append(b, data...)
		if len(b) > 0 && b[len(b)-1] != '\n' {
			b = append(b, '\n')
		}
		break
	}
	b = append(b, c.pem...)
	return os.WriteFile(path, b, 0o644)
}

func systemCAPaths() []string {
	return []string{
		"/etc/ssl/certs/ca-certificates.crt",
		"/etc/pki/tls/certs/ca-bundle.crt",
		"/etc/ssl/ca-bundle.pem",
		"/etc/ssl/cert.pem",
	}
}

// Leaf returns a hostname leaf cert signed by this CA (cached).
func (c *MitmCA) Leaf(hostname string) (*tls.Certificate, error) {
	if c == nil {
		return nil, fmt.Errorf("mitm: nil ca")
	}
	host := strings.ToLower(strings.TrimSpace(hostname))
	if host == "" {
		return nil, fmt.Errorf("mitm: empty hostname")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if leaf, ok := c.leafs[host]; ok {
		return leaf, nil
	}
	leaf, err := c.generateLeaf(host)
	if err != nil {
		return nil, err
	}
	if len(c.leafs) >= maxCachedLeafs {
		c.leafs = make(map[string]*tls.Certificate)
	}
	c.leafs[host] = leaf
	return leaf, nil
}

func (c *MitmCA) generateLeaf(hostname string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hostname},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{hostname},
	}
	if ip := net.ParseIP(hostname); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
		tmpl.DNSNames = nil
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return nil, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	// Include CA in chain for clients that expect intermediates.
	certPEM = append(certPEM, c.pem...)
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}
	return &tlsCert, nil
}

// ServerTLSConfig returns a tls.Config that presents a leaf for hostname.
func (c *MitmCA) ServerTLSConfig(hostname string) (*tls.Config, error) {
	leaf, err := c.Leaf(hostname)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		Certificates: []tls.Certificate{*leaf},
		NextProtos:   []string{"http/1.1"},
		MinVersion:   tls.VersionTLS12,
	}, nil
}

// ClientTLSConfig returns a tls.Config for dialing the real upstream.
func ClientTLSConfig(serverName string) *tls.Config {
	return &tls.Config{
		ServerName: serverName,
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
	}
}

// RootPool returns a cert pool containing only this CA (for tests / clients).
func (c *MitmCA) RootPool() *x509.CertPool {
	pool := x509.NewCertPool()
	pool.AddCert(c.cert)
	return pool
}
