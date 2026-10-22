package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"strconv"
)

// Decimal preserves config and journal amounts exactly, including JSON numeric inputs.
// All calculations use integer droplets (NESS), satoshis (EMC), or whole hours.
type Decimal string

var decimalSyntax = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]{1,2})?$`)

func (d *Decimal) UnmarshalJSON(b []byte) error {
	if len(b) > 128 {
		return errors.New("amount too long")
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if e := json.Unmarshal(b, &s); e != nil {
			return e
		}
		*d = Decimal(s)
	} else {
		if !json.Valid(b) || bytes.Equal(b, []byte("null")) {
			return errors.New("invalid amount")
		}
		*d = Decimal(b)
	}
	if !decimalSyntax.MatchString(string(*d)) {
		return errors.New("invalid decimal amount")
	}
	return nil
}
func (d Decimal) units(decimals int) (uint64, error) {
	if len(d) > 128 || !decimalSyntax.MatchString(string(d)) {
		return 0, errors.New("amount too long")
	}
	r, ok := new(big.Rat).SetString(string(d))
	if !ok || r.Sign() < 0 {
		return 0, errors.New("invalid amount")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	r.Mul(r, new(big.Rat).SetInt(scale))
	if !r.IsInt() || !r.Num().IsUint64() || r.Num().Uint64() > math.MaxInt64 {
		return 0, errors.New("amount exceeds precision or supported range")
	}
	return r.Num().Uint64(), nil
}
func scaledDecimal(units uint64, decimals int) Decimal {
	scale := uint64(1)
	for i := 0; i < decimals; i++ {
		scale *= 10
	}
	if decimals == 0 {
		return Decimal(strconv.FormatUint(units, 10))
	}
	return Decimal(fmt.Sprintf("%d.%0*d", units/scale, decimals, units%scale))
}
func multiply(a, b uint64) (uint64, error) {
	if b != 0 && a > math.MaxInt64/b {
		return 0, errors.New("amount overflow")
	}
	return a * b, nil
}
func add(a, b uint64) (uint64, error) {
	if math.MaxUint64-a < b {
		return 0, errors.New("amount overflow")
	}
	return a + b, nil
}
func (a *App) reward(seconds int64) (Decimal, uint64, error) {
	if seconds <= 0 {
		return "", 0, errors.New("reward is zero")
	}
	rate, e := a.cfg.CoinsPerSecond.units(6)
	if e != nil {
		return "", 0, e
	}
	coins, e := multiply(rate, uint64(seconds))
	if e != nil {
		return "", 0, e
	}
	hours, e := multiply(uint64(a.cfg.HoursPerSecond), uint64(seconds))
	return scaledDecimal(coins, 6), hours, e
}

// The minimum is 1/20 = 5% of input hours, rounded UP to a whole hour.
func requiredNESSFee(hours uint64) uint64 {
	fee := hours / 20
	if hours%20 != 0 {
		fee++
	}
	return fee
}
