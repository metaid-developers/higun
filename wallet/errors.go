package wallet

import (
	"fmt"
	"net/http"
)

const (
	CodeSuccess            = 0
	CodeUnsupportedChain   = -4001
	CodeInvalidAddress     = -4002
	CodeInvalidQuery       = -4003
	CodeInvalidRawTx       = -4004
	CodeTxNotFound         = -4041
	CodeCoreUnavailable    = -5001
	CodeInvalidUpstream    = -5002
	CodeInternal           = -5003
	CodeBroadcastRejected  = -5004
	CodeFeeRateUnavailable = -5005
)

type WalletError struct {
	Code       int
	Message    string
	HTTPStatus int
}

func (e *WalletError) Error() string {
	return fmt.Sprintf("%d: %s", e.Code, e.Message)
}

func NewWalletError(code int, message string) *WalletError {
	return NewHTTPWalletError(http.StatusInternalServerError, code, message)
}

func NewHTTPWalletError(status int, code int, message string) *WalletError {
	return &WalletError{Code: code, Message: message, HTTPStatus: status}
}
