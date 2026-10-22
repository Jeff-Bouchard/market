package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNVSConfirmationMatchesReviewedRecord(t *testing.T) {
	for _, tc := range []struct {
		name   string
		modify func(map[string]any)
		done   bool
	}{
		{"matches", func(_ map[string]any) {}, true},
		{"different-owner", func(m map[string]any) { m["address"] = "other" }, false},
		{"different-value", func(m map[string]any) { m["value"] = "other" }, false},
		{"different-transaction", func(m map[string]any) { m["txid"] = "other" }, false},
		{"expired", func(m map[string]any) { m["expires_in"] = -1 }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			slot := Slot{ID: "record", Session: "visitor", Kind: "nvs", Status: "payed", Name: "dns:test.emc", Value: "public", Owner: testEMC, Txid: "test-tx"}
			a.store.slot(slot)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var req struct {
					Method string `json:"method"`
				}
				json.NewDecoder(r.Body).Decode(&req)
				if req.Method != "name_show" {
					t.Error("confirmation attempted a mutation")
				}
				record := map[string]any{"name": slot.Name, "value": slot.Value, "address": slot.Owner, "txid": slot.Txid, "expires_in": 100}
				tc.modify(record)
				json.NewEncoder(w).Encode(map[string]any{"result": record})
			}))
			defer server.Close()
			a.cfg.EMCURL = server.URL
			v, _, e := a.confirmNVS(slot)
			if e != nil {
				t.Fatal(e)
			}
			if (v.(Slot).Status == "done") != tc.done {
				t.Fatal("incorrect confirmation", v)
			}
		})
	}
}
