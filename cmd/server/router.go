package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

//go:embed web
var webFiles embed.FS

type Router struct {
	mqtt    *MQTTService
	storage *Storage
}

func NewRouter(mqtt *MQTTService, storage *Storage) *Router {
	return &Router{mqtt: mqtt, storage: storage}
}

func (rt *Router) SetupRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v1/trigger", rt.handleTrigger)
	mux.HandleFunc("/api/v1/data", rt.handleGetData)
	mux.HandleFunc("/api/v1/stations", rt.handleGetAllStations)

	webFS, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("無法讀取嵌入的 web 目錄: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	return mux
}

func (rt *Router) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		stationID = "STATION-001" // 預設測站
	}

	// Reg 1199 = 1 (觸發測量)[cite: 1]
	cmd := iclmodbus.BuildWriteSingleRegister(0x01, 1199, 1)
	resp, err := rt.mqtt.SendStationCommand(stationID, cmd, 5*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("觸發測站 [%s] 測量失敗: %v", stationID, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"station_id": stationID,
		"message":    "已成功下發測量指令",
		"raw_hex":    fmt.Sprintf("%X", resp),
	})
}

func (rt *Router) handleGetData(w http.ResponseWriter, r *http.Request) {
	stationID := r.URL.Query().Get("station_id")
	if stationID == "" {
		stationID = "STATION-001"
	}

	// 讀取 Reg 1219 起 72 個 Registers (Channel 1 & Channel 2)[cite: 1]
	cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 72)
	resp, err := rt.mqtt.SendStationCommand(stationID, cmd, 5*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("讀取測站 [%s] 數據失敗: %v", stationID, err), http.StatusInternalServerError)
		return
	}

	data, err := iclmodbus.ParseDualChannelResponse(stationID, 0x01, resp)
	if err != nil {
		http.Error(w, fmt.Sprintf("數據解析失敗: %v", err), http.StatusInternalServerError)
		return
	}

	rt.storage.UpdateStationData(stationID, data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func (rt *Router) handleGetAllStations(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(rt.storage.GetAllStationsData())
}