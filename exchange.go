package main

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"unicode"
)

func (a *App) exchange(id string, in map[string]json.RawMessage) (any, int, error) {
	action, e := textField(in, "action", 16)
	if e != nil {
		return nil, 400, e
	}
	kind, e := textField(in, "kind", 16)
	if e != nil {
		return nil, 400, e
	}
	if kind != "nvs" && kind != "exchange" {
		return nil, 400, bad("Invalid request kind.")
	}
	if action == "prepare_payment" || action == "submit_payment" {
		return a.browserPayment(id, kind, action, in)
	}
	if action == "status" || action == "cancel" {
		sid, e := textField(in, "slot_id", 64)
		if e != nil {
			return nil, 400, e
		}
		slot, ok := a.store.Slots[sid]
		if !ok || slot.Session != id || slot.Kind != kind {
			return nil, 403, &apiError{403, "Request not found in this session."}
		}
		if slot.ExchangeRate != 0 {
			if action == "cancel" {
				return a.cancelDirectExchange(slot)
			}
			return a.directExchangeStatus(slot, boolField(in, "process"))
		}
		if action == "cancel" {
			return nil, 400, bad("Only unpaid direct exchanges can be cancelled.")
		}
		if boolField(in, "process") {
			if kind == "nvs" && slot.Status == "payed" {
				return a.confirmNVS(slot)
			}
			if slot.Status == "generated" {
				var lastPaymentError error
				for _, currency := range []string{"EMC", "NESS", "NCH"} {
					p, ok := slot.Payments[currency]
					if !ok {
						continue
					}
					paid, e := a.paid(currency, p)
					if e != nil {
						lastPaymentError = e
						continue
					}
					if !paid {
						continue
					}
					if e = a.checkEMCWallet(); e != nil {
						return nil, 503, e
					}
					slot.Status = "review"
					slot.Error = "Registration is pending operator review. Do not pay again."
					if e = a.store.slot(slot); e != nil {
						return nil, 503, e
					}
					args := []any{slot.Name, slot.Value, slot.Days}
					if slot.Owner != "" {
						args = append(args, slot.Owner)
					}
					var tx string
					if e = a.rpc("name_new", args, &tx); e != nil {
						return slot, 200, nil
					}
					if !txPattern.MatchString(tx) {
						return slot, 200, nil
					}
					slot.Txid = tx
					slot.Status = "payed"
					slot.Error = ""
					if e = a.store.slot(slot); e != nil {
						return nil, 503, e
					}
					break
				}
				if slot.Status == "generated" && lastPaymentError != nil {
					return nil, 503, lastPaymentError
				}
			}
			if kind == "exchange" && slot.Status == "payed" {
				return a.exchangeStatus(slot)
			}
		}
		return slot, 200, nil
	}
	if action != "create" {
		return nil, 400, bad("Unknown action.")
	}
	if kind == "exchange" {
		return a.createDirectExchange(id, in)
	}
	if a.cfg.EMCURL == "" {
		return nil, 503, &apiError{503, "The operator must configure Emercoin RPC before creating requests."}
	}
	slot := Slot{ID: randomID(16), Session: id, Kind: kind, Created: a.now().Unix(), Status: "generated"}
	if kind == "nvs" {
		slot.Name, e = textField(in, "name", 512)
		if e != nil {
			return nil, 400, e
		}
		if _, ok := in["value"]; ok {
			slot.Value, e = textField(in, "value", 10240)
			if e != nil {
				return nil, 400, e
			}
		} else {
			slot.Value = defaultNS
		}
		slot.Owner, e = textField(in, "owner", 64)
		if e != nil {
			return nil, 400, e
		}
		if slot.Owner != "" && !validEMCAddress(slot.Owner) {
			return nil, 400, bad("Enter your own EMC owner address.")
		}
		var days string
		if json.Unmarshal(in["days"], &days) != nil {
			var n int
			if json.Unmarshal(in["days"], &n) != nil {
				return nil, 400, bad("Invalid duration.")
			}
			days = strconv.Itoa(n)
		}
		slot.Days, e = strconv.Atoi(days)
		if e != nil || slot.Days < 100 || slot.Days > 3650 {
			return nil, 400, bad("Duration must be 100–3650 days.")
		}
		if slot.Name == "" || slot.Value == "" || strings.IndexFunc(slot.Name, unicode.IsControl) >= 0 {
			return nil, 400, bad("Enter a record name and value.")
		}
	}
	var last int64
	for _, other := range a.store.Slots {
		if other.Created > last {
			last = other.Created
		}
		if other.Name == slot.Name {
			if other.Session == id && other.Kind == slot.Kind && other.Value == slot.Value && other.Owner == slot.Owner && other.Days == slot.Days {
				return other, 200, nil
			}
			return nil, 409, &apiError{409, "A request already exists for this name. Check its original request."}
		}
	}
	if a.now().Unix()-last < 30 {
		return nil, 429, &apiError{429, "Wait 30 seconds between new requests."}
	}
	exists, e := a.nameExists(slot.Name)
	if e != nil {
		return nil, 503, e
	}
	if exists {
		return nil, 409, &apiError{409, "This name is already registered. Manage it in the owning Emercoin wallet."}
	}
	slot.Payments, e = a.payments(slot.Days)
	if e != nil {
		return nil, 503, e
	}
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	return slot, 201, nil
}
func (a *App) confirmNVS(slot Slot) (any, int, error) {
	var record struct {
		Name    string `json:"name"`
		Value   string `json:"value"`
		Address string `json:"address"`
		Txid    string `json:"txid"`
		Expires int64  `json:"expires_in"`
		Deleted bool   `json:"deleted"`
		Expired bool   `json:"expired"`
	}
	if e := a.rpc("name_show", []any{slot.Name}, &record); e != nil {
		var rpc *rpcError
		if errors.As(e, &rpc) && rpc.Code == -4 {
			return slot, 200, nil
		}
		return nil, 503, e
	}
	if record.Name == slot.Name && record.Value == slot.Value && record.Txid == slot.Txid && (slot.Owner == "" || record.Address == slot.Owner) && record.Expires > 0 && !record.Deleted && !record.Expired {
		slot.Status = "done"
		if e := a.store.slot(slot); e != nil {
			return nil, 503, e
		}
	}
	return slot, 200, nil
}
func (a *App) exchangeStatus(slot Slot) (any, int, error) {
	u, e := url.Parse(a.cfg.ExchangeURL)
	if e != nil {
		return nil, 503, e
	}
	q := u.Query()
	q.Set("address", slot.Address)
	q.Set("pay_address", slot.PayAddress)
	u.RawQuery = q.Encode()
	var result struct {
		Status     string  `json:"status"`
		GenAddress string  `json:"gen_address"`
		Hours      float64 `json:"hours"`
		Receive    float64 `json:"recieve"`
		Error      string  `json:"error"`
	}
	if e = a.request("GET", u.String(), "", "", nil, &result, ""); e != nil {
		return nil, 503, e
	}
	view := slot
	view.Hours = result.Hours
	view.Receive = result.Receive
	if result.GenAddress != "" {
		if !validNESSAddress(result.GenAddress) {
			return nil, 503, bad("Exchange returned invalid receiving address.")
		}
		view.GenAddress = result.GenAddress
	}
	switch strings.ToLower(result.Status) {
	case "activated":
		view.Status = "activated"
	case "payed":
		view.Status = "done"
	case "error", "nofunds":
		view.Status = "error"
		view.Error = "The exchange needs operator review. " + result.Error
	}
	return view, 200, nil
}
