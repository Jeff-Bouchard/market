package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

type Claim struct {
	ID          string
	TokenTime   int64
	AvailableAt int64
	ExpiresAt   int64
	Released    bool
}
type Session struct {
	CSRF    string
	Expires int64
	Claim   Claim
}
type Redemption struct {
	State   string `json:"state"`
	Address string `json:"address"`
	Seconds int64  `json:"seconds"`
	Txid    string `json:"txid,omitempty"`
}
type Payment struct {
	Address string  `json:"addr"`
	Minimum Decimal `json:"min_sum"`
	Days    int     `json:"days"`
}
type Slot struct {
	BrowserPaymentState string              `json:"browser_payment_state,omitempty"`
	BrowserPaymentTx    string              `json:"browser_payment_tx,omitempty"`
	BrowserPaymentRaw   string              `json:"browser_payment_raw,omitempty"`
	BrowserSource       string              `json:"browser_source,omitempty"`
	BrowserTemplate     *createdTransaction `json:"browser_template,omitempty"`
	ID                  string              `json:"slot_id"`
	Session             string              `json:"-"`
	Kind                string              `json:"kind"`
	Name                string              `json:"name"`
	Value               string              `json:"value"`
	Owner               string              `json:"owner,omitempty"`
	Days                int                 `json:"days"`
	Status              string              `json:"status"`
	Created             int64               `json:"created"`
	Payments            map[string]Payment  `json:"payments"`
	Address             string              `json:"address,omitempty"`
	PayAddress          string              `json:"pay_address,omitempty"`
	GenAddress          string              `json:"gen_address,omitempty"`
	Hours               float64             `json:"hours,omitempty"`
	Receive             float64             `json:"recieve,omitempty"`
	Error               string              `json:"error,omitempty"`
	Txid                string              `json:"txid,omitempty"`
	ExchangeRate        uint64              `json:"exchange_rate,omitempty"`
	ExchangeHours       uint64              `json:"exchange_nch,omitempty,string"`
	ExchangeCoins       Decimal             `json:"exchange_ness,omitempty"`
}
type event struct {
	Type         string
	ID           string
	Session      *Session
	Redemption   *Redemption
	Slot         *Slot
	OwnerSession string
}
type Store struct {
	file        *os.File
	lock        string
	failed      bool
	Sessions    map[string]Session
	Redemptions map[string]Redemption
	Slots       map[string]Slot
}

func openStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	lock := filepath.Join(dir, "server.lock")
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, fmt.Errorf("data directory is locked; stop the other server, or remove %s only after confirming no server is running: %w", lock, err)
	}
	fmt.Fprintln(f, os.Getpid())
	f.Close()
	s := &Store{lock: lock, Sessions: map[string]Session{}, Redemptions: map[string]Redemption{}, Slots: map[string]Slot{}}
	s.file, err = os.OpenFile(filepath.Join(dir, "state.jsonl"), os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		os.Remove(lock)
		return nil, err
	}
	reader := bufio.NewReader(s.file)
	for n := 1; ; n++ {
		line, e := reader.ReadBytes('\n')
		if e == io.EOF && len(line) == 0 {
			break
		}
		if e != nil {
			s.Close()
			return nil, fmt.Errorf("incomplete journal at line %d; restore a verified backup before restarting", n)
		}
		var v event
		if err = json.Unmarshal(line, &v); err != nil {
			s.Close()
			return nil, fmt.Errorf("invalid journal line %d: %w", n, err)
		}
		if err = s.apply(v); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}
func (s *Store) apply(e event) error {
	switch e.Type {
	case "session":
		if e.Session == nil {
			return errors.New("invalid session event")
		}
		s.Sessions[e.ID] = *e.Session
	case "redemption":
		if e.Redemption == nil {
			return errors.New("invalid redemption event")
		}
		s.Redemptions[e.ID] = *e.Redemption
	case "slot":
		if e.Slot == nil {
			return errors.New("invalid slot event")
		}
		v := *e.Slot
		v.Session = e.OwnerSession
		s.Slots[e.ID] = v
	default:
		return errors.New("unknown journal event")
	}
	return nil
}

// Callers hold the application mutex. Sync BEFORE acknowledging or performing external writes.
func (s *Store) save(e event) error {
	if s.failed {
		return errors.New("storage is unavailable; operator review required")
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	n, err := s.file.Write(b)
	if err == nil && n != len(b) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = s.file.Sync()
	}
	if err != nil {
		s.failed = true
		return errors.New("durable storage failed; stop and review the server")
	}
	return s.apply(e)
}
func (s *Store) session(id string, v Session) error {
	return s.save(event{Type: "session", ID: id, Session: &v})
}
func (s *Store) redemption(id string, v Redemption) error {
	return s.save(event{Type: "redemption", ID: id, Redemption: &v})
}
func (s *Store) slot(v Slot) error {
	return s.save(event{Type: "slot", ID: v.ID, Slot: &v, OwnerSession: v.Session})
}
func (s *Store) Close() {
	if s.file != nil {
		s.file.Close()
	}
	os.Remove(s.lock)
}
