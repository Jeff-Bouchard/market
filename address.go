package main

import (
	"bytes"
	"crypto/sha256"
	"math/big"
	"strings"
)

const base58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

func decodeBase58(s string) []byte {
	if len(s) < 20 || len(s) > 40 {
		return nil
	}
	n := new(big.Int)
	base := big.NewInt(58)
	for _, c := range s {
		i := strings.IndexRune(base58Alphabet, c)
		if i < 0 {
			return nil
		}
		n.Mul(n, base)
		n.Add(n, big.NewInt(int64(i)))
	}
	leading := 0
	for leading < len(s) && s[leading] == '1' {
		leading++
	}
	return append(make([]byte, leading), n.Bytes()...)
}
func encodeBase58(b []byte) string {
	n := new(big.Int).SetBytes(b)
	base := big.NewInt(58)
	r := new(big.Int)
	result := []byte{}
	for n.Sign() > 0 {
		n.QuoRem(n, base, r)
		result = append(result, base58Alphabet[r.Int64()])
	}
	for _, v := range b {
		if v != 0 {
			break
		}
		result = append(result, '1')
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return string(result)
}

// NESS uses Skycoin's key[20] + version[1] + SHA256 checksum[4].
// It is NOT Bitcoin's version-first, double-SHA256 Base58Check format.
func validNESSAddress(s string) bool {
	b := decodeBase58(s)
	if len(b) != 25 || b[20] != 0 || exampleAddresses[s] || bytes.Equal(b[:20], make([]byte, 20)) {
		return false
	}
	sum := sha256.Sum256(b[:21])
	return bytes.Equal(sum[:4], b[21:]) && encodeBase58(b) == s
}

// EMC mainnet P2PKH=33 and P2SH=92; do not apply NESS checks to EMC addresses.
func validEMCAddress(s string) bool {
	b := decodeBase58(s)
	if len(b) != 25 || (b[0] != 33 && b[0] != 92) || exampleAddresses[s] {
		return false
	}
	one := sha256.Sum256(b[:21])
	two := sha256.Sum256(one[:])
	return bytes.Equal(two[:4], b[21:]) && encodeBase58(b) == s
}
