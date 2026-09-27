package main

import (
	"embed"
	
	"io/fs"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

//go:embed web/*
var webFiles embed.FS

func SetupRouter(storage *Storage, mqttSvc *MQTTService) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

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

	subFS, err := fs.Sub(webFiles, "web")
	if err == nil {
		r.StaticFS("/web", http.FS(subFS))

		r.GET("/", func(c *gin.Context) {
			content, err := fs.ReadFile(subFS, "index.html")
			if err != nil {
				c.String(http.StatusNotFound, "Index file not found")
				return
			}
			c.Data(http.StatusOK, "text/html; charset=utf-8", content)
		})

		r.GET("/app.js", func(c *gin.Context) {
			content, err := fs.ReadFile(subFS, "app.js")
			if err != nil {
				c.String(http.StatusNotFound, "app.js not found")
				return
			}
			c.Data(http.StatusOK, "application/javascript; charset=utf-8", content)
		})
	}

	api := r.Group("/api")
	{
		stations := api.Group("/stations")
		{
			// SSE 即時廣播通道：當網關上線/狀態改變時立即推播
			stations.GET("/events", func(c *gin.Context) {
				c.Writer.Header().Set("Content-Type", "text/event-stream")
				c.Writer.Header().Set("Cache-Control", "no-cache")
				c.Writer.Header().Set("Connection", "keep-alive")
				c.Writer.Header().Set("Transfer-Encoding", "chunked")

				ticker := time.NewTicker(2 * time.Second)
				defer ticker.Stop()

				notifyChan := c.Request.Context().Done()
				for {
					select {
					case <-notifyChan:
						return
					case <-ticker.C:
						// 定期廣播目前線上測站清單
						list := storage.GetOnlineStations()
						if list == nil {
							list = []string{}
						}
						c.SSEvent("online_update", list)
						c.Writer.Flush()
					}
				}
			})

			stations.GET("/online", func(c *gin.Context) {
				list := storage.GetOnlineStations()
				if list == nil {
					list = []string{}
				}
				c.JSON(http.StatusOK, list)
			})

			stations.GET("/data/all", func(c *gin.Context) {
				c.JSON(http.StatusOK, storage.GetAllLatestData())
			})

			station := stations.Group("/:id")
			{
				station.GET("/data/latest", func(c *gin.Context) {
					stationID := c.Param("id")
					data, found := storage.GetLatestData(stationID)
					if !found {
						c.JSON(http.StatusNotFound, gin.H{"error": "尚未取得測站數據"})
						return
					}
					c.JSON(http.StatusOK, data)
				})

				station.GET("/data/history", func(c *gin.Context) {
					stationID := c.Param("id")
					c.JSON(http.StatusOK, storage.GetHistory(stationID))
				})

				station.POST("/measure", func(c *gin.Context) {
					stationID := c.Param("id")
					if stationID == "" {
						c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的 station_id"})
						return
					}

					if !storage.TrySetMeasuring(stationID, true) {
						c.JSON(http.StatusConflict, gin.H{"error": "該測站正處於量測程序中，請稍後再試"})
						return
					}

					go func() {
						defer storage.TrySetMeasuring(stationID, false)
						ExecuteMeasurementWorkflow(stationID, mqttSvc, storage)
					}()

					c.JSON(http.StatusOK, gin.H{
						"status":     "accepted",
						"message":    "已成功啟動量測程序",
						"station_id": stationID,
					})
				})
			}
		}
	}

	return r
}