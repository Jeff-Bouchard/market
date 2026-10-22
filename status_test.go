package main

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHelpfulStatusResponses(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{errors.New("unlock the selected Emercoin wallet for transactions"), 503, "EMC_WALLET_LOCKED"},
		{&rpcError{-19, "Wallet file not specified"}, 503, "EMC_WALLET_SELECTION"},
		{errors.New("upstream connection failed"), 503, "SERVICE_UNAVAILABLE"},
		{&apiError{400, "Enter your own valid NESS receiving address."}, 400, "CHECK_INPUT"},
		{&apiError{503, "Payment needs operator review. Reference: test"}, 503, "OPERATOR_REVIEW"},
	} {
		got := explainError(tc.err, tc.status)
		if got.Code != tc.code || got.Title == "" || got.NextStep == "" {
			t.Fatalf("%+v", got)
		}
	}
	for _, state := range []string{"generated", "submitted", "payed", "review", "cancelled", "unexpected"} {
		w := httptest.NewRecorder()
		respond(w, 200, Slot{ID: "request", Status: state})
		var got struct {
			Detail statusDetail `json:"status_detail"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &got); e != nil {
			t.Fatal(e)
		}
		if got.Detail.Code == "" || got.Detail.NextStep == "" {
			t.Fatal(state, w.Body.String())
		}
		if state == "review" && !strings.Contains(got.Detail.NextStep, "Do not send another payment") {
			t.Fatal("unsafe review instruction")
		}
	}
	w := httptest.NewRecorder()
	respond(w, 403, map[string]string{"error": "Session expired. Refresh and try again."})
	if !strings.Contains(w.Body.String(), "SESSION_OR_ACCESS") {
		t.Fatal(w.Body.String())
	}
}
