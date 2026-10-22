package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const claimWait = 8 * time.Second

var txPattern = regexp.MustCompile(`^[a-fA-F0-9]{64}$`)
var exampleAddresses = map[string]bool{"2PdinZNcNzpDbzkggBnEcV4zJgexztpHJ9j": true, "EbRUXJ9zqWsioCexdEpq3HcD7WioxtxFP4": true}

func (a *App) tokenMessage(t int64) []byte {
	return []byte(time.Unix(t, 0).In(a.zone).Format("05:04:15 2006-01-02") + " Privateness POT")
}
func (a *App) sign(t int64) string {
	return strconv.FormatInt(t, 10) + "_" + base64.StdEncoding.EncodeToString(ed25519.Sign(a.key, a.tokenMessage(t)))
}
func (a *App) verify(token string) (int64, error) {
	parts := strings.Split(token, "_")
	if len(parts) != 2 {
		return 0, bad("Token must be UNIXTIME_SIGNATURE.")
	}
	t, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || t <= 0 || t > a.now().Unix() || strconv.FormatInt(t, 10) != parts[0] {
		return 0, bad("Invalid token timestamp.")
	}
	sig, e := base64.StdEncoding.Strict().DecodeString(parts[1])
	if e != nil || len(sig) != 64 || !ed25519.Verify(a.key.Public().(ed25519.PublicKey), a.tokenMessage(t), sig) {
		return 0, bad("This token could not be verified.")
	}
	if a.now().Unix()-t < int64(claimWait/time.Second) {
		return 0, bad("The time token is not ready yet.")
	}
	return t, nil
}

// Preserve the original nearest-redeemed-token progression, without an unbounded loop.
func (a *App) credited(t int64) int64 {
	age := a.now().Unix() - t
	nearest := age
	for k := range a.store.Redemptions {
		v, e := strconv.ParseInt(k, 10, 64)
		if e != nil {
			continue
		}
		d := v - t
		if d < 0 {
			d = -d
		}
		if d < nearest {
			nearest = d
		}
	}
	if nearest <= 0 || age <= 0 {
		return 0
	}
	level := int64(math.Ceil((math.Sqrt(1+8*float64(age)/float64(nearest)) - 1) / 2))
	if level < 1 {
		level = 1
	}
	result := int64(math.Round(float64(age) / float64(level)))
	if result < nearest {
		result = nearest
	}
	return result
}
func (a *App) quote(t int64) (map[string]any, error) {
	seconds := a.credited(t)
	coins, hours, err := a.reward(seconds)
	if err != nil {
		return nil, err
	}
	return map[string]any{"seconds": seconds, "ness": coins, "nch": hours}, nil
}
func (a *App) faucet(id string, s Session, in map[string]json.RawMessage) (any, int, error) {
	action, err := textField(in, "action", 32)
	if err != nil {
		return nil, 400, err
	}
	now := a.now()
	claim := s.Claim
	switch action {
	case "start":
		// Every start replaces the prior claim and begins a full server-side wait.
		claim = Claim{ID: randomID(24), TokenTime: now.Unix(), AvailableAt: now.Add(claimWait).UnixMilli(), ExpiresAt: now.Add(15 * time.Minute).UnixMilli()}
		s.Claim = claim
		if err = a.store.session(id, s); err != nil {
			return nil, 503, err
		}
		return claimStatus(claim, now), 200, nil
	case "release":
		cid, e := textField(in, "claim_id", 48)
		if e != nil {
			return nil, 400, e
		}
		if claim.ID == "" || cid != claim.ID || now.UnixMilli() >= claim.ExpiresAt {
			return nil, 400, bad("Claim expired or belongs to another session. Start again.")
		}
		if claim.Released {
			return nil, 409, &apiError{409, "This claim was already released. Start a fresh eight-second claim; tokens are not cached for retrieval."}
		}
		result := claimStatus(claim, now)
		if now.UnixMilli() < claim.AvailableAt {
			return result, 425, nil
		}
		claim.Released = true
		s.Claim = claim
		if err = a.store.session(id, s); err != nil {
			return nil, 503, err
		}
		result["token"] = a.sign(claim.TokenTime)
		return result, 200, nil
	case "quote", "withdraw":
		token, e := textField(in, "token", 160)
		if e != nil {
			return nil, 400, e
		}
		t, e := a.verify(token)
		if e != nil {
			return nil, 400, e
		}
		key := strconv.FormatInt(t, 10)
		if existing, ok := a.store.Redemptions[key]; ok {
			address, _ := textField(in, "address", 64)
			if action == "withdraw" && existing.Address == address {
				code := 409
				if existing.State == "submitted" {
					code = 200
				}
				return map[string]any{"redemption": existing}, code, nil
			}
			return nil, 409, &apiError{409, "Token already redeemed or awaiting payment review."}
		}
		quote, e := a.quote(t)
		if e != nil {
			return nil, 400, bad("The requested amount exceeds the supported range.")
		}
		if action == "quote" {
			return map[string]any{"quote": quote}, 200, nil
		}
		address, e := textField(in, "address", 64)
		if e != nil || !validNESSAddress(address) {
			return nil, 400, bad("Enter your own valid NESS receiving address.")
		}
		if a.cfg.NESSURL == "" || a.cfg.NESSWallet == "" {
			return nil, 503, &apiError{503, "The operator must configure the NESS wallet before cash-out is available."}
		}
		if e = a.checkNESSNode(); e != nil {
			return nil, 503, &apiError{503, e.Error()}
		}
		redemption := Redemption{State: "review", Address: address, Seconds: quote["seconds"].(int64)}
		if e = a.store.redemption(key, redemption); e != nil {
			return nil, 503, e
		}
		tx, e := a.pay(address, redemption.Seconds)
		if e != nil {
			return nil, 503, &apiError{503, "Payment needs operator review. Do not resubmit this token. Reference: " + key}
		}
		redemption.State = "submitted"
		redemption.Txid = tx
		if e = a.store.redemption(key, redemption); e != nil {
			return nil, 503, e
		}
		return map[string]any{"redemption": redemption, "quote": quote}, 200, nil
	default:
		return nil, 400, bad("Unknown action.")
	}
}
func claimStatus(c Claim, now time.Time) map[string]any {
	remaining := c.AvailableAt - now.UnixMilli()
	if remaining < 0 {
		remaining = 0
	}
	return map[string]any{"claim_id": c.ID, "remaining_ms": remaining}
}
