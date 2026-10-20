package facturoporti

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"hash"
	"strings"
	"time"
)

// CSD is Cladex's Certificado de Sello Digital as Facturoporti wants it with every stamp:
// the certificate and the still-encrypted private key, as the SAT issues them, and the
// key's password. Parsed from them: the RFC it belongs to, its number and validity.
type CSD struct {
	Cert     []byte // the .cer, DER
	Key      []byte // the .key, encrypted PKCS#8 DER
	Password string

	RFC       string
	Number    string // noCertificado, the 20 digits the SAT prints on every CFDI
	NotBefore time.Time
	NotAfter  time.Time
}

// oidUniqueIdentifier is x500UniqueIdentifier, where SAT certificates carry the
// holder's RFC ("EKU9003173C9 / VADA800927DJ3" for a persona moral: its RFC, then its
// legal representative's).
var oidUniqueIdentifier = asn1.ObjectIdentifier{2, 5, 4, 45}

// ParseCSD checks that cert, key and password belong together: the certificate parses,
// the password opens the key, and the key is the certificate's. It does not judge the
// validity dates; see CSD.CheckValidAt.
func ParseCSD(cert, key []byte, password string) (*CSD, error) {
	c, err := x509.ParseCertificate(cert)
	if err != nil {
		return nil, fmt.Errorf("el certificado (.cer) no se puede leer: %w", err)
	}
	priv, err := decryptPKCS8(key, password)
	if err != nil {
		return nil, err
	}
	pub, ok := priv.(interface{ Public() crypto.PublicKey })
	if !ok {
		return nil, errors.New("la llave privada (.key) no es de un tipo conocido")
	}
	if eq, ok := pub.Public().(interface{ Equal(crypto.PublicKey) bool }); !ok || !eq.Equal(c.PublicKey) {
		return nil, errors.New("la llave privada (.key) no corresponde al certificado (.cer)")
	}
	return &CSD{
		Cert:      cert,
		Key:       key,
		Password:  password,
		RFC:       certRFC(c.Subject),
		Number:    certNumber(c),
		NotBefore: c.NotBefore,
		NotAfter:  c.NotAfter,
	}, nil
}

// CheckValidAt reports an error when the CSD is not valid at t.
func (c *CSD) CheckValidAt(t time.Time) error {
	if t.Before(c.NotBefore) {
		return fmt.Errorf("el certificado es válido hasta el %s", c.NotBefore.Format("2006-01-02"))
	}
	if t.After(c.NotAfter) {
		return fmt.Errorf("el certificado venció el %s", c.NotAfter.Format("2006-01-02"))
	}
	return nil
}

func certRFC(subject pkix.Name) string {
	for _, a := range subject.Names {
		if a.Type.Equal(oidUniqueIdentifier) {
			if s, ok := a.Value.(string); ok {
				rfc, _, _ := strings.Cut(s, "/")
				return strings.TrimSpace(rfc)
			}
		}
	}
	return ""
}

// certNumber is the certificate's serial as the SAT writes it: the serial's bytes are
// ASCII digits ("30001000000500003416").
func certNumber(c *x509.Certificate) string {
	b := c.SerialNumber.Bytes()
	for _, ch := range b {
		if ch < '0' || ch > '9' {
			return c.SerialNumber.Text(16)
		}
	}
	return string(b)
}

// The PKCS#5 v2 structures the SAT's .key files use (RFC 8018): PBES2 with PBKDF2 and
// 3DES-CBC, or AES-CBC in newer tools.
var (
	oidPBES2      = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 13}
	oidPBKDF2     = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 5, 12}
	oidHMACSHA1   = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 7}
	oidHMACSHA256 = asn1.ObjectIdentifier{1, 2, 840, 113549, 2, 9}
	oidDESEDE3CBC = asn1.ObjectIdentifier{1, 2, 840, 113549, 3, 7}
	oidAES128CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 2}
	oidAES256CBC  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 1, 42}
)

type encryptedPrivateKeyInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Data      []byte
}

type pbes2Params struct {
	KDF    pkix.AlgorithmIdentifier
	Scheme pkix.AlgorithmIdentifier
}

type pbkdf2Params struct {
	Salt       []byte
	Iterations int
	KeyLength  int                      `asn1:"optional"`
	PRF        pkix.AlgorithmIdentifier `asn1:"optional"`
}

func decryptPKCS8(der []byte, password string) (any, error) {
	var info encryptedPrivateKeyInfo
	if rest, err := asn1.Unmarshal(der, &info); err != nil || len(rest) > 0 {
		return nil, errors.New("la llave privada (.key) no se puede leer")
	}
	if !info.Algorithm.Algorithm.Equal(oidPBES2) {
		return nil, errors.New("la llave privada (.key) usa un cifrado desconocido")
	}
	var params pbes2Params
	if _, err := asn1.Unmarshal(info.Algorithm.Parameters.FullBytes, &params); err != nil || !params.KDF.Algorithm.Equal(oidPBKDF2) {
		return nil, errors.New("la llave privada (.key) usa un cifrado desconocido")
	}
	var kdf pbkdf2Params
	if _, err := asn1.Unmarshal(params.KDF.Parameters.FullBytes, &kdf); err != nil {
		return nil, errors.New("la llave privada (.key) no se puede leer")
	}
	var prf func() hash.Hash
	switch {
	case len(kdf.PRF.Algorithm) == 0 || kdf.PRF.Algorithm.Equal(oidHMACSHA1):
		prf = sha1.New
	case kdf.PRF.Algorithm.Equal(oidHMACSHA256):
		prf = sha256.New
	default:
		return nil, errors.New("la llave privada (.key) usa un cifrado desconocido")
	}
	var iv []byte
	if _, err := asn1.Unmarshal(params.Scheme.Parameters.FullBytes, &iv); err != nil {
		return nil, errors.New("la llave privada (.key) no se puede leer")
	}
	var keyLen int
	var newCipher func([]byte) (cipher.Block, error)
	switch {
	case params.Scheme.Algorithm.Equal(oidDESEDE3CBC):
		keyLen, newCipher = 24, des.NewTripleDESCipher
	case params.Scheme.Algorithm.Equal(oidAES128CBC):
		keyLen, newCipher = 16, aes.NewCipher
	case params.Scheme.Algorithm.Equal(oidAES256CBC):
		keyLen, newCipher = 32, aes.NewCipher
	default:
		return nil, errors.New("la llave privada (.key) usa un cifrado desconocido")
	}
	k, err := pbkdf2.Key(prf, password, kdf.Salt, kdf.Iterations, keyLen)
	if err != nil {
		return nil, fmt.Errorf("la llave privada (.key) no se puede abrir: %w", err)
	}
	block, err := newCipher(k)
	if err != nil || len(iv) != block.BlockSize() || len(info.Data) == 0 || len(info.Data)%block.BlockSize() != 0 {
		return nil, errors.New("la llave privada (.key) no se puede leer")
	}
	plain := make([]byte, len(info.Data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, info.Data)
	// A wrong password shows up as bad padding or as bytes that aren't a key.
	wrong := errors.New("la contraseña no abre la llave privada (.key)")
	pad := int(plain[len(plain)-1])
	if pad == 0 || pad > block.BlockSize() || !bytes.Equal(plain[len(plain)-pad:], bytes.Repeat([]byte{byte(pad)}, pad)) {
		return nil, wrong
	}
	priv, err := x509.ParsePKCS8PrivateKey(plain[:len(plain)-pad])
	if err != nil {
		return nil, wrong
	}
	return priv, nil
}
