package api

import (
	"embed"
	"html/template"
	"io"
	"mime"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/metaid/utxo_indexer/indexer"
	"github.com/metaid/utxo_indexer/syslogs"
)

//go:embed templates/* static/*
var templatesFS embed.FS

func (server *Server) setupWebRoutes() {
	// 注册自定义函数
	funcMap := template.FuncMap{
		"date":  formatDate,
		"add":   add,
		"sub":   sub,
		"short": short,
	}
	//server.Router.StaticFS("/static", http.FS(templatesFS))
	// 用自定义 Handler 替换 StaticFS，确保 MIME 类型正确
	server.Router.GET("/static/*filepath", func(c *gin.Context) {
		fp := c.Param("filepath")
		fp = strings.TrimPrefix(fp, "/")
		embedPath := "static/" + fp
		f, err := templatesFS.Open(embedPath)
		if err != nil {
			c.Status(404)
			return
		}
		defer f.Close()
		data, err := io.ReadAll(f)
		if err != nil {
			c.Status(500)
			return
		}
		ext := filepath.Ext(fp)
		mimeType := mime.TypeByExtension(ext)
		if mimeType == "" {
			mimeType = "application/octet-stream"
		}
		c.Data(200, mimeType, data)
	})
	//server.Router.LoadHTMLGlob("templates/**/*.html")
	//templates := template.Must(template.New("").Funcs(funcMap).ParseGlob("templates/**/*.html"))

	// 递归加载 templates 目录下所有 .html，并使用 funcMap
	tpls := template.New("").Funcs(funcMap)
	tpls, err := tpls.ParseFS(templatesFS, "templates/**/*.html")
	// err := filepath.WalkDir("templates", func(path string, d os.DirEntry, err error) error {
	// 	if err != nil {
	// 		return err
	// 	}
	// 	if d.IsDir() || !strings.HasSuffix(path, ".html") {
	// 		return nil
	// 	}
	// 	b, err := os.ReadFile(path)
	// 	if err != nil {
	// 		return err
	// 	}
	// 	// 模板名采用文件名（例如 "index.html"、"blocklog.html"、"header.html"）
	// 	name := filepath.Base(path)
	// 	if _, err := tpls.New(name).Parse(string(b)); err != nil {
	// 		return err
	// 	}
	// 	return nil
	// })
	if err != nil {
		panic("failed to load templates: " + err.Error())
	}
	server.Router.SetHTMLTemplate(tpls)

	//server.Router.SetHTMLTemplate(templates)
	server.Router.GET("/", dashIndex)
	server.Router.GET("/logs", dashBlockLogs)
	server.Router.GET("/reorg", dashReorg)
	server.Router.GET("/err", dashErr)
}
func dashIndex(c *gin.Context) {
	data := indexer.BaseCount
	c.HTML(200, "index.html", gin.H{
		"data": data,
	})
}
func dashBlockLogs(c *gin.Context) {
	// 获取分页参数
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	// 转换为整数
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}

	// 计算偏移量
	offset := (page - 1) * limit

	// 查询日志数据
	data, err := syslogs.QueryIndexerLogs(limit, offset)
	if err != nil {
		c.HTML(500, "blocklog.html", gin.H{"error": err.Error()})
		return
	}

	// 将分页数据传递给模板
	c.HTML(200, "blocklog.html", gin.H{
		"logs":        data,
		"CurrentPage": page,
		"Limit":       limit,
	})
}
func dashErr(c *gin.Context) {
	// 获取分页参数
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	// 转换为整数
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}

	// 计算偏移量
	offset := (page - 1) * limit

	// 查询日志数据
	data, err := syslogs.QueryErrLogs(limit, offset)
	if err != nil {
		c.HTML(500, "errlog.html", gin.H{"error": err.Error()})
		return
	}

	// 将分页数据传递给模板
	c.HTML(200, "errlog.html", gin.H{
		"logs":        data,
		"CurrentPage": page,
		"Limit":       limit,
	})
}

func dashReorg(c *gin.Context) {
	// 获取分页参数
	pageStr := c.DefaultQuery("page", "1")
	limitStr := c.DefaultQuery("limit", "20")

	// 转换为整数
	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 {
		limit = 20
	}

	// 计算偏移量
	offset := (page - 1) * limit

	// 查询日志数据
	data, err := syslogs.QueryReorgLogs(limit, offset)
	if err != nil {
		c.HTML(500, "reorg.html", gin.H{"error": err.Error()})
		return
	}

	// 将分页数据传递给模板
	c.HTML(200, "reorg.html", gin.H{
		"logs":        data,
		"CurrentPage": page,
		"Limit":       limit,
	})
}
