package wallet

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func (g *Gateway) getBalance(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, format, ok := g.parseCommon(c)
	if !ok {
		return
	}
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidAddress, "address is required"))
		return
	}

	balance, err := g.service.GetBalance(c.Request.Context(), chain, address)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}

	if format == FormatMetalet {
		if chain == ChainBTC {
			c.JSON(http.StatusOK, NewMetaletBTCBalanceResponse(balance))
			return
		}
		c.JSON(http.StatusOK, NewMetaletMVCDOGEBalanceResponse(balance))
		return
	}
	c.JSON(http.StatusOK, NewStandardBalanceResponse(balance))
}

func (g *Gateway) getUTXOs(c *gin.Context) {
	if !g.ensureService(c) {
		return
	}
	chain, format, ok := g.parseCommon(c)
	if !ok {
		return
	}
	address := strings.TrimSpace(c.Param("address"))
	if address == "" {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidAddress, "address is required"))
		return
	}

	confirmedOnly, err := parseBoolDefault(c.Query("confirmedOnly"), false)
	if err != nil {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "confirmedOnly must be true or false"))
		return
	}
	sortOrder, sortErr := parseSortDefault(c.DefaultQuery("sort", "desc"))
	if sortErr != nil {
		g.writeWalletError(c, sortErr)
		return
	}

	utxos, err := g.service.GetUTXOs(c.Request.Context(), chain, address)
	if err != nil {
		g.writeServiceError(c, err)
		return
	}
	normalized, err := NormalizeUTXOs(utxos, UTXOOptions{ConfirmedOnly: confirmedOnly, Sort: sortOrder})
	if err != nil {
		g.writeServiceError(c, err)
		return
	}

	if format == FormatMetalet {
		c.JSON(http.StatusOK, NewMetaletUTXOResponse(chain, normalized))
		return
	}
	c.JSON(http.StatusOK, NewStandardUTXOResponse(chain, address, confirmedOnly, sortOrder, normalized))
}

func (g *Gateway) parseCommon(c *gin.Context) (Chain, ResponseFormat, bool) {
	chain, ok := NormalizeChain(c.Param("chain"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusNotFound, CodeUnsupportedChain, "unsupported chain"))
		return "", "", false
	}
	format, ok := NormalizeFormat(c.Query("format"))
	if !ok {
		g.writeWalletError(c, NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "format must be standard or metalet"))
		return "", "", false
	}
	return chain, format, true
}

func (g *Gateway) ensureService(c *gin.Context) bool {
	if g != nil && g.service != nil {
		return true
	}
	g.writeWalletError(c, NewHTTPWalletError(http.StatusServiceUnavailable, CodeCoreUnavailable, "wallet service is not configured"))
	return false
}

func (g *Gateway) writeServiceError(c *gin.Context, err error) {
	var walletErr *WalletError
	if errors.As(err, &walletErr) {
		if walletErr.HTTPStatus == 0 {
			walletErr.HTTPStatus = http.StatusInternalServerError
		}
		g.writeWalletError(c, publicServiceError(walletErr))
		return
	}
	g.writeWalletError(c, NewHTTPWalletError(http.StatusInternalServerError, CodeInternal, "internal wallet error"))
}

func (g *Gateway) writeWalletError(c *gin.Context, err *WalletError) {
	status := http.StatusInternalServerError
	if err != nil && err.HTTPStatus != 0 {
		status = err.HTTPStatus
	}
	c.JSON(status, ErrorEnvelope(err))
}

func parseBoolDefault(raw string, defaultValue bool) (bool, error) {
	if strings.TrimSpace(raw) == "" {
		return defaultValue, nil
	}
	return strconv.ParseBool(raw)
}

func parseSortDefault(raw string) (string, *WalletError) {
	sortOrder := strings.ToLower(strings.TrimSpace(raw))
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		return "", NewHTTPWalletError(http.StatusBadRequest, CodeInvalidQuery, "sort must be asc or desc")
	}
	return sortOrder, nil
}

func publicServiceError(err *WalletError) *WalletError {
	message := "internal wallet error"
	switch err.Code {
	case CodeCoreUnavailable:
		message = "core unavailable"
	case CodeInvalidUpstream:
		message = "invalid upstream response"
	case CodeInternal:
		message = "internal wallet error"
	}
	return NewHTTPWalletError(err.HTTPStatus, err.Code, message)
}
