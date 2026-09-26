package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// 支援環境變數作為預設值，提升 Docker / Cloud 部署彈性
	defaultBroker := getEnv("MQTT_BROKER", "tcp://127.0.0.1:8888")
	defaultPort := getEnv("HTTP_PORT", ":8083")
	defaultDB := getEnv("DB_PATH", "metricorr_ICL-M.db")

	brokerURL := flag.String("broker", defaultBroker, "MQTT Broker 位址")
	httpPort := flag.String("port", defaultPort, "HTTP API 服務 Port")
	sqlitePath := flag.String("db", defaultDB, "SQLite 資料庫檔案路徑")
	flag.Parse()

	log.Println("=========================================")
	log.Println("🚀 啟動 Cathodic Protection 監控服務中...")
	log.Println("=========================================")

	// 1. 初始化資料庫
	storageSvc := NewStorage(*sqlitePath)

	// 2. 初始化 MQTT 服務
	mqttSvc, err := NewMQTTService(*brokerURL)
	if err != nil {
		log.Fatalf("❌ MQTT 服務初始化失敗: %v", err)
	}
	defer mqttSvc.Close()

	// 3. 設定 HTTP 路由與 Web 服務
	router := SetupRouter(storageSvc, mqttSvc)

	srv := &http.Server{
		Addr:    *httpPort,
		Handler: router,
	}

	// 在 Goroutine 中啟動 HTTP 伺服器
	go func() {
		log.Printf("[Server] 🌐 API 伺服器已就緒: http://127.0.0.1%s", *httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP 伺服器異常終止: %v", err)
		}
	}()

	// 4. 動態定時任務：探採資料庫內所有測站
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("[Server] ⏰ 執行例行定時量測任務...")
			allStations := storageSvc.GetAllLatestData()
			for stationID := range allStations {
				// 透過 storageSvc 實例呼叫 TrySetMeasuring 方法
				if storageSvc.TrySetMeasuring(stationID, true) {
					go func(stID string) {
						defer storageSvc.TrySetMeasuring(stID, false)
						ExecuteMeasurementWorkflow(stID, mqttSvc, storageSvc)
					}(stationID)
				}
			}
		}
	}()

	// 5. 監聽關機訊號實作 Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] 🛑 收到關機訊號，啟動安全關機流程...")

	// 給予 5 秒時間處理未完成的 HTTP 請求
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP Server 強制關機: %v", err)
	}

	log.Println("[Server] 🟢 服務已安全結束。")
}