package api

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/metaid/utxo_indexer/api/respond"
	"github.com/metaid/utxo_indexer/storage"
)

// getNftAddressUtxos gets NFT UTXO list by address
func (s *NftServer) getNftAddressUtxos(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	// Get pagination parameters
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get NFT UTXOs
	utxos, total, nextCursor, err := s.indexer.GetNftUTXOsByAddress(address, codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUTXOsResponse{
		Address:    address,
		UTXOs:      utxos,
		Total:      total,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, time.Now().UnixMilli()-startTime))
}

// getNftGenesisUtxos gets NFT UTXO list by codeHash and genesis
func (s *NftServer) getNftGenesisUtxos(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get tokenIndex filter parameters
	tokenIndexStr := c.Query("tokenIndex")
	tokenIndexMinStr := c.Query("tokenIndexMin")
	tokenIndexMaxStr := c.Query("tokenIndexMax")

	var tokenIndex, tokenIndexMin, tokenIndexMax uint64
	var hasTokenIndex, hasTokenIndexMin, hasTokenIndexMax bool

	if tokenIndexStr != "" {
		val, err := strconv.ParseUint(tokenIndexStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndex parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndex = val
		hasTokenIndex = true
	}

	if tokenIndexMinStr != "" {
		val, err := strconv.ParseUint(tokenIndexMinStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndexMin parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndexMin = val
		hasTokenIndexMin = true
	}

	if tokenIndexMaxStr != "" {
		val, err := strconv.ParseUint(tokenIndexMaxStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndexMax parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndexMax = val
		hasTokenIndexMax = true
	}

	// Get NFT UTXOs
	utxos, err := s.indexer.GetNftUTXOsByCodeHashGenesis(codeHash, genesis, hasTokenIndex, tokenIndex, hasTokenIndexMin, tokenIndexMin, hasTokenIndexMax, tokenIndexMax)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisUTXOsResponse{
		CodeHash: codeHash,
		Genesis:  genesis,
		UTXOs:    utxos,
		Total:    len(utxos),
	}, time.Now().UnixMilli()-startTime))
}

// getNftAddressSellUtxos gets NFT sell UTXO list by address
func (s *NftServer) getNftAddressSellUtxos(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	// Get pagination parameters
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get NFT sell UTXOs
	utxos, total, nextCursor, err := s.indexer.GetNftSellUTXOsByAddress(address, codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftSellUTXOsResponse{
		Address:    address,
		UTXOs:      utxos,
		Total:      total,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, time.Now().UnixMilli()-startTime))
}

// getNftGenesisSellUtxos gets NFT sell UTXO list by codeHash and genesis
func (s *NftServer) getNftGenesisSellUtxos(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get tokenIndex filter parameters
	tokenIndexStr := c.Query("tokenIndex")
	tokenIndexMinStr := c.Query("tokenIndexMin")
	tokenIndexMaxStr := c.Query("tokenIndexMax")

	var tokenIndex, tokenIndexMin, tokenIndexMax uint64
	var hasTokenIndex, hasTokenIndexMin, hasTokenIndexMax bool

	if tokenIndexStr != "" {
		val, err := strconv.ParseUint(tokenIndexStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndex parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndex = val
		hasTokenIndex = true
	}

	if tokenIndexMinStr != "" {
		val, err := strconv.ParseUint(tokenIndexMinStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndexMin parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndexMin = val
		hasTokenIndexMin = true
	}

	if tokenIndexMaxStr != "" {
		val, err := strconv.ParseUint(tokenIndexMaxStr, 10, 64)
		if err != nil {
			c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("invalid tokenIndexMax parameter"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
			return
		}
		tokenIndexMax = val
		hasTokenIndexMax = true
	}

	// Get NFT sell UTXOs
	utxos, err := s.indexer.GetNftSellUTXOsByCodeHashGenesis(codeHash, genesis, hasTokenIndex, tokenIndex, hasTokenIndexMin, tokenIndexMin, hasTokenIndexMax, tokenIndexMax)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisSellUTXOsResponse{
		CodeHash: codeHash,
		Genesis:  genesis,
		UTXOs:    utxos,
		Total:    len(utxos),
	}, time.Now().UnixMilli()-startTime))
}

// getNftAddressUtxoCount gets NFT UTXO count by address
func (s *NftServer) getNftAddressUtxoCount(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get NFT UTXO count
	count, err := s.indexer.GetNftUtxoCountByAddress(address)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAddressUtxoCountResponse{
		Address: address,
		Count:   count,
	}, time.Now().UnixMilli()-startTime))
}

