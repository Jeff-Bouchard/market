package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmercoinWalletRouting(t *testing.T) {
	for _, name := range []string{"emc25-kaisen.dat", "", "folder/wallet #é.dat"} {
		t.Run(name, func(t *testing.T) {
			a := testApp(t)
			a.cfg.EMCWallet = &name
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if r.URL.Path != "/wallet/"+name {
					t.Errorf("wrong wallet path %q", r.URL.Path)
				}
				var call struct {
					Method string `json:"method"`
					Params []any  `json:"params"`
				}
				json.NewDecoder(r.Body).Decode(&call)
				if call.Method != "getwalletinfo" {
					t.Error(call.Method)
				}
				fmt.Fprint(w, `{"result":{"private_keys_enabled":true},"error":null}`)
			}))
			defer server.Close()
			a.cfg.EMCURL = server.URL
			if e := a.checkEMCWallet(); e != nil {
				t.Fatal(e)
			}
			if calls != 1 {
				t.Fatal(calls)
			}
			a.emcWalletURL = ""
			a.cfg.EMCURL += "/wallet/wrong"
			if e := a.checkEMCWallet(); e == nil || calls != 1 {
				t.Fatal("conflicting path accepted")
			}
		})
	}
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint("auto", count), func(t *testing.T) {
			a := testApp(t)
			writes := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var call struct {
					Method string `json:"method"`
				}
				json.NewDecoder(r.Body).Decode(&call)
				if call.Method == "listwallets" {
					names := []string{}
					for i := 0; i < count; i++ {
						names = append(names, fmt.Sprint("wallet", i))
					}
					json.NewEncoder(w).Encode(map[string]any{"result": names})
					return
				}
				writes++
				if r.URL.Path != "/wallet/wallet0" {
					t.Error(r.URL.Path)
				}
				w.WriteHeader(500)
				fmt.Fprint(w, `{"error":{"code":-19,"message":"mock write failure"}}`)
			}))
			defer server.Close()
			a.cfg.EMCURL = server.URL
			var tx string
			if e := a.rpc("name_new", []any{"dns:test", "value", 100}, &tx); e == nil {
				t.Fatal("failure ignored")
			}
			want := 0
			if count == 1 {
				want = 1
			}
			if writes != want {
				t.Fatal("ambiguous selection or write retry", writes)
			}
		})
	}
}

func TestEMCWalletReadinessAndNameErrors(t *testing.T) {
	a := testApp(t)
	name := "selected"
	a.cfg.EMCWallet = &name
	result := `{"private_keys_enabled":false}`
	message := "failed to read from name DB"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string `json:"method"`
		}
		json.NewDecoder(r.Body).Decode(&call)
		if call.Method == "name_show" {
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"code": -4, "message": message}})
			return
		}
		fmt.Fprint(w, `{"result":`+result+`}`)
	}))
	defer server.Close()
	a.cfg.EMCURL = server.URL
	if e := a.checkEMCWallet(); e == nil {
		t.Fatal("watch-only wallet accepted")
	}
	result = `{"private_keys_enabled":true,"unlocked_until":0}`
	if e := a.checkEMCWallet(); e == nil {
		t.Fatal("locked wallet accepted")
	}
	result = `{"private_keys_enabled":true}`
	if e := a.checkEMCWallet(); e != nil {
		t.Fatal(e)
	}
	if exists, e := a.nameExists("dns:test"); e != nil || exists {
		t.Fatal(exists, e)
	}
	message = "failed to read transaction from disk"
	if _, e := a.nameExists("dns:test"); e == nil {
		t.Fatal("disk failure treated as available name")
	}
}

func TestDirectExchangeQuote(t *testing.T) {
	for _, tc := range []struct {
		n    uint64
		want Decimal
	}{{1, "0.000100"}, {10000, "1.000000"}, {8760, "0.876000"}, {10001, "1.000100"}} {
		got, e := exchangeQuote(tc.n)
		if e != nil || got != tc.want {
			t.Fatal(tc, got, e)
		}
	}
	for _, n := range []uint64{0, ^uint64(0)} {
		if _, e := exchangeQuote(n); e == nil {
			t.Fatal("invalid amount accepted", n)
		}
	}
}

