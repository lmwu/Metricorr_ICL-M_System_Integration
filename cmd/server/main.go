package main

import (
	"encoding/json"
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

	// 2. 初始化儲存庫與 MQTT 服務
	storage := NewStorage()
	mqttSvc := NewMQTTService(storage)

	// 3. 假設陸域管線沿線有 3 個 CP 測站
	onshoreStations := []string{"STATION-001", "STATION-002", "STATION-003"}

	// 定時全管線平行採集 (例如每 10 分鐘一次)
	go startPipelineAutoScan(mqttSvc, storage, onshoreStations, 10*time.Minute)

	// 4. API 路由設定
	mux := http.NewServeMux()

	// API 1: 取得全管線所有測站最新 CP 數據
	mux.HandleFunc("/api/v1/stations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(storage.GetAllStationsData())
	})

	// API 2: 手動指定單一測站即時觸發採集 /api/v1/trigger?station_id=STATION-001
	mux.HandleFunc("/api/v1/trigger", func(w http.ResponseWriter, r *http.Request) {
		stationID := r.URL.Query().Get("station_id")
		if stationID == "" {
			http.Error(w, "缺少 station_id 參數", http.StatusBadRequest)
			return
		}

		// 下發 Modbus Function 0x03 讀取 72 個 Registers (單台網關的 Slave ID 固定為 1)
		cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 72)
		resp, err := mqttSvc.SendStationCommand(stationID, cmd, 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := iclmodbus.ParseDualChannelResponse(stationID, 0x01, resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		storage.UpdateStationData(stationID, data)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	log.Println("[Server] 陸域陰極防蝕多測站監控後台已啟動於 :8080")
	http.ListenAndServe(":8080", mux)
}

// 平行掃描全線所有獨立 CP 測站 (Goroutine 併發)
func startPipelineAutoScan(mqttSvc *MQTTService, storage *Storage, stations []string, interval time.Duration) {
	ticker := time.NewTicker(interval)
	for range ticker.C {
		log.Println("[全線巡檢] 開始採集所有陸域 CP 測站數據...")
		var wg sync.WaitGroup

		for _, stID := range stations {
			wg.Add(1)
			go func(stationID string) {
				defer wg.Done()

				cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 72)
				resp, err := mqttSvc.SendStationCommand(stationID, cmd, 5*time.Second)
				if err != nil {
					log.Printf("[掃描失敗] 測站 %s 無回應: %v", stationID, err)
					return
				}

				data, err := iclmodbus.ParseDualChannelResponse(stationID, 0x01, resp)
				if err == nil {
					storage.UpdateStationData(stationID, data)
					log.Printf("[掃描成功] 測站 %s | Ch1 Eoff: %.3f V", stationID, data.Channel1.Eoff)
				}
			}(stID)
		}

		wg.Wait()
		log.Println("[全線巡檢] 所有測站採集完畢。")
	}
}
