package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("Emercoin RPC %d: %s", e.Code, e.Message) }
func (a *App) request(method, endpoint, contentType, csrf string, body []byte, out any, basic string) error {
	req, e := http.NewRequest(method, endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Cache-Control", "no-store, no-cache")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if csrf != "" {
		req.Header.Set("X-CSRF-Token", csrf)
	}
	if basic == "emc" {
		req.SetBasicAuth(a.cfg.EMCUser, a.cfg.EMCPassword)
	}
	if basic == "ness" && (a.cfg.NESSAPIUser != "" || a.cfg.NESSAPIPassword != "") {
		req.SetBasicAuth(a.cfg.NESSAPIUser, a.cfg.NESSAPIPassword)
	}
	resp, e := a.client.Do(req)
	if e != nil {
		return errors.New("upstream connection failed")
	}
	defer resp.Body.Close()
	raw, e := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024+1))
	if e != nil || len(raw) > 2*1024*1024 {
		return errors.New("invalid upstream response")
	}
	// Emercoin returns JSON-RPC errors with HTTP 500; preserve the RPC code.
	if resp.StatusCode != 200 && !(basic == "emc" && resp.StatusCode == 500) {
		return &upstreamHTTPError{resp.StatusCode}
	}
	if e = json.Unmarshal(raw, out); e != nil {
		return errors.New("upstream returned invalid JSON")
	}
	return nil
}
func (a *App) rpc(method string, params []any, out any) error {
	endpoint := a.cfg.EMCURL
	switch method {
	case "getnewaddress", "getreceivedbyaddress", "name_new", "getwalletinfo":
		var e error
		endpoint, e = a.emcWalletEndpoint()
		if e != nil {
			return e
		}
	}
	return a.rpcAt(endpoint, method, params, out)
}
func (a *App) rpcAt(endpoint, method string, params []any, out any) error {
	if a.cfg.EMCURL == "" {
		return errors.New("configure emc_url first")
	}
	b, _ := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": "privateness", "method": method, "params": params})
	var response struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if e := a.request("POST", endpoint, "application/json", "", b, &response, "emc"); e != nil {
		return e
	}
	if response.Error != nil {
		return response.Error
	}
	if len(response.Result) == 0 || string(response.Result) == "null" {
		return errors.New("empty RPC result")
	}
	return json.Unmarshal(response.Result, out)
}
func (a *App) emcWalletEndpoint() (string, error) {
	if a.emcWalletURL != "" {
		return a.emcWalletURL, nil
	}
	u, e := url.Parse(a.cfg.EMCURL)
	if e != nil || u.Host == "" {
		return "", errors.New("configure emc_url first")
	}
	if index := strings.Index(u.Path, "/wallet/"); index >= 0 {
		if a.cfg.EMCWallet != nil && u.Path[index+8:] != *a.cfg.EMCWallet {
			return "", errors.New("emc_wallet conflicts with the wallet path in emc_url")
		}
		a.emcWalletURL = u.String()
		return a.emcWalletURL, nil
	}
	wallet := a.cfg.EMCWallet
	if wallet == nil {
		var loaded []string
		if e = a.rpcAt(u.String(), "listwallets", []any{}, &loaded); e != nil {
			return "", fmt.Errorf("cannot select an Emercoin wallet: set emc_wallet to its loaded name: %w", e)
		}
		if len(loaded) != 1 {
			return "", fmt.Errorf("Emercoin has %d loaded wallets; set emc_wallet to the exact wallet name (empty string selects the default wallet)", len(loaded))
		}
		wallet = &loaded[0]
	}
	u.RawPath = strings.TrimRight(u.EscapedPath(), "/") + "/wallet/" + url.PathEscape(*wallet)
	u.Path = strings.TrimRight(u.Path, "/") + "/wallet/" + *wallet
	a.emcWalletURL = u.String()
	return a.emcWalletURL, nil
}
func (a *App) checkEMCWallet() error {
	var info struct {
		PrivateKeys   *bool  `json:"private_keys_enabled"`
		UnlockedUntil *int64 `json:"unlocked_until"`
	}
	if e := a.rpc("getwalletinfo", []any{}, &info); e != nil {
		return e
	}
	if info.PrivateKeys != nil && !*info.PrivateKeys {
		return errors.New("selected Emercoin wallet cannot sign registrations (private keys disabled)")
	}
	if info.UnlockedUntil != nil && *info.UnlockedUntil <= a.now().Unix() {
		return errors.New("unlock the selected Emercoin wallet for transactions before registering; minting-only unlock is not sufficient")
	}
	return nil
}
func (a *App) ness(path string, out any) error {
	if a.cfg.NESSURL == "" {
		return errors.New("configure ness_url first")
	}
	return a.request("GET", strings.TrimRight(a.cfg.NESSURL, "/")+path, "", "", nil, out, "ness")
}
func (a *App) nessPost(path string, body []byte, contentType string, out any) error {
	var token struct {
		CSRF string `json:"csrf_token"`
	}
	if e := a.ness("/api/v1/csrf", &token); e != nil {
		var he *upstreamHTTPError
		if !errors.As(e, &he) || he.Status != 404 {
			return e
		}
	} else if token.CSRF == "" {
		return errors.New("wallet CSRF token missing")
	}

	return a.request("POST", strings.TrimRight(a.cfg.NESSURL, "/")+path, contentType, token.CSRF, body, out, "ness")
}
func (a *App) newNESSAddress() (string, error) {
	form := url.Values{"id": {a.cfg.NESSWallet}, "num": {"1"}, "password": {a.cfg.NESSPassword}}
	var response struct {
		Addresses []string `json:"addresses"`
	}
	e := a.nessPost("/api/v1/wallet/newAddress", []byte(form.Encode()), "application/x-www-form-urlencoded", &response)
	if e != nil {
		return "", e
	}
	if len(response.Addresses) != 1 || !validNESSAddress(response.Addresses[0]) {
		return "", errors.New("wallet returned invalid address")
	}
	return response.Addresses[0], nil
}
func (a *App) pay(address string, seconds int64) (string, error) {
	coinAmount, hours, e := a.reward(seconds)
	if e != nil {
		return "", e
	}
	return a.payAmount(address, coinAmount, hours, "")
}
func (a *App) payAmount(address string, coinAmount Decimal, hours uint64, allowedDeposit string) (string, error) {
	coins, e := coinAmount.units(6)
	if e != nil {
		return "", e
	}
	var balance balanceResponse
	if e := a.ness("/api/v1/wallet/balance?id="+url.QueryEscape(a.cfg.NESSWallet), &balance); e != nil {
		return "", e
	}
	addresses := []string{}
	owned := map[string]bool{}
	var liquid uint64
	for address := range balance.Addresses {
		if a.reservedAddress(address) && address != allowedDeposit {
			continue
		}
		b := balance.Addresses[address]
		liquid, e = add(liquid, min(b.Confirmed.Coins, b.Predicted.Coins))
		if e != nil {
			return "", e
		}
		if !validNESSAddress(address) {
			return "", errors.New("wallet returned invalid source address")
		}
		addresses = append(addresses, address)
		owned[address] = true
	}
	var reserved uint64
	for _, slot := range a.store.Slots {
		if slot.ExchangeRate == 0 || slot.Status == "submitted" || slot.Status == "done" || slot.Status == "cancelled" || slot.GenAddress == allowedDeposit {
			continue
		}
		n, err := slot.ExchangeCoins.units(6)
		if err != nil {
			return "", errors.New("stored exchange quote needs operator review")
		}
		reserved, e = add(reserved, n)
		if e != nil {
			return "", e
		}
	}
	if reserved > 0 && (reserved > liquid || coins > liquid-reserved) {
		return "", errors.New("wallet liquidity is reserved for pending exchanges; operator funding required")
	}
	if len(addresses) == 0 {
		return "", errors.New("wallet has no addresses")
	}
	sort.Strings(addresses)
	// Explicit wallet-owned change prevents BIP44-generated change from escaping the verified set.
	var request walletTransactionRequest
	request.WalletID = a.cfg.NESSWallet
	request.Password = a.cfg.NESSPassword
	request.HoursSelection.Type = "manual"
	request.Addresses = addresses
	request.ChangeAddress = addresses[0]
	request.To = []receiver{{address, string(coinAmount), string(scaledDecimal(hours, 0))}}
	body, _ := json.Marshal(request)
	var tx createdTransaction
	if e = a.nessPost("/api/v1/wallet/transaction", body, "application/json", &tx); e != nil {
		return "", e
	}
	if e = validateCreated(tx, address, coins, hours, owned); e != nil {
		return "", e
	}
	body, _ = json.Marshal(map[string]string{"rawtx": tx.Encoded})
	var id string
	if e = a.nessPost("/api/v1/injectTransaction", body, "application/json", &id); e != nil {
		return "", e
	}
	if id != tx.Transaction.Txid {
		return "", errors.New("transaction receipt mismatch")
	}
	return id, nil
}
func (a *App) reservedAddress(address string) bool {
	for _, slot := range a.store.Slots {
		if slot.ExchangeRate != 0 {
			if slot.Status != "submitted" && slot.Status != "done" && slot.GenAddress == address {
				return true
			}
		} else if slot.Status != "payed" && slot.Status != "done" {
			for currency, p := range slot.Payments {
				if currency != "EMC" && p.Address == address {
					return true
				}
			}
		}
	}
	return false
}

