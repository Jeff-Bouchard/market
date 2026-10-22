package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testApp(t *testing.T) *App {
	t.Helper()
	a, e := newApp(Config{DataDir: t.TempDir(), CoinsPerSecond: "0.000001", HoursPerSecond: 1, EMCPrice: "0.01", NESSPrice: "0.1", NCHPrice: "1"})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(a.store.Close)
	return a
}
func input(s string) map[string]json.RawMessage {
	var m map[string]json.RawMessage
	json.Unmarshal([]byte(s), &m)
	return m
}
func TestPortalListenPorts(t *testing.T) {
	if got := testApp(t).cfg.Listen; got != "127.0.0.1:8787" {
		t.Fatalf("default listener = %q", got)
	}
	for _, address := range []string{"127.0.0.1:8080", ":8000", "[::1]:8080", "0.0.0.0:08000"} {
		if _, err := newApp(Config{Listen: address}); err == nil || !strings.Contains(err.Error(), "reserved for IPFS and Skywire") {
			t.Errorf("reserved listener %q: %v", address, err)
		}
	}
}
func TestClaimDeadlineAndSignature(t *testing.T) {
	a := testApp(t)
	now := time.Unix(1700000000, 123000000)
	a.now = func() time.Time { return now }
	s := Session{CSRF: "test", Expires: now.Add(time.Hour).Unix()}
	a.store.session("one", s)
	v, _, e := a.faucet("one", s, input(`{"action":"start"}`))
	if e != nil {
		t.Fatal(e)
	}
	claim := v.(map[string]any)
	if _, ok := claim["token"]; ok {
		t.Fatal("token leaked before deadline")
	}
	if claim["remaining_ms"] != int64(8000) {
		t.Fatal(claim)
	}
	s = a.store.Sessions["one"]
	release := input(`{"action":"release","claim_id":"` + s.Claim.ID + `"}`)
	now = now.Add(7999 * time.Millisecond)
	_, code, _ := a.faucet("one", s, release)
	if code != 425 {
		t.Fatal(code)
	}
	now = now.Add(time.Millisecond)
	v, code, e = a.faucet("one", s, release)
	if e != nil || code != 200 {
		t.Fatal(code, e)
	}
	token := v.(map[string]any)["token"].(string)
	if _, e = a.verify(token); e != nil {
		t.Fatal(e)
	}
	if _, e = a.verify(token[:len(token)-4] + "AAAA"); e == nil {
		t.Fatal("accepted tampered token")
	}
	if _, _, e = a.faucet("two", Session{}, release); e == nil {
		t.Fatal("foreign release accepted")
	}
}
func TestEveryClaimRequiresFreshWait(t *testing.T) {
	a := testApp(t)
	now := time.Unix(1700000000, 0)
	a.now = func() time.Time { return now }
	s := Session{CSRF: "test", Expires: now.Add(time.Hour).Unix()}
	start := func() map[string]any {
		t.Helper()
		v, code, err := a.faucet("fresh", s, input(`{"action":"start"}`))
		if err != nil || code != 200 {
			t.Fatal(code, err)
		}
		s = a.store.Sessions["fresh"]
		if v.(map[string]any)["remaining_ms"] != int64(8000) {
			t.Fatal("start did not reset the full wait")
		}
		return v.(map[string]any)
	}
	release := func(cid string) (any, int, error) {
		return a.faucet("fresh", a.store.Sessions["fresh"], input(`{"action":"release","claim_id":"`+cid+`"}`))
	}
	first := start()["claim_id"].(string)
	now = now.Add(9 * time.Second)
	second := start()["claim_id"].(string)
	if first == second {
		t.Fatal("reused claim")
	}
	if _, _, err := release(first); err == nil {
		t.Fatal("superseded claim accepted")
	}
	now = now.Add(time.Second)
	if _, code, err := release(second); err != nil || code != 425 {
		t.Fatal("old deadline bypassed fresh wait", code, err)
	}
	now = now.Add(9 * time.Second)
	if _, code, err := release(second); err != nil || code != 200 {
		t.Fatal(code, err)
	}
	if _, code, err := release(second); err == nil || code != 409 {
		t.Fatal("released token was retrieved again")
	}
	s = a.store.Sessions["fresh"]
	third := start()["claim_id"].(string)
	if _, code, err := release(third); err != nil || code != 425 {
		t.Fatal("released claim bypassed fresh wait", code, err)
	}
}
func TestHTTPAndDefault(t *testing.T) {
	a := testApp(t)
	handler := a.handler()
	r := httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("GET", "http://127.0.0.1:8787/", nil))
	html := r.Body.String()
	if !strings.Contains(r.Header().Get("Cache-Control"), "no-store") {
		t.Fatal("homepage permits caching")
	}
	if !strings.Contains(html, `spellcheck="false">`+defaultNS+`</textarea>`) {
		t.Fatal("NS not a real default")
	}
	if strings.Contains(html, "<?") {
		t.Fatal("PHP remains")
	}
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, httptest.NewRequest("GET", "http://127.0.0.1:8787/api/faucet", nil))
	var session map[string]any
	json.Unmarshal(r.Body.Bytes(), &session)
	if session["wait_seconds"] != float64(8) {
		t.Fatal("API wait does not match the eight-second claim", session["wait_seconds"])
	}
	cookies := r.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly {
		t.Fatal("missing secure session attributes")
	}
	req := httptest.NewRequest("POST", "http://127.0.0.1:8787/api/faucet", strings.NewReader(`{"action":"start"}`))
	req.AddCookie(cookies[0])
	req.Header.Set("Content-Type", "application/json")
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, req)
	if r.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	req = httptest.NewRequest("POST", "http://127.0.0.1:8787/api/faucet", strings.NewReader(`{"action":"start"}`))
	req.AddCookie(cookies[0])
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", session["csrf"].(string))
	req.Header.Set("Origin", "https://attacker.invalid")
	r = httptest.NewRecorder()
	handler.ServeHTTP(r, req)
	if r.Code != 403 {
		t.Fatal("foreign origin accepted")
	}
}
func TestPersistenceAndLock(t *testing.T) {
	dir := t.TempDir()
	s, e := openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = openStore(dir); e == nil {
		t.Fatal("second writer allowed")
	}
	s.slot(Slot{ID: "a", Session: "owner", Name: "dns:test.emc", Status: "review"})
	s.redemption("123", Redemption{State: "review", Address: "a"})
	s.Close()
	s, e = openStore(dir)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.Slots["a"].Session != "owner" || s.Redemptions["123"].State != "review" {
		t.Fatal("durable state lost")
	}
}
func TestNVSDefaultPriceAndReplay(t *testing.T) {
	a := testApp(t)
	paid := 0.01
	writes := 0
	receivedValue := ""
	tx := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var call struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		json.NewDecoder(r.Body).Decode(&call)
		var result any
		switch call.Method {
		case "name_show":
			io.WriteString(w, `{"error":{"code":-4,"message":"not found"},"result":null}`)
			return
		case "listwallets":
			result = []string{"test"}
		case "getwalletinfo":
			result = map[string]bool{"private_keys_enabled": true}
		case "getnewaddress":
			result = testEMC
		case "getreceivedbyaddress":
			result = paid
		case "name_new":
			writes++
			json.Unmarshal(call.Params[1], &receivedValue)
			result = tx
		default:
			t.Error(call.Method)
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result, "error": nil})
	}))
	defer server.Close()
	a.cfg.EMCURL = server.URL
	v, code, e := a.exchange("owner", input(`{"action":"create","kind":"nvs","name":"dns:test.emc","owner":"","days":"200"}`))
	if e != nil || code != 201 {
		t.Fatal(code, e)
	}
	slot := v.(Slot)
	if slot.Value != defaultNS || slot.Payments["EMC"].Minimum != "0.02000000" {
		t.Fatal(slot)
	}
	request := input(`{"action":"status","kind":"nvs","slot_id":"` + slot.ID + `","process":true}`)
	if _, _, e = a.exchange("other", request); e == nil {
		t.Fatal("foreign slot allowed")
	}
	a.exchange("owner", request)
	if writes != 0 {
		t.Fatal("underpayment accepted")
	}
	paid = 0.02
	if _, _, e = a.exchange("owner", request); e != nil {
		t.Fatal(e)
	}
	a.exchange("owner", request)
	if writes != 1 || receivedValue != defaultNS {
		t.Fatal(writes, receivedValue)
	}
}
func TestUncertainPaymentNotRetried(t *testing.T) {
	a := testApp(t)
	a.cfg.NESSWallet = "wallet"
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(503) }))
	defer server.Close()
	a.cfg.NESSURL = server.URL
	token := a.sign(a.now().Add(-20 * time.Second).Unix())
	request := input(`{"action":"withdraw","token":"` + token + `","address":"` + testNESS + `"}`)
	a.faucet("owner", Session{}, request)
	a.faucet("owner", Session{}, request)
	if calls != 2 || len(a.store.Redemptions) != 0 {
		t.Fatal("preflight must not consume a token", calls)
	}
}
