package main

import (
	"encoding/json"
	"errors"
	"net/url"
)

const nchPerNESS uint64 = 10000

func exchangeQuote(hours uint64) (Decimal, error) {
	if hours == 0 {
		return "", errors.New("enter a positive whole NCH amount")
	}
	// One whole NCH buys exactly 100 droplets at the fixed 10,000:1 rate.
	coins, e := multiply(hours, 1000000/nchPerNESS)
	if e != nil {
		return "", e
	}
	return scaledDecimal(coins, 6), nil
}

func (a *App) createDirectExchange(id string, in map[string]json.RawMessage) (any, int, error) {
	var amount Decimal
	if e := json.Unmarshal(in["hours"], &amount); e != nil {
		return nil, 400, bad("Enter the whole NCH amount to exchange.")
	}
	hours, e := amount.units(0)
	if e != nil {
		return nil, 400, bad("NCH amount must be a supported whole number.")
	}
	coins, e := exchangeQuote(hours)
	if e != nil {
		return nil, 400, bad(e.Error())
	}
	address, e := textField(in, "pay_address", 64)
	if e != nil || !validNESSAddress(address) {
		return nil, 400, bad("Enter your own valid NESS receiving address.")
	}
	if a.cfg.NESSURL == "" || a.cfg.NESSWallet == "" {
		return nil, 503, &apiError{503, "Configure the funded NESS wallet to enable direct exchanges."}
	}
	var last int64
	for _, other := range a.store.Slots {
		if other.Session != id {
			continue
		}
		if other.ExchangeRate == nchPerNESS && other.Status == "generated" && other.PayAddress == address && other.ExchangeHours == hours {
			return other, 200, nil
		}
		if other.Created > last {
			last = other.Created
		}
	}
	if a.now().Unix()-last < 30 {
		return nil, 429, &apiError{429, "Wait 30 seconds between new requests."}
	}
	if e = a.checkNESSNode(); e != nil {
		return nil, 503, e
	}
	// Check unreserved confirmed liquidity before offering a deposit address.
	var balance balanceResponse
	if e = a.ness("/api/v1/wallet/balance?id="+url.QueryEscape(a.cfg.NESSWallet), &balance); e != nil {
		return nil, 503, e
	}
	available := uint64(0)
	for source, b := range balance.Addresses {
		if !validNESSAddress(source) {
			return nil, 503, errors.New("node returned an invalid wallet address")
		}
		if a.reservedAddress(source) {
			continue
		}
		available, e = add(available, min(b.Confirmed.Coins, b.Predicted.Coins))
		if e != nil {
			return nil, 503, e
		}
	}
	needed, _ := coins.units(6)
	// Outstanding direct quotes reserve liquidity as well as their deposit inputs.
	for _, other := range a.store.Slots {
		if other.ExchangeRate == 0 || other.Status == "submitted" || other.Status == "done" || other.Status == "cancelled" {
			continue
		}
		n, err := other.ExchangeCoins.units(6)
		if err != nil {
			return nil, 503, errors.New("stored exchange quote needs operator review")
		}
		if n > available {
			available = 0
		} else {
			available -= n
		}
	}
	if available < needed {
		return nil, 503, &apiError{503, "The exchange wallet needs more unreserved NESS before accepting this deposit."}
	}
	deposit, e := a.newNESSAddress()
	if e != nil {
		return nil, 503, e
	}
	// A deposit address must be unused, even if the node erroneously repeats one.
	if _, used := balance.Addresses[deposit]; used {
		return nil, 503, errors.New("node reused an existing wallet address; no deposit accepted")
	}
	for _, other := range a.store.Slots {
		if other.GenAddress == deposit {
			return nil, 503, errors.New("node reused an exchange deposit address")
		}
		for _, p := range other.Payments {
			if p.Address == deposit {
				return nil, 503, errors.New("node reused a payment address")
			}
		}
	}
	slot := Slot{ID: randomID(16), Session: id, Kind: "exchange", Status: "generated", Created: a.now().Unix(), GenAddress: deposit, PayAddress: address, ExchangeRate: nchPerNESS, ExchangeHours: hours, ExchangeCoins: coins}
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	return slot, 201, nil
}

func (a *App) cancelDirectExchange(slot Slot) (any, int, error) {
	if slot.Status == "cancelled" {
		return slot, 200, nil
	}
	if slot.Status != "generated" {
		return nil, 409, &apiError{409, "This exchange can no longer be cancelled."}
	}
	if e := a.checkNESSNode(); e != nil {
		return nil, 503, e
	}
	var balance balanceResponse
	if e := a.ness("/api/v1/balance?addrs="+url.QueryEscape(slot.GenAddress), &balance); e != nil {
		return nil, 503, e
	}
	if balance.Confirmed.Coins != 0 || balance.Predicted.Coins != 0 {
		return nil, 409, &apiError{409, "A deposit is present or pending. Check payment instead."}
	}
	slot.Status = "cancelled"
	slot.Error = "Cancelled. Never send funds to this address; a late deposit requires operator reconciliation."
	if e := a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	return slot, 200, nil
}

func (a *App) directExchangeStatus(slot Slot, process bool) (any, int, error) {
	if !process || slot.Status != "generated" {
		return slot, 200, nil
	}
	quote, e := exchangeQuote(slot.ExchangeHours)
	if e != nil || slot.ExchangeRate != nchPerNESS || quote != slot.ExchangeCoins || !validNESSAddress(slot.GenAddress) || !validNESSAddress(slot.PayAddress) {
		return nil, 503, errors.New("stored exchange quote is invalid; operator review required")
	}
	if e = a.checkNESSNode(); e != nil {
		return nil, 503, e
	}
	var balance balanceResponse
	if e = a.ness("/api/v1/balance?addrs="+url.QueryEscape(slot.GenAddress), &balance); e != nil {
		return nil, 503, e
	}
	if balance.Confirmed.Coins == 0 || balance.Confirmed.Hours < slot.ExchangeHours {
		return slot, 200, nil
	}
	if balance.Confirmed.Coins != 1 || balance.Confirmed.Hours != slot.ExchangeHours {
		slot.Status = "review"
		slot.Error = "The deposit differs from the quote. The operator must reconcile it; do not send another payment."
		if e = a.store.slot(slot); e != nil {
			return nil, 503, e
		}
		return slot, 200, nil
	}
	// Record the obligation BEFORE any signing/broadcast call. Unknown results are
	// never retried automatically, including after process restart.
	slot.Status = "review"
	slot.Error = "Exchange payout is pending operator review. Do not deposit again."
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	txid, e := a.payAmount(slot.PayAddress, slot.ExchangeCoins, 0, slot.GenAddress)
	if e != nil {
		return slot, 200, nil
	}
	slot.Txid = txid
	slot.Status = "submitted"
	slot.Error = ""
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	return slot, 200, nil
}