type receiver struct {
	Address string `json:"address"`
	Coins   string `json:"coins"`
	Hours   string `json:"hours"`
}
type walletTransactionRequest struct {
	WalletID       string `json:"wallet_id"`
	Password       string `json:"password"`
	HoursSelection struct {
		Type string `json:"type"`
	} `json:"hours_selection"`
	Addresses     []string   `json:"addresses"`
	ChangeAddress string     `json:"change_address"`
	To            []receiver `json:"to"`
}
type balanceAmounts struct {
	Coins uint64 `json:"coins"`
	Hours uint64 `json:"hours"`
}
type balancePair struct {
	Confirmed balanceAmounts `json:"confirmed"`
	Predicted balanceAmounts `json:"predicted"`
}
type balanceResponse struct {
	balancePair
	Addresses map[string]balancePair `json:"addresses"`
}
type upstreamHTTPError struct{ Status int }

func (e *upstreamHTTPError) Error() string { return fmt.Sprintf("upstream returned HTTP %d", e.Status) }
func (a *App) nameExists(name string) (bool, error) {
	var record map[string]any
	e := a.rpc("name_show", []any{name}, &record)
	if e == nil {
		if record["expired"] == true || record["deleted"] == true {
			return false, nil
		}
		return true, nil
	}
	var re *rpcError
	if errors.As(e, &re) && re.Code == -4 && (re.Message == "failed to read from name DB" || re.Message == "no result returned" || re.Message == "not found") {
		return false, nil
	}
	return false, e
}
func (a *App) payments(days int) (map[string]Payment, error) {
	n := uint64((days + 99) / 100)
	payments := map[string]Payment{}
	if e := a.checkEMCWallet(); e != nil {
		return nil, e
	}
	// Finish read-only validation before allocating any wallet addresses.
	nessEnabled := a.cfg.NESSURL != "" && a.cfg.NESSWallet != ""
	if nessEnabled {
		if e := a.checkNESSNode(); e != nil {
			return nil, e
		}
	}
	emc, e := a.cfg.EMCPrice.units(8)
	if e != nil {
		return nil, e
	}
	emc, e = multiply(emc, n)
	if e != nil {
		return nil, e
	}
	var address string
	if e := a.rpc("getnewaddress", []any{"NVS-Exchange", "legacy"}, &address); e != nil {
		return nil, e
	}
	if !validEMCAddress(address) {
		return nil, errors.New("Emercoin returned invalid mainnet address")
	}
	payments["EMC"] = Payment{address, scaledDecimal(emc, 8), days}
	if nessEnabled {
		for _, currency := range []string{"NESS", "NCH"} {
			price := a.cfg.NESSPrice
			precision := 6
			if currency == "NCH" {
				price = a.cfg.NCHPrice
				precision = 0
			}
			amount, e := price.units(precision)
			if e != nil {
				return nil, e
			}
			amount, e = multiply(amount, n)
			if e != nil {
				return nil, e
			}
			address, e := a.newNESSAddress()
			if e != nil {
				return nil, e
			}
			payments[currency] = Payment{address, scaledDecimal(amount, precision), days}
		}
	}
	return payments, nil
}
func (a *App) paid(currency string, p Payment) (bool, error) {
	precision := 6
	if currency == "EMC" {
		precision = 8
	} else if currency == "NCH" {
		precision = 0
	}
	minimum, e := p.Minimum.units(precision)
	if e != nil || minimum == 0 {
		return false, errors.New("invalid stored payment price")
	}
	if currency == "EMC" {
		var amount Decimal
		if e := a.rpc("getreceivedbyaddress", []any{p.Address, 1}, &amount); e != nil {
			return false, e
		}
		units, e := amount.units(8)
		return units >= minimum, e
	}
	if e := a.checkNESSNode(); e != nil {
		return false, e
	}
	var balance balanceResponse
	if e := a.ness("/api/v1/balance?addrs="+url.QueryEscape(p.Address), &balance); e != nil {
		return false, e
	}
	if currency == "NESS" {
		return balance.Confirmed.Coins >= minimum, nil
	}
	if currency == "NCH" {
		return balance.Confirmed.Hours >= minimum && balance.Confirmed.Coins >= 1, nil
	}
	return false, errors.New("unsupported currency")
}