func TestDirectExchangeLifecycle(t *testing.T) {
	for _, scenario := range []string{"paid", "uncertain", "overpaid", "cancel"} {
		t.Run(scenario, func(t *testing.T) {
			a := testApp(t)
			a.cfg.NESSWallet = "wallet.wlt"
			now := time.Unix(1800000000, 0)
			a.now = func() time.Time { return now }
			var deposit balanceResponse
			broadcasts, allocations, signs := 0, 0, 0
			tx := mockCreated(testNESS, 1000000, 0, 20000, 1000, 2000000)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if serveNodeIdentity(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/v1/csrf":
					fmt.Fprint(w, `{"csrf_token":"token"}`)
				case "/api/v1/wallet/balance":
					b := balanceResponse{Addresses: map[string]balancePair{testChange: {Confirmed: balanceAmounts{2000000, 20000}, Predicted: balanceAmounts{2000000, 20000}}}}
					if allocations > 0 {
						b.Addresses[nessGenesisAddress] = deposit.balancePair
					}
					json.NewEncoder(w).Encode(b)
				case "/api/v1/wallet/newAddress":
					allocations++
					fmt.Fprintf(w, `{"addresses":[%q]}`, nessGenesisAddress)
				case "/api/v1/balance":
					if r.URL.Query().Get("addrs") != nessGenesisAddress {
						t.Error("wrong deposit query")
					}
					json.NewEncoder(w).Encode(deposit)
				case "/api/v1/wallet/transaction":
					signs++
					var req walletTransactionRequest
					json.NewDecoder(r.Body).Decode(&req)
					if len(req.To) != 1 || req.To[0].Coins != "1.000000" || req.To[0].Hours != "0" || req.To[0].Address != testNESS {
						t.Error("wrong net payout", req.To)
					}
					if !strings.Contains(strings.Join(req.Addresses, ","), nessGenesisAddress) {
						t.Error("own deposit unavailable")
					}
					json.NewEncoder(w).Encode(tx)
				case "/api/v1/injectTransaction":
					broadcasts++
					if scenario == "uncertain" {
						w.WriteHeader(503)
					} else {
						json.NewEncoder(w).Encode(tx.Transaction.Txid)
					}
				default:
					t.Error("unexpected endpoint", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			a.cfg.NESSURL = server.URL
			request := input(`{"action":"create","kind":"exchange","hours":"10000","pay_address":"` + testNESS + `"}`)
			v, code, e := a.exchange("owner", request)
			if e != nil || code != 201 {
				t.Fatal(code, e)
			}
			slot := v.(Slot)
			if slot.ExchangeCoins != "1.000000" || slot.ExchangeRate != 10000 || slot.ExchangeHours != 10000 {
				t.Fatal(slot)
			}
			v, code, e = a.exchange("owner", request)
			if e != nil || code != 200 || allocations != 1 || v.(Slot).ID != slot.ID {
				t.Fatal("non-idempotent create", code, e)
			}
			status := input(`{"action":"status","kind":"exchange","slot_id":"` + slot.ID + `","process":true}`)
			if _, _, e = a.exchange("stranger", status); e == nil {
				t.Fatal("foreign slot accepted")
			}
			deposit.Predicted = balanceAmounts{1, 10000}
			v, _, e = a.exchange("owner", status)
			if e != nil || v.(Slot).Status != "generated" || broadcasts != 0 {
				t.Fatal("unconfirmed accepted")
			}
			cancel := input(`{"action":"cancel","kind":"exchange","slot_id":"` + slot.ID + `"}`)
			if _, _, e = a.exchange("owner", cancel); e == nil {
				t.Fatal("pending deposit cancellation")
			}
			if scenario == "cancel" {
				deposit.Predicted = balanceAmounts{}
				v, _, e = a.exchange("owner", cancel)
				if e != nil || v.(Slot).Status != "cancelled" || !a.reservedAddress(slot.GenAddress) {
					t.Fatal("cancel failed or address recycled", e)
				}
				deposit.Confirmed = balanceAmounts{1, 10000}
				a.exchange("owner", status)
				if broadcasts != 0 {
					t.Fatal("cancelled request paid")
				}
				return
			}
			deposit.Confirmed = balanceAmounts{1, 9999}
			a.exchange("owner", status)
			if broadcasts != 0 {
				t.Fatal("underpayment accepted")
			}
			deposit.Confirmed = balanceAmounts{1, 10000}
			if scenario == "overpaid" {
				deposit.Confirmed.Hours++
			}
			v, _, e = a.exchange("owner", status)
			if e != nil {
				t.Fatal(e)
			}
			want := "submitted"
			if scenario != "paid" {
				want = "review"
			}
			if v.(Slot).Status != want {
				t.Fatal(v)
			}
			a.exchange("owner", status)
			expected := 1
			if scenario == "overpaid" {
				expected = 0
			}
			if broadcasts != expected || signs != expected {
				t.Fatal("duplicate/wrong payout", broadcasts, signs)
			}
			// Replay the durable journal into a restarted app before another status check.
			a.store.Close()
			restarted, e := newApp(a.cfg)
			if e != nil {
				t.Fatal(e)
			}
			defer restarted.store.Close()
			restarted.exchange("owner", status)
			if broadcasts != expected {
				t.Fatal("broadcast repeated after restart")
			}
		})
	}
}

func TestDepositsAndLiquidityProtected(t *testing.T) {
	a := testApp(t)
	a.cfg.NESSWallet = "wallet"
	a.store.Slots["quote"] = Slot{ExchangeRate: 10000, ExchangeCoins: "1.000000", Status: "generated", GenAddress: nessGenesisAddress}
	a.store.Slots["nvs"] = Slot{Status: "generated", Payments: map[string]Payment{"NCH": {Address: testNESS}}}
	if !a.reservedAddress(testNESS) || !a.reservedAddress(nessGenesisAddress) {
		t.Fatal("deposit not reserved")
	}
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path != "/api/v1/wallet/balance" {
			t.Error("signing attempted without free liquidity")
		}
		fmt.Fprintf(w, `{"addresses":{%q:{"confirmed":{"coins":1000000},"predicted":{"coins":1000000}},%q:{"confirmed":{"coins":999999999},"predicted":{"coins":999999999}}}}`, testChange, nessGenesisAddress)
	}))
	defer server.Close()
	a.cfg.NESSURL = server.URL
	if _, e := a.payAmount(testNESS, "0.000001", 0, ""); e == nil {
		t.Fatal("reserved liquidity spent")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}
