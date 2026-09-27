package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	defaultBrokerPort := getEnv("MQTT_PORT", ":8888")
	defaultHttpPort := getEnv("HTTP_PORT", ":8083")
	defaultDB := getEnv("DB_PATH", "metricorr_ICL-M.db")

	brokerPort := flag.String("mqtt-port", defaultBrokerPort, "內嵌 MQTT Broker 監聽 Port")
	httpPort := flag.String("port", defaultHttpPort, "HTTP API 服務 Port")
	sqlitePath := flag.String("db", defaultDB, "SQLite 資料庫檔案路徑")
	flag.Parse()

	clientBrokerURL := fmt.Sprintf("tcp://127.0.0.1%s", *brokerPort)
	log.Println("=========================================")
	log.Println("🚀 啟動 Cathodic Protection 陰極防蝕監控服務中...")
	log.Println("=========================================")

	// 1. 初始化資料庫與存儲層
	storageSvc := NewStorage(*sqlitePath)

	// ========================================================
	// [階段 1] 啟動模組化內嵌 MQTT Broker (Mochi-MQTT)
	// ========================================================
	broker := mochi.New(nil)

	// 載入基本認證 Hook (可依需求改為自訂認證)
	if err := broker.AddHook(new(auth.AllowHook), nil); err != nil {
		log.Fatalf("❌ 無法載入權限管理 Hook: %v", err)
	}

	// 🌟 載入模組化 RMU 連線監聽 Hook (注入自訂或預設的 GatewayIdentifier)
	rmuHook := NewClientLoggerHook(storageSvc, DefaultGatewayIdentifier)
	if err := broker.AddHook(rmuHook, nil); err != nil {
		log.Fatalf("❌ 掛載 RMU 連線監聽 Hook 失敗: %v", err)
	}

	tcpListener := listeners.NewTCP(listeners.Config{
		ID:      "rmu-tcp-listener",
		Address: *brokerPort,
	})
	if err := broker.AddListener(tcpListener); err != nil {
		log.Fatalf("❌ 無法建立 TCP 監聽器: %v", err)
	}

	go func() {
		if err := broker.Serve(); err != nil {
			log.Fatalf("❌ Broker 運行異常: %v", err)
		}
	}()

	time.Sleep(1 * time.Second)
	log.Printf("[Broker] 🟢 內嵌 MQTT Broker 已啟動於 %s", *brokerPort)

	// ========================================================
	// [階段 2] 啟動 Core MQTT 通訊服務與 Web API
	// ========================================================

	mqttSvc, err := NewMQTTService(clientBrokerURL, storageSvc)
	if err != nil {
		log.Fatalf("❌ MQTT Core 服務初始化失敗: %v", err)
	}
	defer mqttSvc.Close()

	// 設定 Web API 路由
	router := SetupRouter(storageSvc, mqttSvc)

	srv := &http.Server{
		Addr:    *httpPort,
		Handler: router,
	}

	go func() {
		log.Printf("[Server] 🌐 API 伺服器已就緒: http://127.0.0.1%s", *httpPort)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ HTTP 伺服器異常終止: %v", err)
		}
	}()

	// 4. 定時工作流 (預設 1 小時輪詢全網關) !!!!!! 非常重要內定設定
	go func() {
		ticker := time.NewTicker(1 * time.Minute) // 每 1 分鐘觸發一次 (可依需求調整)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("[Server] ⏰ 觸發例行全區測站探採任務...")
			allStations := storageSvc.GetAllLatestData()
			for stationID := range allStations {
				if storageSvc.TrySetMeasuring(stationID, true) {
					go func(stID string) {
						defer storageSvc.TrySetMeasuring(stID, false)
						ExecuteMeasurementWorkflow(stID, mqttSvc, storageSvc)
					}(stationID)
				}
			}
		}
	}()

	// ========================================================
	// [階段 3] 優雅關機處理 (Graceful Shutdown)
	// ========================================================
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Server] 🛑 收到關機指令，安全清理資源中...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP Server 強制關機: %v", err)
	}

	broker.Close()
	log.Println("[Server] 🟢 所有服務已完全停止。")
}