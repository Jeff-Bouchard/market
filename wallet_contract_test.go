package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

const testNESS = "27nAhbBjHLcvD3UdbrH1YouKWYwmG94K9cw" // upstream wallet fixture, never used outside fake services
const testChange = "2THDupTBEo7UqB6dsVizkYUvkKq82Qn4gjf"

func TestHostRebindingRejected(t *testing.T) {
	a := testApp(t)
	for _, host := range []string{"attacker.invalid:8787", "127.0.0.1:9999"} {
		r := httptest.NewRequest("GET", "http://"+host+"/api/faucet", nil)
		w := httptest.NewRecorder()
		a.handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatal("untrusted host accepted", host)
		}
	}
}

var testEMC = func() string {
	b := append([]byte{33}, bytes.Repeat([]byte{3}, 20)...)
	h := sha256.Sum256(b)
	h = sha256.Sum256(h[:])
	return encodeBase58(append(b, h[:4]...))
}()

func serveNodeIdentity(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/api/v1/health":
		json.NewEncoder(w).Encode(map[string]any{"wallet_api_enabled": true, "blockchain_pubkey": nessBlockchainKey, "fiber": map[string]string{"ticker": "NESS", "coin_hours_ticker": "NCH"}, "user_verify_transaction": map[string]int{"burn_factor": 20, "max_decimals": 6}, "unconfirmed_verify_transaction": map[string]int{"burn_factor": 20, "max_decimals": 6}})
		return true
	case "/api/v1/blocks":
		if r.URL.RawQuery != "start=0&end=0" {
			w.WriteHeader(400)
			return true
		}
		fmt.Fprintf(w, `{"blocks":[{"header":{"seq":0,"timestamp":1650046010},"body":{"txns":[{"outputs":[{"dst":%q,"coins":"165000000.000000"}]}]}}]}`, nessGenesisAddress)
		return true
	}
	return false
}
func TestNESSBurnFactor20(t *testing.T) {
	for _, tc := range []struct{ input, fee uint64 }{{0, 0}, {1, 1}, {19, 1}, {20, 1}, {21, 2}, {100, 5}, {101, 6}, {200, 10}, {^uint64(0), 922337203685477581}} {
		if v := requiredNESSFee(tc.input); v != tc.fee {
			t.Fatalf("input %d: got %d want %d", tc.input, v, tc.fee)
		}
	}
}
func TestExactAmounts(t *testing.T) {
	for _, tc := range []struct {
		s    string
		p    int
		want uint64
	}{{"0.1", 6, 100000}, {"0.000001", 6, 1}, {"1e-6", 6, 1}, {"9007199254740993", 0, 9007199254740993}, {"0.00000001", 8, 1}} {
		var d Decimal
		if e := json.Unmarshal([]byte(tc.s), &d); e != nil {
			t.Fatal(e)
		}
		u, e := d.units(tc.p)
		if e != nil || u != tc.want {
			t.Fatal(d, u, e)
		}
	}
	for _, s := range []Decimal{"-1", "0.0000001", "NaN", "9223372036854775808"} {
		if _, e := s.units(6); e == nil {
			t.Fatal("invalid amount accepted", s)
		}
	}
	if _, e := multiply(9223372036854775807, 2); e == nil {
		t.Fatal("overflow accepted")
	}
}
func TestAddressChecks(t *testing.T) {
	for _, s := range []string{testNESS, testChange, nessGenesisAddress} {
		if !validNESSAddress(s) {
			t.Fatal("valid NESS rejected", s)
		}
	}
	if !validEMCAddress(testEMC) {
		t.Fatal("valid EMC rejected")
	}
	for _, s := range []string{testNESS[:len(testNESS)-1] + "1", testEMC, strings.Repeat("N", 34), "2PdinZNcNzpDbzkggBnEcV4zJgexztpHJ9j"} {
		if validNESSAddress(s) {
			t.Fatal("invalid NESS accepted", s)
		}
	}
	if validEMCAddress(testNESS) {
		t.Fatal("NESS accepted as EMC")
	}
}
func TestUpstreamGoldenContracts(t *testing.T) {
	raw, e := os.ReadFile("testdata/upstream/balance-two-addrs.golden")
	if e != nil {
		t.Fatal(e)
	}
	var b balanceResponse
	if e = json.Unmarshal(raw, &b); e != nil || b.Confirmed.Coins != 1022100000000 || b.Confirmed.Hours != 1013748655 {
		t.Fatal(b, e)
	}
	var encoded struct {
		Encoded string `json:"encoded_transaction"`
	}
	raw, _ = os.ReadFile("testdata/upstream/transaction-encoded.golden")
	if e = json.Unmarshal(raw, &encoded); e != nil {
		t.Fatal(e)
	}
	parsed, e := decodeTransaction(encoded.Encoded)
	if e != nil {
		t.Fatal(e)
	}
	var readable struct {
		Transaction struct {
			ID      string   `json:"txid"`
			Inputs  []string `json:"inputs"`
			Outputs []struct {
				Address string  `json:"dst"`
				Coins   Decimal `json:"coins"`
				Hours   uint64  `json:"hours"`
			} `json:"outputs"`
		} `json:"txn"`
	}
	raw, _ = os.ReadFile("testdata/upstream/transaction-block-517.golden")
	if e = json.Unmarshal(raw, &readable); e != nil {
		t.Fatal(e)
	}
	if parsed.ID != readable.Transaction.ID || len(parsed.Outputs) != len(readable.Transaction.Outputs) {
		t.Fatal("upstream encoded/readable mismatch")
	}
	for i, o := range parsed.Outputs {
		expected := readable.Transaction.Outputs[i]
		coins, e := expected.Coins.units(6)
		if e != nil || o.Address != expected.Address || o.Coins != coins || o.Hours != expected.Hours {
			t.Fatal("upstream output mismatch")
		}
	}
}

