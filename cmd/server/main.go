package main

import (
	"log"
	"net/http"
	"sync"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

func main() {
	db := NewDatabase("metricorr.db")
	storage := NewStorage(db)

	broker := StartEmbeddedMQTTBroker(storage)
	defer broker.Close()
	time.Sleep(500 * time.Millisecond)

	mqttSvc := NewMQTTService(storage)

	go startUnattendedAutoScheduler(mqttSvc, storage)

	router := NewRouter(mqttSvc, storage, db)
	listenAddr := "127.0.0.1:8083"
	log.Printf("[Server] 陰極防蝕無人值守後台 UI 已啟動於: http://%s (請經由 Nginx 代理存取)", listenAddr)

	if err := http.ListenAndServe(listenAddr, router.SetupRoutes()); err != nil {
		log.Fatalf("[Server] HTTP 服務啟動失敗: %v", err)
	}
}

func startUnattendedAutoScheduler(mqttSvc *MQTTService, storage *Storage) {
	lastPollTime := time.Time{}

	for {
		time.Sleep(1 * time.Second) // 1 秒輪詢計數器，支援靈活回應 UI 頻率修改

		interval := storage.GetPollInterval()
		if interval <= 0 {
			continue
		}

		if time.Since(lastPollTime) < interval {
			continue
		}

		lastPollTime = time.Now()
		stations := storage.GetStationIDs()

		if len(stations) > 0 {
			log.Printf("[自動巡檢] 開始採集已註冊測站 (%d 個)...", len(stations))
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
	}
}

