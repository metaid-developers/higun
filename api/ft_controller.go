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
	ft "github.com/metaid/utxo_indexer/indexer/contract/meta-contract-ft"
	"github.com/metaid/utxo_indexer/storage"
)

func (s *FtServer) getFtBalance(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	balances, err := s.indexer.GetFtBalance(address, codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtBalanceResponse{
		Balances: balances,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtUTXOs(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	utxos, err := s.indexer.GetFtUTXOs(address, codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUTXOsResponse{
		Address: address,
		UTXOs:   utxos,
		Count:   len(utxos),
	}, time.Now().UnixMilli()-startTime))
}

// DB
func (s *FtServer) getDbFtUtxoByTx(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	tx := c.Query("tx")
	if tx == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("tx parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	utxos, err := s.indexer.GetDbFtUtxoByTx(tx)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUtxoByTxResponse{
		UTXOs: string(utxos),
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getDbFtIncomeByAddress(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	income, err := s.indexer.GetDbAddressFtIncome(address, codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtIncomeResponse{
		Income: income,
	}, time.Now().UnixMilli()-startTime))
}

// db
func (s *FtServer) getDbFtSpendByAddress(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	spend, err := s.indexer.GetDbAddressFtSpend(address, codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSpendResponse{
		Spend: spend,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getDbUniqueFtIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	if codeHash == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	genesis := c.Query("genesis")
	if genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("genesis parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	income, err := s.indexer.GetDbUniqueFtIncome(codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUniqueFtIncomeResponse{
		CodeHash: codeHash,
		Genesis:  genesis,
		Income:   income,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getDbUniqueFtSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	if codeHash == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	genesis := c.Query("genesis")
	if genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("genesis parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	spend, err := s.indexer.GetDbUniqueFtSpend(codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUniqueFtSpendResponse{
		CodeHash: codeHash,
		Genesis:  genesis,
		Spend:    spend,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtMempoolUTXOs(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	income, spend, err := s.indexer.GetMempoolFtUTXOs(address, codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// Convert common.FtUtxo to ft.FtUTXO
	incomeUTXOs := make([]*ft.FtUTXO, 0, len(income))
	for _, utxo := range income {
		txIndex, err := strconv.ParseInt(utxo.Index, 10, 64)
		if err != nil {
			continue
		}
		incomeUTXOs = append(incomeUTXOs, &ft.FtUTXO{
			CodeHash:      utxo.CodeHash,
			Genesis:       utxo.Genesis,
			Name:          utxo.Name,
			Symbol:        utxo.Symbol,
			SensibleId:    utxo.SensibleId,
			Decimal:       uint8(0), // Need to get from elsewhere
			Txid:          utxo.TxID,
			TxIndex:       txIndex,
			ValueString:   utxo.Amount,
			SatoshiString: utxo.Value,
			Value:         0, // Need to convert
			Satoshi:       0, // Need to convert
			Height:        0,
			Address:       utxo.Address,
			Flag:          "unconfirmed",
		})
	}

	spendUTXOs := make([]*ft.FtUTXO, 0, len(spend))
	for _, utxo := range spend {
		txIndex, err := strconv.ParseInt(utxo.Index, 10, 64)
		if err != nil {
			continue
		}
		spendUTXOs = append(spendUTXOs, &ft.FtUTXO{
			CodeHash:      utxo.CodeHash,
			Genesis:       utxo.Genesis,
			Name:          utxo.Name,
			Symbol:        utxo.Symbol,
			SensibleId:    utxo.SensibleId,
			Decimal:       uint8(0), // Need to get from elsewhere
			Txid:          utxo.TxID,
			TxIndex:       txIndex,
			ValueString:   utxo.Amount,
			SatoshiString: utxo.Value,
			Value:         0, // Need to convert
			Satoshi:       0, // Need to convert
			Height:        0,
			Address:       utxo.Address,
			Flag:          "unconfirmed",
		})
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtMempoolUTXOsResponse{
		Address: address,
		Income:  incomeUTXOs,
		Spend:   spendUTXOs,
		Count:   len(incomeUTXOs) + len(spendUTXOs),
	}, time.Now().UnixMilli()-startTime))
}

// getAllFtIncome gets FT income data for all addresses
func (s *FtServer) getDbAllFtIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	incomeData, err := s.indexer.GetAllDbAddressFtIncome()
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	if incomeData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAllIncomeResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAllIncomeResponse{
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

// getAllFtSpend gets FT spend data for all addresses
func (s *FtServer) getDbAllFtSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	spendData, err := s.indexer.GetAllDbAddressFtSpend()
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	if spendData == nil {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAllSpendResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAllSpendResponse{
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

// getAddressFtIncome gets FT income data for specified address
func (s *FtServer) getDbAddressFtIncome(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	incomeData, err := s.indexer.GetDbAddressFtIncome(address, codeHash, genesis)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtIncomeResponse{
				Income: []string{},
			}, time.Now().UnixMilli()-startTime))
			return
		}
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtIncomeResponse{
		Income: incomeData,
	}, time.Now().UnixMilli()-startTime))
}

// getAddressFtSpend gets FT spend data for specified address
func (s *FtServer) getDbAddressFtSpend(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")
	if address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("address parameter is required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	spendData, err := s.indexer.GetDbAddressFtSpend(address, codeHash, genesis)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSpendResponse{
				Spend: []string{},
			}, time.Now().UnixMilli()-startTime))
			return
		}
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSpendResponse{
		Spend: spendData,
	}, time.Now().UnixMilli()-startTime))
}

// getFtInfo gets FT information
func (s *FtServer) getDbFtInfo(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Build query key
	key := codeHash + "@" + genesis

	// Get FT information
	ftInfo, err := s.indexer.GetFtInfo(key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtInfoResponse{
				CodeHash: codeHash,
				Genesis:  genesis,
			}, time.Now().UnixMilli()-startTime))
			return
		}
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	response := respond.FtInfoResponse{
		CodeHash:   ftInfo.CodeHash,
		Genesis:    ftInfo.Genesis,
		SensibleId: ftInfo.SensibleId,
		Name:       ftInfo.Name,
		Symbol:     ftInfo.Symbol,
		Decimal:    ftInfo.Decimal,
	}
	c.JSONP(http.StatusOK, respond.RespSuccess(response, time.Now().UnixMilli()-startTime))
}

