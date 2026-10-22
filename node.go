package main

import (
	"errors"
	"fmt"
)

// Pinned to ness-network/ness fiber.toml at 9dd55e9ee8fec7de9e195cce7d8c98fb922ae648.
const nessBlockchainKey = "02933015bd2fa1e0a885c05fb08eb7c647bf8c3188ed5120b51d0d09ccaf525036"
const nessGenesisAddress = "24GJTLPMoz61sV4J4qg1n14x5qqDwXqyJJy"

type verifyPolicy struct {
	BurnFactor  uint32 `json:"burn_factor"`
	MaxDecimals uint8  `json:"max_decimals"`
}
type nodeHealth struct {
	WalletEnabled bool         `json:"wallet_api_enabled"`
	BlockchainKey string       `json:"blockchain_pubkey"`
	User          verifyPolicy `json:"user_verify_transaction"`
	Unconfirmed   verifyPolicy `json:"unconfirmed_verify_transaction"`
	Fiber         struct {
		Ticker      string `json:"ticker"`
		HoursTicker string `json:"coin_hours_ticker"`
	} `json:"fiber"`
}

func (a *App) checkNESSNode() error {
	var health nodeHealth
	if e := a.ness("/api/v1/health", &health); e != nil {
		return fmt.Errorf("NESS node check failed: %w", e)
	}
	if health.Fiber.Ticker != "NESS" || health.Fiber.HoursTicker != "NCH" {
		return errors.New("configured node is not reporting NESS/NCH")
	}
	if !health.WalletEnabled {
		return errors.New("enable the node WALLET API set")
	}
	if health.User.BurnFactor != 20 || health.User.MaxDecimals != 6 {
		return fmt.Errorf("NESS wallet policy reports burn_factor=%d, max_decimals=%d; expected 20 (1/20 = 5%%) and 6. Restart the NESS NODE with USER_BURN_FACTOR=20 and USER_MAX_DECIMALS=6 in its environment. Setting these on the faucet does not change the node. Your token has not been spent", health.User.BurnFactor, health.User.MaxDecimals)
	}
	if health.Unconfirmed.BurnFactor != 20 || health.Unconfirmed.MaxDecimals != 6 {
		return fmt.Errorf("NESS network policy reports burn_factor=%d, max_decimals=%d; expected 20 and 6. Check the node's NESS fiber configuration and burn-factor-unconfirmed/max-decimals-unconfirmed flags. Your token has not been spent", health.Unconfirmed.BurnFactor, health.Unconfirmed.MaxDecimals)
	}
	if health.BlockchainKey != "" && health.BlockchainKey != nessBlockchainKey {
		return errors.New("node blockchain public key does not match NESS")
	}
	// The old NESS health response lacks blockchain_pubkey. Verify the genesis data
	// for both old and new nodes, rather than relying on branding or a port number.
	var result struct {
		Blocks []struct {
			Header struct {
				Sequence  uint64 `json:"seq"`
				Timestamp uint64 `json:"timestamp"`
			} `json:"header"`
			Body struct {
				Transactions []struct {
					Outputs []struct {
						Address string  `json:"dst"`
						Coins   Decimal `json:"coins"`
					} `json:"outputs"`
				} `json:"txns"`
			} `json:"body"`
		} `json:"blocks"`
	}
	if e := a.ness("/api/v1/blocks?start=0&end=0", &result); e != nil {
		return fmt.Errorf("NESS genesis check failed: %w", e)
	}
	if len(result.Blocks) != 1 {
		return errors.New("missing NESS genesis block")
	}
	b := result.Blocks[0]
	if b.Header.Sequence != 0 || b.Header.Timestamp != 1650046010 || len(b.Body.Transactions) != 1 || len(b.Body.Transactions[0].Outputs) != 1 {
		return errors.New("genesis does not match NESS")
	}
	out := b.Body.Transactions[0].Outputs[0]
	coins, e := out.Coins.units(6)
	if e != nil || out.Address != nessGenesisAddress || coins != 165000000000000 {
		return errors.New("genesis output does not match NESS")
	}
	return nil
}