// Produce synthetic signed-shaped bytes ONLY for mock-node response validation tests.
// Signature authenticity is tested by the node, not fabricated here.
func mockCreated(address string, coins, hours, inputHours, fee uint64, totalCoins ...uint64) createdTransaction {
	inputCoins := uint64(1000000)
	if len(totalCoins) > 0 {
		inputCoins = totalCoins[0]
	}
	var tx createdTransaction
	buf := new(bytes.Buffer)
	binary.Write(buf, binary.LittleEndian, uint32(0))
	buf.WriteByte(0)
	buf.Write(make([]byte, 32))
	binary.Write(buf, binary.LittleEndian, uint32(1))
	buf.Write(bytes.Repeat([]byte{1}, 65))
	innerStart := buf.Len()
	binary.Write(buf, binary.LittleEndian, uint32(1))
	buf.Write(bytes.Repeat([]byte{2}, 32))
	outputs := []decodedOutput{{address, coins, hours}, {testChange, inputCoins - coins, inputHours - hours - fee}}
	binary.Write(buf, binary.LittleEndian, uint32(len(outputs)))
	for _, o := range outputs {
		b := decodeBase58(o.Address)
		buf.WriteByte(b[20])
		buf.Write(b[:20])
		binary.Write(buf, binary.LittleEndian, o.Coins)
		binary.Write(buf, binary.LittleEndian, o.Hours)
		tx.Transaction.Outputs = append(tx.Transaction.Outputs, createdOutput{o.Address, scaledDecimal(o.Coins, 6), strconv.FormatUint(o.Hours, 10)})
	}
	raw := buf.Bytes()
	binary.LittleEndian.PutUint32(raw[:4], uint32(len(raw)))
	inner := sha256.Sum256(raw[innerStart:])
	copy(raw[5:37], inner[:])
	outer := sha256.Sum256(raw)
	tx.Encoded = hex.EncodeToString(raw)
	tx.Transaction.Txid = hex.EncodeToString(outer[:])
	tx.Transaction.Fee = strconv.FormatUint(fee, 10)
	tx.Transaction.Inputs = []createdInput{{UxID: strings.Repeat("02", 32), Address: testChange, Coins: scaledDecimal(inputCoins, 6), Hours: strconv.FormatUint(inputHours, 10)}}
	return tx
}
func TestTransactionValidation(t *testing.T) {
	owned := map[string]bool{testChange: true}
	tx := mockCreated(testNESS, 20, 20, 100, 5)
	if e := validateCreated(tx, testNESS, 20, 20, owned); e != nil {
		t.Fatal(e)
	}
	if e := validateCreated(mockCreated(testNESS, 20, 20, 100, 4), testNESS, 20, 20, owned); e == nil {
		t.Fatal("underpaid 5% burn accepted")
	}
	if e := validateCreated(mockCreated(testNESS, 20, 20, 21, 1), testNESS, 20, 20, owned); e == nil {
		t.Fatal("rounding not enforced")
	}
	if e := validateCreated(mockCreated(testNESS, 20, 19, 21, 2), testNESS, 20, 19, owned); e != nil {
		t.Fatal(e)
	}
	tx.Transaction.Outputs[0].Coins = "0.000021"
	if e := validateCreated(tx, testNESS, 20, 20, owned); e == nil {
		t.Fatal("description mismatch accepted")
	}
	tx = mockCreated(testChange, 20, 20, 100, 5)
	if e := validateCreated(tx, testNESS, 20, 20, owned); e == nil {
		t.Fatal("wrong recipient accepted")
	}
	tx = mockCreated(testNESS, 20, 20, 100, 5)
	if e := validateCreated(tx, testNESS, 20, 20, map[string]bool{}); e == nil {
		t.Fatal("foreign change accepted")
	}
}
func TestWalletContractsAndNoDuplicateBroadcast(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		for _, uncertain := range []bool{false, true} {
			t.Run(fmt.Sprintf("csrfDisabled=%v/uncertain=%v", disabled, uncertain), func(t *testing.T) {
				a := testApp(t)
				a.cfg.NESSWallet = "wallet.wlt"
				a.cfg.NESSPassword = "wallet-secret"
				a.cfg.NESSAPIUser = "api-user"
				a.cfg.NESSAPIPassword = "api-secret"
				broadcasts := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					u, p, ok := r.BasicAuth()
					if !ok || u != "api-user" || p != "api-secret" {
						t.Error("missing node API auth")
						w.WriteHeader(401)
						return
					}
					if serveNodeIdentity(w, r) {
						return
					}
					if r.Method == "POST" && !disabled && r.Header.Get("X-CSRF-Token") != "csrf-token" {
						t.Error("missing CSRF")
						w.WriteHeader(403)
						return
					}
					switch r.URL.Path {
					case "/api/v1/csrf":
						if disabled {
							w.WriteHeader(404)
						} else {
							fmt.Fprint(w, `{"csrf_token":"csrf-token"}`)
						}
					case "/api/v1/wallet/newAddress":
						if e := r.ParseForm(); e != nil {
							t.Error(e)
						}
						if r.Form.Get("id") != "wallet.wlt" || r.Form.Get("num") != "1" || r.Form.Get("password") != "wallet-secret" {
							t.Error("newAddress form mismatch")
						}
						fmt.Fprintf(w, `{"addresses":[%q]}`, testChange)
					case "/api/v1/wallet/balance":
						if r.URL.Query().Get("id") != "wallet.wlt" {
							t.Error("wallet balance id mismatch")
						}
						fmt.Fprintf(w, `{"confirmed":{"coins":1000000,"hours":100},"predicted":{"coins":1000000,"hours":100},"addresses":{%q:{"confirmed":{"coins":1000000,"hours":100}}}}`, testChange)
					case "/api/v1/wallet/transaction":
						var req walletTransactionRequest
						if e := json.NewDecoder(r.Body).Decode(&req); e != nil {
							t.Error(e)
						}
						if req.WalletID != "wallet.wlt" || req.Password != "wallet-secret" || req.HoursSelection.Type != "manual" || len(req.To) != 1 || req.To[0].Coins != "0.000020" || req.To[0].Hours != "20" || req.ChangeAddress != testChange {
							t.Errorf("transaction request mismatch: %+v", req)
						}
						json.NewEncoder(w).Encode(mockCreated(testNESS, 20, 20, 100, 5))
					case "/api/v1/injectTransaction":
						broadcasts++
						var body map[string]string
						json.NewDecoder(r.Body).Decode(&body)
						tx := mockCreated(testNESS, 20, 20, 100, 5)
						if body["rawtx"] != tx.Encoded {
							t.Error("rawtx mismatch")
						}
						if uncertain {
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
				if _, e := a.newNESSAddress(); e != nil {
					t.Fatal(e)
				}
				now := time.Unix(1700000020, 0)
				a.now = func() time.Time { return now }
				token := a.sign(now.Unix() - 20)
				request := input(`{"action":"withdraw","token":"` + token + `","address":"` + testNESS + `"}`)
				_, _, e := a.faucet("owner", Session{}, request)
				if !uncertain && e != nil {
					t.Fatal(e)
				}
				if uncertain && e == nil {
					t.Fatal("unknown broadcast accepted")
				}
				a.faucet("owner", Session{}, request)
				if broadcasts != 1 {
					t.Fatal("broadcast repeated", broadcasts)
				}
				expected := "submitted"
				if uncertain {
					expected = "review"
				}
				if a.store.Redemptions["1700000000"].State != expected {
					t.Fatal(a.store.Redemptions)
				}
			})
		}
	}
}
func TestNodeMismatchDoesNotConsumeToken(t *testing.T) {
	a := testApp(t)
	a.cfg.NESSWallet = "wallet"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"wallet_api_enabled":true,"fiber":{"ticker":"SKY"}}`)
	}))
	defer server.Close()
	a.cfg.NESSURL = server.URL
	token := a.sign(a.now().Add(-20 * time.Second).Unix())
	_, _, e := a.faucet("owner", Session{}, input(`{"action":"withdraw","token":"`+token+`","address":"`+testNESS+`"}`))
	if e == nil || len(a.store.Redemptions) != 0 {
		t.Fatal("wrong node consumed a token")
	}
}

func TestNodeFeePrecisionAndLegacyHealth(t *testing.T) {
	for _, tc := range []struct {
		name           string
		burn, decimals int
		key            string
		ok             bool
	}{
		{"NESS-current", 20, 6, nessBlockchainKey, true},
		{"NESS-legacy-health", 20, 6, "", true},
		{"stock-Skycoin-fee", 10, 6, nessBlockchainKey, false},
		{"stock-Skycoin-precision", 20, 3, nessBlockchainKey, false},
		{"wrong-key", 20, 6, strings.Repeat("a", 66), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := testApp(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/v1/health" {
					fmt.Fprintf(w, `{"wallet_api_enabled":true,"blockchain_pubkey":%q,"fiber":{"ticker":"NESS","coin_hours_ticker":"NCH"},"user_verify_transaction":{"burn_factor":%d,"max_decimals":%d},"unconfirmed_verify_transaction":{"burn_factor":%d,"max_decimals":%d}}`, tc.key, tc.burn, tc.decimals, tc.burn, tc.decimals)
					return
				}
				if !serveNodeIdentity(w, r) {
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			a.cfg.NESSURL = server.URL
			if e := a.checkNESSNode(); (e == nil) != tc.ok {
				t.Fatal(tc.name, e)
			}
		})
	}
}
func TestMixedNodePolicyExplainsFixBeforeWalletWrite(t *testing.T) {
	a := testApp(t)
	a.cfg.NESSWallet = "wallet"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || r.URL.Path != "/api/v1/health" {
			t.Error("unexpected node operation", r.Method, r.URL.Path)
		}
		fmt.Fprint(w, `{"wallet_api_enabled":true,"fiber":{"ticker":"NESS","coin_hours_ticker":"NCH"},"user_verify_transaction":{"burn_factor":10,"max_transaction_size":32768,"max_decimals":3},"unconfirmed_verify_transaction":{"burn_factor":20,"max_transaction_size":32768,"max_decimals":6}}`)
	}))
	defer server.Close()
	a.cfg.NESSURL = server.URL
	token := a.sign(a.now().Add(-20 * time.Second).Unix())
	_, _, err := a.faucet("owner", Session{}, input(`{"action":"withdraw","token":"`+token+`","address":"`+testNESS+`"}`))
	if err == nil {
		t.Fatal("mixed node policy accepted")
	}
	for _, detail := range []string{"burn_factor=10", "max_decimals=3", "USER_BURN_FACTOR=20", "USER_MAX_DECIMALS=6"} {
		if !strings.Contains(err.Error(), detail) {
			t.Errorf("missing diagnostic %q in %v", detail, err)
		}
	}
	if len(a.store.Redemptions) != 0 {
		t.Fatal("preflight failure spent token")
	}
}
func TestPaidUnitsAndUnderpayment(t *testing.T) {
	a := testApp(t)
	coins := uint64(299999)
	hours := uint64(2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveNodeIdentity(w, r) {
			return
		}
		fmt.Fprintf(w, `{"confirmed":{"coins":%d,"hours":%d}}`, coins, hours)
	}))
	defer server.Close()
	a.cfg.NESSURL = server.URL
	p := Payment{testNESS, "0.300000", 300}
	paid, e := a.paid("NESS", p)
	if e != nil || paid {
		t.Fatal("underpayment", paid, e)
	}
	coins = 300000
	paid, e = a.paid("NESS", p)
	if e != nil || !paid {
		t.Fatal("exact payment", paid, e)
	}
	p.Minimum = "3"
	paid, e = a.paid("NCH", p)
	if e != nil || paid {
		t.Fatal("NCH underpayment", paid, e)
	}
	hours = 3
	coins = 0
	paid, e = a.paid("NCH", p)
	if e != nil || paid {
		t.Fatal("NCH without droplet", paid, e)
	}
	coins = 1
	paid, e = a.paid("NCH", p)
	if e != nil || !paid {
		t.Fatal("NCH payment", paid, e)
	}
}
