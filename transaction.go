package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"strconv"
)

type createdOutput struct {
	Address string  `json:"address"`
	Coins   Decimal `json:"coins"`
	Hours   string  `json:"hours"`
}
type createdInput struct {
	UxID    string  `json:"uxid"`
	Address string  `json:"address"`
	Coins   Decimal `json:"coins"`
	Hours   string  `json:"calculated_hours"`
}
type createdTransaction struct {
	Encoded     string `json:"encoded_transaction"`
	Transaction struct {
		Txid    string          `json:"txid"`
		Fee     string          `json:"fee"`
		Inputs  []createdInput  `json:"inputs"`
		Outputs []createdOutput `json:"outputs"`
	} `json:"transaction"`
}
type decodedOutput struct {
	Address      string
	Coins, Hours uint64
}
type decodedTransaction struct {
	ID      string
	Inputs  []string
	Outputs []decodedOutput
}

// Mirrors the current Skycoin encoder wire layout, using only Go's standard library.
// The node remains responsible for secp256k1 signatures and UTXO consensus validation.
func decodeTransaction(encoded string) (decodedTransaction, error) {
	return decodeTransactionMode(encoded, true)
}
func decodeTransactionMode(encoded string, signed bool) (decodedTransaction, error) {
	var result decodedTransaction
	b, e := hex.DecodeString(encoded)
	if e != nil || len(b) < 49 || len(b) > 32768 {
		return result, errors.New("invalid encoded transaction size")
	}
	if int(binary.LittleEndian.Uint32(b[:4])) != len(b) || b[4] != 0 {
		return result, errors.New("invalid transaction header")
	}
	hash := sha256.Sum256(b)
	result.ID = hex.EncodeToString(hash[:])
	pos := 37
	count := func(size int) ([]byte, int, error) {
		if pos+4 > len(b) {
			return nil, 0, errors.New("truncated transaction")
		}
		n := int(binary.LittleEndian.Uint32(b[pos : pos+4]))
		pos += 4
		if n == 0 || n > 65535 || n > (len(b)-pos)/size {
			return nil, 0, errors.New("invalid transaction array")
		}
		raw := b[pos : pos+n*size]
		pos += n * size
		return raw, n, nil
	}
	sigs, ns, e := count(65)
	if e != nil {
		return result, e
	}
	for i := 0; i < ns; i++ {
		if bytes.Equal(sigs[i*65:(i+1)*65], make([]byte, 65)) == signed {
			return result, errors.New("unexpected transaction signature state")
		}
	}
	innerStart := pos
	inputs, ni, e := count(32)
	if e != nil || ni != ns {
		return result, errors.New("invalid transaction inputs")
	}
	seen := map[string]bool{}
	for i := 0; i < ni; i++ {
		id := hex.EncodeToString(inputs[i*32 : (i+1)*32])
		if seen[id] {
			return result, errors.New("duplicate input")
		}
		seen[id] = true
		result.Inputs = append(result.Inputs, id)
	}
	outputs, no, e := count(37)
	if e != nil {
		return result, e
	}
	if pos != len(b) {
		return result, errors.New("trailing transaction bytes")
	}
	inner := sha256.Sum256(b[innerStart:])
	if !bytes.Equal(inner[:], b[5:37]) {
		return result, errors.New("transaction inner hash mismatch")
	}
	for i := 0; i < no; i++ {
		o := outputs[i*37 : (i+1)*37]
		addressBytes := append(append([]byte{}, o[1:21]...), o[0])
		sum := sha256.Sum256(addressBytes)
		addressBytes = append(addressBytes, sum[:4]...)
		address := encodeBase58(addressBytes)
		coins := binary.LittleEndian.Uint64(o[21:29])
		hours := binary.LittleEndian.Uint64(o[29:37])
		if !validNESSAddress(address) || coins == 0 {
			return result, errors.New("invalid transaction output")
		}
		result.Outputs = append(result.Outputs, decodedOutput{address, coins, hours})
	}
	return result, nil
}
func validateCreated(tx createdTransaction, address string, coins, hours uint64, owned map[string]bool) error {
	return validateCreatedMode(tx, address, coins, hours, owned, true)
}
func validateCreatedMode(tx createdTransaction, address string, coins, hours uint64, owned map[string]bool, signed bool) error {
	parsed, e := decodeTransactionMode(tx.Encoded, signed)
	if e != nil {
		return e
	}
	if tx.Transaction.Txid != parsed.ID || len(parsed.Inputs) != len(tx.Transaction.Inputs) || len(parsed.Outputs) != len(tx.Transaction.Outputs) {
		return errors.New("transaction description does not match encoded transaction")
	}
	var inCoins, inHours, outCoins, outHours, recipientCoins, recipientHours uint64
	for i, in := range tx.Transaction.Inputs {
		if in.UxID != parsed.Inputs[i] {
			return errors.New("input description mismatch")
		}
		c, e := in.Coins.units(6)
		if e != nil {
			return e
		}
		h, e := strconv.ParseUint(in.Hours, 10, 64)
		if e != nil {
			return e
		}
		inCoins, e = add(inCoins, c)
		if e != nil {
			return e
		}
		inHours, e = add(inHours, h)
		if e != nil {
			return e
		}
	}
	for i, o := range parsed.Outputs {
		desc := tx.Transaction.Outputs[i]
		c, e := desc.Coins.units(6)
		if e != nil {
			return e
		}
		h, e := strconv.ParseUint(desc.Hours, 10, 64)
		if e != nil || desc.Address != o.Address || c != o.Coins || h != o.Hours {
			return errors.New("output description mismatch")
		}
		if o.Address == address {
			recipientCoins, e = add(recipientCoins, o.Coins)
			if e != nil {
				return e
			}
			recipientHours, e = add(recipientHours, o.Hours)
			if e != nil {
				return e
			}
		} else if !owned[o.Address] {
			return errors.New("change output is not in the payout wallet")
		}
		outCoins, e = add(outCoins, o.Coins)
		if e != nil {
			return e
		}
		outHours, e = add(outHours, o.Hours)
		if e != nil {
			return e
		}
	}
	if recipientCoins != coins || recipientHours != hours {
		return errors.New("transaction does not pay the exact quoted amount")
	}
	if outCoins != inCoins || outHours > inHours {
		return errors.New("transaction totals do not balance")
	}
	fee, e := strconv.ParseUint(tx.Transaction.Fee, 10, 64)
	if e != nil || fee == 0 || fee != inHours-outHours || fee < requiredNESSFee(inHours) {
		return errors.New("transaction does not meet NESS minimum burn: ceil(input hours / 20)")
	}
	return nil
}