// getNftAddressSummary gets NFT address summary
func (s *NftServer) getNftAddressSummary(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get pagination parameters
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get NFT address summary
	summaries, total, nextCursor, err := s.indexer.GetNftAddressSummary(address, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAddressSummaryResponse{
		Address:    address,
		Summary:    summaries,
		Total:      total,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, time.Now().UnixMilli()-startTime))
}

// getNftSummary gets NFT summary list
func (s *NftServer) getNftSummary(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get pagination parameters
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get NFT summary data
	nftInfos, total, nextCursor, err := s.indexer.GetNftSummary(cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftSummaryResponse{
		Summary:    nftInfos,
		Total:      total,
		Cursor:     cursor,
		NextCursor: nextCursor,
		Size:       size,
	}, time.Now().UnixMilli()-startTime))
}

// getDbNftUtxoByTx gets NFT UTXO by transaction ID
func (s *NftServer) getDbNftUtxoByTx(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	tx := c.Query("tx")
	if tx == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("tx parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	utxos, err := s.indexer.GetDbNftUtxoByTx(tx)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUtxoByTxResponse{
		UTXOs: string(utxos),
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllNftUtxo gets all NFT UTXO data with pagination
func (s *NftServer) getDbAllNftUtxo(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	key := c.Query("key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAllNftUtxo(key, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"data": data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAddressNftIncomeValid gets valid NFT income data for specified address
func (s *NftServer) getDbAddressNftIncomeValid(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	incomeData, total, totalPages, err := s.indexer.GetDbAddressNftIncomeValid(address, codeHash, genesis, page, pageSize)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftIncomeValidResponse{
				Address:    address,
				IncomeData: []string{},
				Pagination: struct {
					CurrentPage int `json:"current_page"`
					PageSize    int `json:"page_size"`
					Total       int `json:"total"`
					TotalPages  int `json:"total_pages"`
				}{
					CurrentPage: page,
					PageSize:    pageSize,
					Total:       0,
					TotalPages:  0,
				},
			}, time.Now().UnixMilli()-startTime))
			return
		}
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftIncomeValidResponse{
		Address:    address,
		IncomeData: incomeData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAddressNftIncome gets NFT income data for specified address
func (s *NftServer) getDbAddressNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAddressNftIncome(address, codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"address": address,
		"income":  data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAddressNftSpend gets NFT spend data for specified address
func (s *NftServer) getDbAddressNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAddressNftSpend(address, codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"address": address,
		"spend":   data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbCodeHashGenesisNftIncome gets NFT income data by codeHash and genesis
func (s *NftServer) getDbCodeHashGenesisNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbCodeHashGenesisNftIncome(codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"codeHash": codeHash,
		"genesis":  genesis,
		"income":   data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbCodeHashGenesisNftSpend gets NFT spend data by codeHash and genesis
func (s *NftServer) getDbCodeHashGenesisNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbCodeHashGenesisNftSpend(codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"codeHash": codeHash,
		"genesis":  genesis,
		"spend":    data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAddressSellNftIncome gets NFT sell income data for specified address
func (s *NftServer) getDbAddressSellNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAddressSellNftIncome(address, codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"address": address,
		"income":  data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAddressSellNftSpend gets NFT sell spend data for specified address
func (s *NftServer) getDbAddressSellNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAddressSellNftSpend(address, codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"address": address,
		"spend":   data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbCodeHashGenesisSellNftIncome gets NFT sell income data by codeHash and genesis
func (s *NftServer) getDbCodeHashGenesisSellNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbCodeHashGenesisSellNftIncome(codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"codeHash": codeHash,
		"genesis":  genesis,
		"income":   data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbCodeHashGenesisSellNftSpend gets NFT sell spend data by codeHash and genesis
func (s *NftServer) getDbCodeHashGenesisSellNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbCodeHashGenesisSellNftSpend(codeHash, genesis, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"codeHash": codeHash,
		"genesis":  genesis,
		"spend":    data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllNftInfo gets all NFT info data with pagination
func (s *NftServer) getDbAllNftInfo(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	key := c.Query("key")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	data, total, totalPages, err := s.indexer.GetDbAllNftInfo(key, page, pageSize)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"data": data,
		"pagination": gin.H{
			"current_page": page,
			"page_size":    pageSize,
			"total":        total,
			"total_pages":  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getNftGenesis gets NFT genesis information
func (s *NftServer) getNftGenesis(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get NFT genesis information
	nftGenesisInfo, err := s.indexer.GetNftGenesis(codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisInfoResponse{
		CodeHash:    nftGenesisInfo.CodeHash,
		Genesis:     nftGenesisInfo.Genesis,
		SensibleId:  nftGenesisInfo.SensibleId,
		Txid:        nftGenesisInfo.Txid,
		TxIndex:     nftGenesisInfo.TxIndex,
		TokenSupply: nftGenesisInfo.TokenSupply,
		TokenIndex:  nftGenesisInfo.TokenIndex,
		ValueString: nftGenesisInfo.ValueString,
		Height:      nftGenesisInfo.Height,
	}, time.Now().UnixMilli()-startTime))
}

// getNftOwners gets NFT owners list by codeHash and genesis
func (s *NftServer) getNftOwners(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get pagination parameters
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get NFT owners information
	ownerInfo, err := s.indexer.GetNftOwners(codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftOwnersResponse{
		List:       ownerInfo.List,
		Total:      ownerInfo.Total,
		Cursor:     ownerInfo.Cursor,
		NextCursor: ownerInfo.NextCursor,
		Size:       ownerInfo.Size,
	}, time.Now().UnixMilli()-startTime))
}

// getAllDbUncheckNftOutpoint gets unchecked NFT outpoint data
func (s *NftServer) getAllDbUncheckNftOutpoint(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get outpoint parameter
	outpoint := c.Query("outpoint")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Get data
	data, err := s.indexer.GetAllDbUncheckNftOutpoint(outpoint)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If outpoint is provided, return result directly
	if outpoint != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUncheckOutpointResponse{
			Data: data,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(data)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = data[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUncheckOutpointResponse{
		Data: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getAllDbNftGenesis gets all NFT Genesis data
func (s *NftServer) getAllDbNftGenesis(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter
	key := c.Query("key")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Get data
	data, err := s.indexer.GetAllDbNftGenesis(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisResponse{
			Data: data,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(data)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = data[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisResponse{
		Data: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getAllDbNftGenesisOutput gets all NFT Genesis Output data
func (s *NftServer) getAllDbNftGenesisOutput(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter
	key := c.Query("key")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Get data
	data, err := s.indexer.GetAllDbNftGenesisOutput(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisOutputResponse{
			Data: data,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(data)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string][]string)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = data[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftGenesisOutputResponse{
		Data: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getAllDbUsedNftIncome gets all used NFT income data
func (s *NftServer) getAllDbUsedNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter
	txId := c.Query("txId")

	// Get pagination parameters
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Get data
	data, err := s.indexer.GetAllDbUsedNftIncome(txId)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if txId != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUsedIncomeResponse{
			Data: data,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(data)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = data[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftUsedIncomeResponse{
		Data: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getMempoolAddressNftSpendMap gets NFT spend data for address in mempool
func (s *NftServer) getMempoolAddressNftSpendMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")

	spendMap, err := s.indexer.GetMempoolAddressNftSpendMap(address)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftSpendMapResponse{
		Address:  address,
		SpendMap: spendMap,
	}, time.Now().UnixMilli()-startTime))
}

// getMempoolAddressNftIncomeMap gets NFT income data for addresses in mempool
// If address parameter is provided, returns data for that address only; otherwise returns all addresses
func (s *NftServer) getMempoolAddressNftIncomeMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get optional address parameter
	address := c.Query("address")

	incomeMap := s.indexer.GetMempoolAddressNftIncomeMap(address)

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAddressNftIncomeMapResponse{
		Address:   address,
		IncomeMap: incomeMap,
	}, time.Now().UnixMilli()-startTime))
}

// getMempoolAddressNftIncomeValidMap gets valid NFT income data for addresses in mempool
// If address parameter is provided, returns data for that address only; otherwise returns all addresses
func (s *NftServer) getMempoolAddressNftIncomeValidMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get optional address parameter
	address := c.Query("address")

	// Get data
	incomeValidMap := s.indexer.GetMempoolAddressNftIncomeValidMap(address)

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAddressNftIncomeValidMapResponse{
		Address:        address,
		IncomeValidMap: incomeValidMap,
	}, time.Now().UnixMilli()-startTime))
}

// getDbInvalidNftOutpoint gets invalid NFT contract UTXO data
func (s *NftServer) getDbInvalidNftOutpoint(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	outpoint := c.Query("outpoint")

	if outpoint == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("outpoint parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Query invalid NFT contract UTXO data
	value, err := s.indexer.QueryInvalidNftOutpoint(outpoint)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	if value == "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
			"outpoint": outpoint,
			"data":     nil,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Parse returned data
	// Format: NftAddress@CodeHash@Genesis@sensibleId@TokenIndex@TxID@Index@Value@TokenSupply@MetaTxId@MetaOutputIndex@height@reason
	parts := strings.Split(value, "@")
	if len(parts) != 13 {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(errors.New("invalid data format"), time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"outpoint": outpoint,
		"data": gin.H{
			"nft_address":       parts[0],
			"code_hash":         parts[1],
			"genesis":           parts[2],
			"sensible_id":       parts[3],
			"token_index":       parts[4],
			"tx_id":             parts[5],
			"index":             parts[6],
			"value":             parts[7],
			"token_supply":      parts[8],
			"meta_tx_id":        parts[9],
			"meta_output_index": parts[10],
			"height":            parts[11],
			"reason":            parts[12],
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllAddressSellNftIncome gets all address NFT sell income data
func (s *NftServer) getDbAllAddressSellNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter (address)
	key := c.Query("key")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	incomeData, err := s.indexer.GetAllDbAddressSellNftIncome(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellIncomeResponse{
			IncomeData: incomeData,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	if incomeData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellIncomeResponse{
			IncomeData: make(map[string]string),
			Pagination: struct {
				CurrentPage int `json:"current_page"`
				PageSize    int `json:"page_size"`
				Total       int `json:"total"`
				TotalPages  int `json:"total_pages"`
			}{
				CurrentPage: page,
				PageSize:    pageSize,
				Total:       0,
				TotalPages:  0,
			},
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(incomeData)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(incomeData))
	for k := range incomeData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = incomeData[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellIncomeResponse{
		IncomeData: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllAddressSellNftSpend gets all address NFT sell spend data
func (s *NftServer) getDbAllAddressSellNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter (address)
	key := c.Query("key")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	spendData, err := s.indexer.GetAllDbAddressSellNftSpend(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellSpendResponse{
			SpendData: spendData,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	if spendData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellSpendResponse{
			SpendData: make(map[string]string),
			Pagination: struct {
				CurrentPage int `json:"current_page"`
				PageSize    int `json:"page_size"`
				Total       int `json:"total"`
				TotalPages  int `json:"total_pages"`
			}{
				CurrentPage: page,
				PageSize:    pageSize,
				Total:       0,
				TotalPages:  0,
			},
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(spendData)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(spendData))
	for k := range spendData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = spendData[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftAllSellSpendResponse{
		SpendData: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllCodeHashGenesisSellNftIncome gets all NFT sell income data by codeHash@genesis
func (s *NftServer) getDbAllCodeHashGenesisSellNftIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter (codeHash@genesis)
	key := c.Query("key")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	incomeData, err := s.indexer.GetAllDbCodeHashGenesisSellNftIncome(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellIncomeResponse{
			IncomeData: incomeData,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	if incomeData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellIncomeResponse{
			IncomeData: make(map[string]string),
			Pagination: struct {
				CurrentPage int `json:"current_page"`
				PageSize    int `json:"page_size"`
				Total       int `json:"total"`
				TotalPages  int `json:"total_pages"`
			}{
				CurrentPage: page,
				PageSize:    pageSize,
				Total:       0,
				TotalPages:  0,
			},
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(incomeData)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(incomeData))
	for k := range incomeData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = incomeData[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellIncomeResponse{
		IncomeData: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}

// getDbAllCodeHashGenesisSellNftSpend gets all NFT sell spend data by codeHash@genesis
func (s *NftServer) getDbAllCodeHashGenesisSellNftSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get key parameter (codeHash@genesis)
	key := c.Query("key")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	spendData, err := s.indexer.GetAllDbCodeHashGenesisSellNftSpend(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellSpendResponse{
			SpendData: spendData,
		}, time.Now().UnixMilli()-startTime))
		return
	}

	if spendData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellSpendResponse{
			SpendData: make(map[string]string),
			Pagination: struct {
				CurrentPage int `json:"current_page"`
				PageSize    int `json:"page_size"`
				Total       int `json:"total"`
				TotalPages  int `json:"total_pages"`
			}{
				CurrentPage: page,
				PageSize:    pageSize,
				Total:       0,
				TotalPages:  0,
			},
		}, time.Now().UnixMilli()-startTime))
		return
	}

	// Calculate pagination
	total := len(spendData)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]string)
	keys := make([]string, 0, len(spendData))
	for k := range spendData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = spendData[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.NftCodeHashGenesisSellSpendResponse{
		SpendData: currentPageData,
		Pagination: struct {
			CurrentPage int `json:"current_page"`
			PageSize    int `json:"page_size"`
			Total       int `json:"total"`
			TotalPages  int `json:"total_pages"`
		}{
			CurrentPage: page,
			PageSize:    pageSize,
			Total:       total,
			TotalPages:  totalPages,
		},
	}, time.Now().UnixMilli()-startTime))
}
