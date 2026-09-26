package main

import (
	"embed"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"
)

//go:embed web/*
var webFiles embed.FS

// SetupRouter 初始化並設定 Gin 路由與靜態資源
func SetupRouter(storage *Storage, mqttSvc *MQTTService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// 允許跨域請求 (CORS)
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// 靜態檔案服務 (使用內嵌檔案系統)
	subFS, err := fs.Sub(webFiles, "web")
	if err == nil {
		r.StaticFS("/web", http.FS(subFS))
		r.GET("/", func(c *gin.Context) {
			c.FileFromFS("index.html", http.FS(subFS))
		})
	}

	// RESTful API 路由配置
	api := r.Group("/api")
	{
		// 取得單一測站最新數據
		api.GET("/data/latest", func(c *gin.Context) {
			stationID := c.DefaultQuery("station", "STATION-001")
			data, found := storage.GetLatestData(stationID)
			if !found {
				c.JSON(http.StatusNotFound, gin.H{"error": "尚未取得測站數據"})
				return
			}
			c.JSON(http.StatusOK, data)
		})

		// 取得所有測站最新狀態
		api.GET("/data/all", func(c *gin.Context) {
			c.JSON(http.StatusOK, storage.GetAllLatestData())
		})

		// 取得歷史趨勢數據
		api.GET("/data/history", func(c *gin.Context) {
			stationID := c.DefaultQuery("station", "STATION-001")
			c.JSON(http.StatusOK, storage.GetHistory(stationID))
		})

		// 手動觸發量測工作流
		api.POST("/measure", func(c *gin.Context) {
			var req struct {
				StationID string `json:"station_id"`
			}
			if err := c.ShouldBindJSON(&req); err != nil || req.StationID == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的 station_id"})
				return
			}

			// 防重複執行鎖檢查（正確使用 storage 實例呼叫 TrySetMeasuring）
			if !storage.TrySetMeasuring(req.StationID, true) {
				c.JSON(http.StatusConflict, gin.H{"error": "該測站正處於量測程序中，請稍後再試"})
				return
			}

			// 異步啟動量測任務
			go func() {
				defer storage.TrySetMeasuring(req.StationID, false)
				ExecuteMeasurementWorkflow(req.StationID, mqttSvc, storage)
			}()

			c.JSON(http.StatusOK, gin.H{
				"status":     "accepted",
				"message":    "已成功啟動量測程序",
				"station_id": req.StationID,
			})
		})
	}

	return r
}