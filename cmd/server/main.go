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
	// 支援環境變數作為預設值，預設 Broker 本地端 Port 為 8888，HTTP API 服務 Port 為 8083，SQLite 資料庫檔案預設為 metricorr_ICL-M.db
	defaultBrokerPort := getEnv("MQTT_PORT", ":8888")
	defaultHttpPort := getEnv("HTTP_PORT", ":8083")
	defaultDB := getEnv("DB_PATH", "metricorr_ICL-M.db")

	brokerPort := flag.String("mqtt-port", defaultBrokerPort, "內嵌 MQTT Broker 監聽的 Port")
	httpPort := flag.String("port", defaultHttpPort, "HTTP API 服務 Port")
	sqlitePath := flag.String("db", defaultDB, "SQLite 資料庫檔案路徑")
	flag.Parse()

	clientBrokerURL := fmt.Sprintf("tcp://127.0.0.1%s", *brokerPort)
	log.Println("=========================================")

	log.Println("=========================================")
	log.Println("🚀 啟動 Cathodic Protection 監控服務中...")
	log.Println("=========================================")

	// ========================================================
	// [階段 1] 啟動內嵌 MQTT Broker (Mochi-MQTT)
	// ========================================================
	broker := mochi.New(nil)

	// 【測試階段】掛載 AllowHook，允許所有匿名連線 (不需帳號密碼)
	if err := broker.AddHook(new(auth.AllowHook), nil); err != nil {
		log.Fatalf("❌ 無法加入允許連線設定: %v", err)
	}

	// 【未來擴充】帳號密碼權限機制 (目前先註解，未來需啟用時解開即可)
	/*
		err := broker.AddHook(new(auth.Hook), &auth.Options{
			Ledger: &auth.Ledger{
				Auth: auth.AuthRules{
					{
						Username: "device_gateway",
						Password: "secret_password",
						Allow:    true,
					},
					{
						Username: "server_admin", // 這是後端 Client 連線要用的帳號
						Password: "admin_password",
						Allow:    true,
					},
				},
			},
		})
		if err != nil {
			log.Fatalf("❌ 無法加入帳密權限設定: %v", err)
		}
	*/

	// 設定 Broker 監聽本地 TCP Port
	tcpListener := listeners.NewTCP(listeners.Config{
		ID:      "t1",
		Address: *brokerPort,
	})
	if err := broker.AddListener(tcpListener); err != nil {
		log.Fatalf("❌ 無法建立 TCP 監聽: %v", err)
	}

	// 在背景啟動 Broker
	go func() {
		if err := broker.Serve(); err != nil {
			log.Fatalf("❌ Broker 運行失敗: %v", err)
		}
	}()

	time.Sleep(1 * time.Second) // 稍微等待確保 Broker 啟動完成
	log.Printf("[Broker] 🟢 內嵌 MQTT Broker 已成功啟動於 %s (允許無帳密連線)", *brokerPort)

	// ========================================================
	// [階段 2] 啟動您的 Web 伺服器與後端業務邏輯
	// ========================================================

	// 1. 初始化資料庫
	storageSvc := NewStorage(*sqlitePath)

	// 2. 初始化 MQTT 客戶端服務 (連線至剛剛啟動的本地 Broker)
	// 💡 注意：未來若啟用帳號密碼，請到 cmd/server/broker.go 內的 NewMQTTService，
	// 加入 opts.SetUsername("server_admin") 與 opts.SetPassword("admin_password")
	mqttSvc, err := NewMQTTService(clientBrokerURL)
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

	// 4. 動態定時任務：每小時自動探採一次所有測站
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()

		for range ticker.C {
			log.Println("[Server] ⏰ 執行例行定時量測任務...")
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

	log.Println("[Server] 🛑 收到關機訊號，啟動安全關機流程...")

	// 給予 5 秒時間處理未完成的 HTTP 請求
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("❌ HTTP Server 強制關機: %v", err)
	}

	// 關閉內嵌的 Broker
	broker.Close()

	log.Println("[Server] 🟢 服務已安全結束。")
}
