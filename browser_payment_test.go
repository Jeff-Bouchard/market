package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestBrowserPaymentContract(t *testing.T) {
	for _, uncertain := range []bool{false, true} {
		t.Run(map[bool]string{false: "submitted", true: "interrupted"}[uncertain], func(t *testing.T) {
			a := testApp(t)
			raw, e := os.ReadFile("testdata/browser-payment-native.json")
			if e != nil {
				t.Fatal(e)
			}
			var fixture struct {
				Address  string             `json:"address"`
				Template createdTransaction `json:"template"`
			}
			if e = json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &fixture); e != nil {
				t.Fatal(e)
			}
			raw, e = os.ReadFile("testdata/browser-payment-signed.json")
			if e != nil {
				t.Fatal(e)
			}
			var signed struct {
				Raw string `json:"rawtx"`
				ID  string `json:"txid"`
			}
			if e = json.Unmarshal(raw, &signed); e != nil {
				t.Fatal(e)
			}
			slot := Slot{ID: "nvs-request", Session: "visitor", Kind: "nvs", Status: "generated", Name: "dns:test.emc", Value: "test", Days: 100, Payments: map[string]Payment{"NCH": {Address: fixture.Template.Transaction.Outputs[0].Address, Minimum: "1"}}}
			if e = a.store.slot(slot); e != nil {
				t.Fatal(e)
			}
			injections := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if serveNodeIdentity(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/v1/csrf":
					json.NewEncoder(w).Encode(map[string]string{"csrf_token": "native"})
				case "/api/v2/transaction":
					var body map[string]json.RawMessage
					json.NewDecoder(r.Body).Decode(&body)
					if _, ok := body["wallet_id"]; ok {
						t.Error("visitor payment used service wallet")
					}
					var addresses []string
					json.Unmarshal(body["addresses"], &addresses)
					if len(addresses) != 1 || addresses[0] != fixture.Address {
						t.Error("incorrect source")
					}
					json.NewEncoder(w).Encode(map[string]any{"data": fixture.Template})
				case "/api/v1/injectTransaction":
					injections++
					if s := a.store.Slots[slot.ID]; s.BrowserPaymentState != "review" || s.BrowserPaymentTx != signed.ID {
						t.Error("broadcast before durable reservation")
					}
					if uncertain {
						http.Error(w, "unavailable", 503)
						return
					}
					json.NewEncoder(w).Encode(signed.ID)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			a.cfg.NESSURL = server.URL
			request := func(action, visitor, tx string) (any, int, error) {
				b, _ := json.Marshal(map[string]string{"action": action, "kind": "nvs", "slot_id": slot.ID, "address": fixture.Address, "rawtx": tx})
				return a.exchange(visitor, input(string(b)))
			}
			if _, code, e := request("prepare_payment", "stranger", ""); e == nil || code != 403 {
				t.Fatal("cross-session preparation accepted")
			}
			if _, _, e := request("prepare_payment", "visitor", ""); e != nil {
				t.Fatal(e)
			}
			tampered := signed.Raw[:len(signed.Raw)-2] + "01"
			if _, _, e := request("submit_payment", "visitor", tampered); e == nil {
				t.Fatal("changed transaction accepted")
			}
			result, _, e := request("submit_payment", "visitor", signed.Raw)
			if e != nil {
				t.Fatal(e)
			}
			want := "submitted"
			if uncertain {
				want = "review"
			}
			if result.(Slot).BrowserPaymentState != want {
				t.Fatal(result)
			}
			if _, _, e = request("submit_payment", "visitor", signed.Raw); e != nil {
				t.Fatal(e)
			}
			if injections != 1 {
				t.Fatal("payment broadcast more than once", injections)
			}
			dir := a.cfg.DataDir
			a.store.Close()
			reopened, e := openStore(dir)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			if reopened.Slots[slot.ID].BrowserPaymentTx != signed.ID || reopened.Slots[slot.ID].BrowserPaymentState != want {
				t.Fatal("payment reservation lost on restart")
			}
		})
	}
}
