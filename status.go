package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type statusDetail struct {
	Code     string `json:"code"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	NextStep string `json:"next_step"`
}

func slotStatus(s Slot) statusDetail {
	switch s.Status {
	case "generated", "updated":
		if s.BrowserPaymentState == "submitted" {
			return statusDetail{"PAYMENT_SUBMITTED", "Your payment is on its way", "Your approved payment has been sent and is awaiting confirmation.", "This page will check confirmation before registering your record. Do not pay again."}
		}
		if s.BrowserPaymentState == "review" {
			return statusDetail{"PAYMENT_REVIEW", "Checking your payment result", "The submission result is uncertain. Your transaction reference is saved.", "Check this request or ask the operator to check its transaction. Do not pay again."}
		}
		if s.Kind == "nvs" {
			return statusDetail{"AWAITING_PAYMENT", "Review your record and cost", "Your record has not been registered yet. Check the information and who will control it before using your balance.", "Choose Review and use my balance to continue. If you already sent a payment, choose Check my payment instead."}
		}
		return statusDetail{"AWAITING_PAYMENT", "Your payment instructions are ready", "No completed payment has been recorded for this request yet. A payment still awaiting confirmation will not start the exchange.", "Follow the instructions below once. If you have already paid, check its confirmation before continuing. Do not pay again."}
	case "submitted":
		return statusDetail{"PAYOUT_SUBMITTED", "Your payout has been submitted", "The NESS node accepted the payout transaction. This is not yet proof of network confirmation.", "Open your receiving wallet and wait for the transaction to confirm. Keep the transaction reference below; no more deposit is needed."}
	case "payed":
		return statusDetail{"REGISTRATION_SUBMITTED", "Your record is being registered", "Your registration has been sent. Confirmation may still be pending.", "Keep the reference below and choose Check my record to see its progress. Do not pay again."}
	case "review", "error":
		return statusDetail{"OPERATOR_REVIEW", "Your request needs a person to check it", "The service cannot safely complete or confirm this request automatically. It has stopped automatic retries to avoid a duplicate payment.", "Do not send another payment. Contact the service operator with the request reference and your wallet transaction reference. They must reconcile the payment before continuing."}
	case "cancelled":
		return statusDetail{"REQUEST_CANCELLED", "This request is cancelled", "The unused quote no longer reserves exchange funds. Its old deposit address must not be used.", "Create a new request if you still want to exchange. If you already sent to the cancelled address, contact the operator with its reference."}
	case "activated":
		return statusDetail{"LEGACY_EXCHANGE_ACTIVE", "Your earlier exchange is active", "This request belongs to the previous exchange service.", "Follow its deposit instructions and check again after confirmation. Do not start a second request for the same payment."}
	case "done":
		if s.Kind == "nvs" {
			return statusDetail{"NVS_CONFIRMED", "Your record is registered", "The registered name, public information and owner match your request.", "Keep your recovery words and receipt. This site does not yet offer record changes or renewals; those use the owning Emercoin wallet."}
		}
		return statusDetail{"EXCHANGE_COMPLETE", "Your exchange service reports completion", "The exchange service has reported that the request is complete.", "Check the receiving wallet and keep your transaction reference."}
	default:
		return statusDetail{"STATUS_UNKNOWN", "We need to check this request", "The service returned an unfamiliar request status.", "Do not pay again. Contact the operator with the request reference below."}
	}
}

func explainError(err error, httpStatus int) statusDetail {
	s := statusDetail{fmt.Sprintf("HTTP_%d", httpStatus), http.StatusText(httpStatus), "The request could not be completed.", "Check the information you entered. If you have already paid, keep your reference and contact the operator before trying again."}
	var ae *apiError
	if errors.As(err, &ae) {
		s.Message = ae.message
	}
	msg := strings.ToLower(err.Error())
	var rpc *rpcError
	switch {
	case errors.As(err, &rpc) && (rpc.Code == -19 || rpc.Code == -18):
		s = statusDetail{"EMC_WALLET_SELECTION", "The registration wallet is unavailable", "Emercoin could not select the configured wallet.", "The operator must load the selected wallet and check the wallet setting. If you already paid, keep your request reference and do not pay again."}
	case (errors.As(err, &rpc) && rpc.Code == -13) || strings.Contains(msg, "unlock") || strings.Contains(msg, "wallet is locked"):
		s = statusDetail{"EMC_WALLET_LOCKED", "The registration wallet needs unlocking", "The service wallet cannot sign a name registration while locked.", "The operator must unlock the selected Emercoin wallet for transactions, then check this same request again. Do not send another payment."}
	case strings.Contains(msg, "private keys disabled"):
		s = statusDetail{"EMC_WALLET_READ_ONLY", "The registration wallet cannot sign", "The selected wallet can watch records but cannot create them.", "The operator must configure a wallet that can sign registrations. Keep any existing payment reference."}
	case strings.Contains(msg, "operator review") || strings.Contains(msg, "payment review") || strings.Contains(msg, "storage"):
		s = statusDetail{"OPERATOR_REVIEW", "Your request needs a person to check it", s.Message, "Do not send again. Contact the operator with the reference below and any payment transaction. An uncertain result must be reconciled first."}
	case strings.Contains(msg, "burn") || strings.Contains(msg, "decimals") || strings.Contains(msg, "genesis"):
		s = statusDetail{"NESS_NODE_POLICY", "Receiving is temporarily unavailable", "Your claim has not been used.", "Keep your claim code. The operator must correct the service settings before receiving can continue."}
	case strings.Contains(msg, "reserved") || strings.Contains(msg, "unreserved"):
		s = statusDetail{"EXCHANGE_LIQUIDITY", "The exchange needs more available NESS", "The wallet has insufficient unreserved funds for this request.", "Try a smaller quote or wait for the operator to fund the service. If a payment is already in review, contact the operator instead of depositing again."}
	case strings.Contains(msg, "configure"):
		s = statusDetail{"SERVICE_NOT_CONFIGURED", "This service is not ready yet", "The operator has not finished connecting the service.", "Keep your claim code or request reference. Setup must be completed before you can receive a balance or register a record."}
	case httpStatus == 403:
		s.Code = "SESSION_OR_ACCESS"
		s.Title = "We could not verify this browser session"
		s.NextStep = "Use the same browser that created the request. Save your claim code before refreshing. If you already paid, contact the operator with your reference rather than creating a new request."
	case httpStatus == 429:
		s.Code = "PLEASE_WAIT"
		s.Title = "A short pause is needed"
		s.NextStep = "Wait 30 seconds, then try once more. An existing request can still be checked."
	case httpStatus == 409:
		s.Code = "REQUEST_CONFLICT"
		s.Title = "This request already has a history"
		s.NextStep = "Read the explanation above and check the original request. Do not repeat a payment that may already have been submitted."
	case httpStatus == 400 || httpStatus == 415:
		s.Code = "CHECK_INPUT"
		s.Title = "Something needs correcting"
		s.NextStep = "Check the indicated field and try again. Copy wallet addresses directly from your own wallet."
	case httpStatus >= 500:
		s.Code = "SERVICE_UNAVAILABLE"
		s.Title = "We cannot reach or use the service right now"
		s.NextStep = "Keep your claim code or request reference. If payment was being submitted, ask the operator to check it before retrying. Otherwise try again later."
	}
	return s
}
