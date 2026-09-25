package main

import (
	"embed"
	"encoding/json"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

//go:embed web/*
var webFiles embed.FS

type Router struct {
	mqtt    *MQTTService
	storage *Storage
	db      *Database
}

func NewRouter(mqtt *MQTTService, storage *Storage, db *Database) *Router {
	return &Router{mqtt: mqtt, storage: storage, db: db}
}

func (rt *Router) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	// API 1: 取得所有測站最新狀態
	mux.HandleFunc("/api/v1/stations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rt.storage.GetAllStationsData())
	})

	// API 2: 單點手動觸發採集 /api/v1/trigger?station_id=ST-KHH-01
	mux.HandleFunc("/api/v1/trigger", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodGet {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		stationID := r.URL.Query().Get("station_id")
		if stationID == "" {
			http.Error(w, "缺少 station_id 參數", http.StatusBadRequest)
			return
		}

		cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 72)
		resp, err := rt.mqtt.SendStationCommand(stationID, cmd, 5*time.Second)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		data, err := iclmodbus.ParseDualChannelResponse(stationID, resp)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		rt.storage.UpdateStationData(data)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	})

	// API 3: 設定無人值守自動輪詢頻率
	mux.HandleFunc("/api/v1/scheduler/interval", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		secondsStr := r.FormValue("seconds")
		sec, err := strconv.Atoi(secondsStr)
		if err != nil || sec < 0 {
			http.Error(w, "無效的 seconds 參數", http.StatusBadRequest)
			return
		}

		newDuration := time.Duration(sec) * time.Second
		rt.storage.SetPollInterval(newDuration)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":   "success",
			"interval": sec,
		})
	})

	// API 4: 取得 SQLite 歷史數據
	mux.HandleFunc("/api/v1/history", func(w http.ResponseWriter, r *http.Request) {
		stationID := r.URL.Query().Get("station_id")
		limitStr := r.URL.Query().Get("limit")
		limit := 30
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}

		history, err := rt.db.GetHistory(stationID, limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(history)
	})

	// API 5: 查詢連線網關動態狀態
	mux.HandleFunc("/api/v1/gateways", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(rt.storage.GetGatewaysStatus())
	})

	// 在路由註冊區域 (mux.HandleFunc) 加入以下接口：
	mux.HandleFunc("/api/v1/station/delete", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodDelete && req.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		stationID := req.URL.Query().Get("station_id")
		if stationID == "" {
			http.Error(w, "Missing station_id", http.StatusBadRequest)
			return
		}

		// 💡 修正點：使用 r.storage 調用 RemoveStation
		rt.storage.RemoveStation(stationID)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"success","message":"測站已成功廢止撤除"}`))
	})

	// 內嵌靜態 Web 頁面
	webFS, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	return mux
}
