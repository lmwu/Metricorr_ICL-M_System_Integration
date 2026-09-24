package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
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

	// 靜態檔案打包與服務 (Web Dashboard)
	webFS, _ := fs.Sub(webFiles, "web")
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	return mux
}

func (rt *Router) handleTrigger(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Reg 1199 = 1 (Start standard measurement)[cite: 1]
	cmd := iclmodbus.BuildWriteSingleRegister(0x01, 1199, 1)
	resp, err := rt.mqtt.SendCommandAndWait(cmd, 3*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("觸發測量失敗: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"message": "已成功下發測量指令",
		"raw_hex": fmt.Sprintf("%X", resp),
	})
}

func (rt *Router) handleGetData(w http.ResponseWriter, r *http.Request) {
	// Reg 1219 起讀取 28 個寄存器[cite: 1]
	cmd := iclmodbus.BuildReadHoldingRegisters(0x01, 1219, 28)
	resp, err := rt.mqtt.SendCommandAndWait(cmd, 3*time.Second)
	if err != nil {
		http.Error(w, fmt.Sprintf("讀取數據失敗: %v", err), http.StatusInternalServerError)
		return
	}

	data, err := iclmodbus.ParseMeasurementResponse(resp)
	if err != nil {
		http.Error(w, fmt.Sprintf("數據解析失敗: %v", err), http.StatusInternalServerError)
		return
	}

	rt.storage.SetLatestData(data)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
