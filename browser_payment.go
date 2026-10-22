package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
)

// Only prepares the NCH payment for this session's immutable NVS request.
// Visitor keys never enter the service wallet or any backend API.
func (a *App) browserPayment(session, kind, action string, in map[string]json.RawMessage) (any, int, error) {
	sid, e := textField(in, "slot_id", 64)
	if e != nil {
		return nil, 400, e
	}
	slot, ok := a.store.Slots[sid]
	if !ok || slot.Session != session || kind != "nvs" || slot.Kind != "nvs" {
		return nil, 403, &apiError{403, "Request not found in this session."}
	}
	if slot.BrowserPaymentState != "" {
		return slot, 200, nil
	}
	if slot.Status != "generated" {
		return nil, 409, &apiError{409, "This request no longer accepts a payment."}
	}
	payment, ok := slot.Payments["NCH"]
	if !ok {
		return nil, 400, bad("NCH payment is not available for this request.")
	}
	hours, e := payment.Minimum.units(0)
	if e != nil {
		return nil, 503, e
	}
	if action == "prepare_payment" {
		source, e := textField(in, "address", 35)
		if e != nil {
			return nil, 400, e
		}
		if !validNESSAddress(source) || source == payment.Address {
			return nil, 400, bad("Choose your NESS wallet.")
		}
		if e = a.checkNESSNode(); e != nil {
			return nil, 503, e
		}
		body, _ := json.Marshal(map[string]any{"ignore_unconfirmed": true, "hours_selection": map[string]string{"type": "manual"}, "addresses": []string{source}, "change_address": source, "to": []receiver{{payment.Address, "0.000001", strconv.FormatUint(hours, 10)}}})
		var response struct {
			Data  createdTransaction `json:"data"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if e = a.nessPost("/api/v2/transaction", body, "application/json", &response); e != nil {
			return nil, 503, e
		}
		if response.Error != nil {
			return nil, 503, errors.New("node could not prepare visitor payment")
		}
		tx := response.Data
		if len(tx.Transaction.Inputs) > 64 {
			return nil, 400, bad("This wallet needs consolidation in the NESS app before paying here.")
		}
		for _, input := range tx.Transaction.Inputs {
			if input.Address != source {
				return nil, 503, errors.New("payment input is not owned by visitor")
			}
		}
		if e = validateCreatedMode(tx, payment.Address, 1, hours, map[string]bool{source: true}, false); e != nil {
			return nil, 503, e
		}
		slot.BrowserSource, slot.BrowserTemplate = source, &tx
		if e = a.store.slot(slot); e != nil {
			return nil, 503, e
		}
		return slot, 200, nil
	}
	if slot.BrowserTemplate == nil {
		return nil, 400, bad("Review the payment before signing it.")
	}
	raw, e := textField(in, "rawtx", 15000)
	if e != nil {
		return nil, 400, e
	}
	parsed, e := decodeTransaction(raw)
	if e != nil {
		return nil, 400, bad("Invalid signed payment.")
	}
	original, _ := hex.DecodeString(slot.BrowserTemplate.Encoded)
	signed, _ := hex.DecodeString(raw)
	offset := 41 + 65*len(slot.BrowserTemplate.Transaction.Inputs)
	if len(signed) != len(original) || offset > len(signed) || !bytes.Equal(signed[:41], original[:41]) || !bytes.Equal(signed[offset:], original[offset:]) {
		return nil, 400, bad("The signed payment differs from the reviewed payment.")
	}
	// Durable before broadcast. An uncertain response must never cause a new spend.
	slot.BrowserPaymentRaw, slot.BrowserPaymentTx, slot.BrowserPaymentState = raw, parsed.ID, "review"
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	body, _ := json.Marshal(map[string]string{"rawtx": raw})
	var txid string
	if e = a.nessPost("/api/v1/injectTransaction", body, "application/json", &txid); e != nil || txid != parsed.ID {
		return slot, 200, nil
	}
	slot.BrowserPaymentState = "submitted"
	if e = a.store.slot(slot); e != nil {
		return nil, 503, e
	}
	return slot, 200, nil
}