// getDbAddressFtIncomeValid gets valid FT income data for specified address
func (s *FtServer) getDbAddressFtIncomeValid(c *gin.Context) {
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

	incomeData, err := s.indexer.GetDbAddressFtIncomeValid(address, codeHash, genesis)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtIncomeValidResponse{
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
	var currentPageData []string
	if start < total {
		currentPageData = incomeData[start:end]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtIncomeValidResponse{
		Address:    address,
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

func (s *FtServer) getAllDbUncheckFtOutpoint(c *gin.Context) {
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
	data, err := s.indexer.GetAllDbUncheckFtOutpoint(outpoint)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If outpoint is provided, return result directly
	if outpoint != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUncheckOutpointResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUncheckOutpointResponse{
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

func (s *FtServer) getAllDbFtGenesis(c *gin.Context) {
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
	data, err := s.indexer.GetAllDbFtGenesis(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisResponse{
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

func (s *FtServer) getAllDbFtGenesisOutput(c *gin.Context) {
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
	data, err := s.indexer.GetAllDbFtGenesisOutput(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if key != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisOutputResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisOutputResponse{
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

func (s *FtServer) getAllDbUsedFtIncome(c *gin.Context) {
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
	data, err := s.indexer.GetAllDbUsedFtIncome(txId)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// If key is provided, return result directly
	if txId != "" {
		c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUsedIncomeResponse{
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

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUsedIncomeResponse{
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

func (s *FtServer) getAllDbFtGenesisUtxo(c *gin.Context) {
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
	data, err := s.indexer.GetAllDbFtGenesisUtxo(key)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// Parse data
	parsedData := make(map[string]*respond.FtGenesisUtxo)
	for outpoint, value := range data {
		parts := strings.Split(value, "@")
		if len(parts) < 9 {
			continue
		}

		// Parse decimal
		decimal, _ := strconv.ParseUint(parts[3], 10, 8)

		// Check if there's an IsSpent flag
		isSpent := false
		if len(parts) > 9 {
			isSpent = parts[9] == "1"
		}

		parsedData[outpoint] = &respond.FtGenesisUtxo{
			SensibleId: parts[0],
			Name:       parts[1],
			Symbol:     parts[2],
			Decimal:    uint8(decimal),
			CodeHash:   parts[4],
			Genesis:    parts[5],
			Amount:     parts[6],
			Index:      parts[7],
			Value:      parts[8],
			IsSpent:    isSpent,
		}
	}

	// If key is provided, return result directly
	if key != "" {
		if utxo, exists := parsedData[key]; exists {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisUtxoResponse{
				Data: map[string]*respond.FtGenesisUtxo{key: utxo},
			}, time.Now().UnixMilli()-startTime))
		} else {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisUtxoResponse{
				Data: make(map[string]*respond.FtGenesisUtxo),
			}, time.Now().UnixMilli()-startTime))
		}
		return
	}

	// Calculate pagination
	total := len(parsedData)
	totalPages := (total + pageSize - 1) / pageSize

	// Get current page data
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}

	// Extract current page data
	currentPageData := make(map[string]*respond.FtGenesisUtxo)
	keys := make([]string, 0, len(parsedData))
	for k := range parsedData {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i := start; i < end && i < len(keys); i++ {
		currentPageData[keys[i]] = parsedData[keys[i]]
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisUtxoResponse{
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

func (s *FtServer) getUncheckFtOutpointTotal(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get total count
	total, err := s.indexer.GetUncheckFtOutpointTotal()
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUncheckOutpointTotalResponse{
		Total: total,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getUniqueFtUTXOs(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	utxos, err := s.indexer.GetUniqueFtUTXOs(codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUniqueUTXOsResponse{
		UTXOs: utxos,
		Count: len(utxos),
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtSummary(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get pagination parameters
	cursorInt, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	// Get FT summary data (cursor 为整型偏移)
	ftInfos, nextCursor, total, err := s.indexer.GetFtSummary(cursorInt, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSummaryResponse{
		FtInfos:    ftInfos,
		Count:      len(ftInfos),
		Cursor:     strconv.Itoa(cursorInt),
		NextCursor: nextCursor,
		Size:       size,
		Total:      total,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtGenesis(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get FT genesis information
	ftGenesisInfo, err := s.indexer.GetFtGenesis(codeHash, genesis)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisInfoResponse{
				CodeHash: codeHash,
				Genesis:  genesis,
			}, time.Now().UnixMilli()-startTime))
			return
		}
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisInfoResponse{
		CodeHash:      ftGenesisInfo.CodeHash,
		Genesis:       ftGenesisInfo.Genesis,
		SensibleId:    ftGenesisInfo.SensibleId,
		Name:          ftGenesisInfo.Name,
		Symbol:        ftGenesisInfo.Symbol,
		Decimal:       ftGenesisInfo.Decimal,
		Txid:          ftGenesisInfo.Txid,
		TxIndex:       ftGenesisInfo.TxIndex,
		ValueString:   ftGenesisInfo.ValueString,
		SatoshiString: ftGenesisInfo.SatoshiString,
		Height:        ftGenesisInfo.Height,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtSupply(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")

	if codeHash == "" || genesis == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash and genesis parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get FT supply information
	supplyInfo, err := s.indexer.GetFtSupply(codeHash, genesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSupplyResponse{
		Confirmed:           supplyInfo.Confirmed,
		Unconfirmed:         supplyInfo.Unconfirmed,
		AllowIncreaseIssues: supplyInfo.AllowIncreaseIssues,
		MaxSupply:           supplyInfo.MaxSupply,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtOwners(c *gin.Context) {
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

	// Get FT owners information
	ownerInfo, err := s.indexer.GetFtOwners(codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtOwnersResponse{
		List:       ownerInfo.List,
		Total:      ownerInfo.Total,
		Cursor:     ownerInfo.Cursor,
		NextCursor: ownerInfo.NextCursor,
		Size:       ownerInfo.Size,
	}, time.Now().UnixMilli()-startTime))
}

// getDbFtSupplyList gets the supply list (/db/ft/supply/list)
func (s *FtServer) getDbFtSupplyList(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	list, err := s.indexer.GetFtSupplyList(codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"list":       list.List,
		"total":      list.Total,
		"cursor":     list.Cursor,
		"nextCursor": list.NextCursor,
		"size":       list.Size,
	}, time.Now().UnixMilli()-startTime))
}

// getDbFtBurnList gets the burn list (/db/ft/burn/list)
func (s *FtServer) getDbFtBurnList(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	cursor, _ := strconv.Atoi(c.DefaultQuery("cursor", "0"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	if size < 1 {
		size = 10
	}

	list, err := s.indexer.GetFtBurnList(codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(gin.H{
		"list":       list.List,
		"total":      list.Total,
		"cursor":     list.Cursor,
		"nextCursor": list.NextCursor,
		"size":       list.Size,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtAddressHistory(c *gin.Context) {
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

	// Get FT address history information
	historyInfo, err := s.indexer.GetFtAddressHistory(address, codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAddressHistoryResponse{
		List:       historyInfo.List,
		Total:      historyInfo.Total,
		Cursor:     historyInfo.Cursor,
		NextCursor: historyInfo.NextCursor,
		Size:       historyInfo.Size,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getFtGenesisHistory(c *gin.Context) {
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

	// Get FT genesis history information
	historyInfo, err := s.indexer.GetFtGenesisHistory(codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtGenesisHistoryResponse{
		List:       historyInfo.List,
		Total:      historyInfo.Total,
		Cursor:     historyInfo.Cursor,
		NextCursor: historyInfo.NextCursor,
		Size:       historyInfo.Size,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getMempoolAddressFtSpendMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	address := c.Query("address")

	spendMap, err := s.indexer.GetMempoolAddressFtSpendMap(address)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtSpendMapResponse{
		Address:  address,
		SpendMap: spendMap,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getMempoolUniqueFtSpendMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHashGenesis := c.Query("codeHashGenesis")

	spendMap, err := s.indexer.GetMempoolUniqueFtSpendMap(codeHashGenesis)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtUniqueSpendMapResponse{
		CodeHashGenesis: codeHashGenesis,
		SpendMap:        spendMap,
	}, time.Now().UnixMilli()-startTime))
}

// getMempoolAddressFtIncomeMap gets FT income data for all addresses in mempool
func (s *FtServer) getMempoolAddressFtIncomeMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	incomeMap := s.mempoolMgr.GetMempoolAddressFtIncomeMap()

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAddressFtIncomeMapResponse{
		Address:   "",
		IncomeMap: incomeMap,
	}, time.Now().UnixMilli()-startTime))
}

// getMempoolAddressFtIncomeValidMap gets valid FT income data for all addresses in mempool
func (s *FtServer) getMempoolAddressFtIncomeValidMap(c *gin.Context) {
	startTime := time.Now().UnixMilli()

	// Get data
	incomeValidMap := s.mempoolMgr.GetMempoolAddressFtIncomeValidMap()

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAddressFtIncomeValidMapResponse{
		Address:        "",
		IncomeValidMap: incomeValidMap,
	}, time.Now().UnixMilli()-startTime))
}

// getFtOwnerTxData gets FT owner transaction data by codeHash, genesis and address
func (s *FtServer) getFtOwnerTxData(c *gin.Context) {
	startTime := time.Now().UnixMilli()
	codeHash := c.Query("codeHash")
	genesis := c.Query("genesis")
	address := c.Query("address")

	if codeHash == "" || genesis == "" || address == "" {
		c.JSONP(http.StatusBadRequest, respond.RespErr(errors.New("codeHash, genesis and address parameters are required"), time.Now().UnixMilli()-startTime, http.StatusBadRequest))
		return
	}

	// Get FT owner transaction data
	ownerTxData, err := s.indexer.GetFtOwnerTxData(codeHash, genesis, address)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtOwnerTxDataResponse{
		CodeHash: ownerTxData.CodeHash,
		Genesis:  ownerTxData.Genesis,
		Address:  ownerTxData.Address,
		Income:   ownerTxData.Income,
		Spend:    ownerTxData.Spend,
	}, time.Now().UnixMilli()-startTime))
}

func (s *FtServer) getDbAddressHistory(c *gin.Context) {
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

	// Get DB address history data
	historyList, err := s.indexer.GetDbAddressHistory(address, codeHash, genesis, cursor, size)
	if err != nil {
		c.JSONP(http.StatusInternalServerError, respond.RespErr(err, time.Now().UnixMilli()-startTime, http.StatusInternalServerError))
		return
	}

	// Convert to response format
	responseList := make([]*respond.FtAddressHistoryDbEntryResponse, 0, len(historyList.List))
	for _, entry := range historyList.List {
		responseList = append(responseList, &respond.FtAddressHistoryDbEntryResponse{
			TxId:   entry.TxId,
			Time:   entry.Time,
			TxType: entry.TxType,
		})
	}

	c.JSONP(http.StatusOK, respond.RespSuccess(respond.FtAddressHistoryDbListResponse{
		Total:      historyList.Total,
		List:       responseList,
		Cursor:     historyList.Cursor,
		NextCursor: historyList.NextCursor,
		Size:       historyList.Size,
	}, time.Now().UnixMilli()-startTime))
}
