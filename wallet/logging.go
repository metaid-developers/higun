package wallet

import (
	"log"
	"net/http"
	"time"
)

func logWalletUpstream(req *http.Request, status int, start time.Time, errMessage string) {
	if req == nil || req.URL == nil {
		return
	}
	duration := time.Since(start).Milliseconds()
	address := truncateAddressForLog(req.URL.Query().Get("address"))
	if errMessage != "" {
		log.Printf("wallet upstream request host=%s path=%s status=%d duration_ms=%d address=%s error=%q", req.URL.Host, req.URL.Path, status, duration, address, errMessage)
		return
	}
	log.Printf("wallet upstream request host=%s path=%s status=%d duration_ms=%d address=%s", req.URL.Host, req.URL.Path, status, duration, address)
}

func truncateAddressForLog(address string) string {
	if len(address) <= 12 {
		return address
	}
	return address[:6] + "..." + address[len(address)-6:]
}
