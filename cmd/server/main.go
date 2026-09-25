package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

func main() {
	// 1. 啟動內嵌 MQTT Broker
	broker := StartEmbeddedMQTTBroker()
	defer broker.Close()
	time.Sleep(500 * time.Millisecond)

	// 2. 初始化 SQLite 資料庫與 Storage
	db := NewDatabase("metricorr.db")
	storage := NewStorage(db)
	mqttSvc := NewMQTTService(storage)

	// 3. 啟動「無人值守」全自動巡檢輪詢服務
	go startUnattendedAutoScheduler(mqttSvc, storage)

	// 4. 啟動 REST API 與 Web UI (僅限本地 127.0.0.1:8083 供 Nginx 代理)
	router := NewRouter(mqttSvc, storage, db)
	listenAddr := "127.0.0.1:8083"
	log.Printf("[Server] 陰極防蝕無人值守後台 UI 已啟動於: http://%s (請經由 Nginx 代理存取)", listenAddr)
	
	if err := http.ListenAndServe(listenAddr, router.SetupRoutes()); err != nil {
		log.Fatalf("[Server] HTTP 服務啟動失敗: %v", err)
	}
}

// 動態響應使用者設定的自動巡檢 Worker
func startUnattendedAutoScheduler(mqttSvc *MQTTService, storage *Storage) {
	for {
		interval := storage.GetPollInterval()
		if interval <= 0 {
			// 當前使用者設為停用採集，每 3 秒檢查一次狀態
			time.Sleep(3 * time.Second)
			continue
		}

		log.Printf("[自動巡檢] 開始針對所有已註冊測站執行採集 (下次採集將於 %v 後)...", interval)

		stations := storage.GetStationIDs()
		if len(stations) > 0 {
			var wg sync.WaitGroup
			for _, stID := range stations {
				wg.Add(1)
				go func(stationID string) {
					defer wg.Done()
					cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 72)
					resp, err := mqttSvc.SendStationCommand(stationID, cmd, 5*time.Second)
					if err != nil {
						log.Printf("[自動採集失敗] 測站 %s: %v", stationID, err)
						return
					}
					data, err := iclmodbus.ParseDualChannelResponse(stationID, resp)
					if err == nil {
						storage.UpdateStationData(data)
						log.Printf("[自動採集成功] 測站 %s | Ch1 Eoff: %.3f V | MetalLoss: %.2f%%", stationID, data.Channel1.Eoff, data.Channel1.MetalLoss)
					}
				}(stID)
			}
			wg.Wait()
		}

		time.Sleep(interval)
	}
}