package main

import (
	"encoding/binary"
	"log"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

func ExecuteMeasurementWorkflow(stationID string, mqttClient *MQTTService, storage *Storage) {
	// 注意：這裡已移除 TrySetMeasuring 的重複上鎖，因外部呼叫前皆已上鎖並安排 defer 解鎖

	// ------------------------------------------------------------------
	// 階段一：發送探採啟動指令 (Reg 1199 = 1)
	// ------------------------------------------------------------------
	log.Printf("[%s] 🚀 1. 發送探採指令 (Reg 1199 = 1)...", stationID)
	cmdTrigger := iclmodbus.BuildTriggerMeasurementCmd(0x01)

	// 加入這行來印出 Hex String
	log.Printf("[%s] 實際下發的 Modbus Hex: %X", stationID, cmdTrigger)

	_, err := mqttClient.SendStationCommand(stationID, cmdTrigger, 3*time.Second)
	if err != nil {
		log.Printf("[%s] ❌ 發送探採指令失敗 (網關無回應或連線中斷): %v", stationID, err)
		return
	}

	// ------------------------------------------------------------------
	// 階段二：輪詢探採狀態 (Reg 1200)
	// ------------------------------------------------------------------
	log.Printf("[%s] ⏳ 2. 輪詢探採狀態 (Reg 1200)...", stationID)
	measurementSuccess := false
	startTime := time.Now()

	// 實體機探採週期約需 10~45 秒，設定 90 秒安全 Timeout
	for time.Since(startTime) < 90*time.Second {
		time.Sleep(3 * time.Second)

		cmdStatus := iclmodbus.BuildReadStatusCmd(0x01)
		respStatus, err := mqttClient.SendStationCommand(stationID, cmdStatus, 3*time.Second)
		if err != nil || len(respStatus) < 7 {
			continue
		}

		statusVal := binary.BigEndian.Uint16(respStatus[3:5])
		log.Printf("[%s] 📊 當前探採狀態碼: %d", stationID, statusVal)

		if statusVal == 3 { // 3 = Completed
			log.Printf("[%s] ✅ 探採完成！準備讀取數據...", stationID)
			measurementSuccess = true
			break
		} else if statusVal == 4 || statusVal == 5 { // 4 = Failed, 5 = HW Error
			log.Printf("[%s] ❌ 探採設備回報錯誤碼: %d", stationID, statusVal)
			return
		}
	}

	if !measurementSuccess {
		log.Printf("[%s] ⚠️ 探採超時，嘗試讀取最後可用數據...", stationID)
	}

	// ------------------------------------------------------------------
	// 階段三：讀取數據與儲存
	// ------------------------------------------------------------------
	// 讀取 Ch1
	log.Printf("[%s] 📡 3. 讀取 Ch1 暫存器...", stationID)
	cmdCh1 := iclmodbus.BuildReadCh1Cmd(0x01)
	respCh1, err1 := mqttClient.SendStationCommand(stationID, cmdCh1, 5*time.Second)
	var ch1Metrics *iclmodbus.ChannelMetrics
	if err1 == nil {
		ch1Metrics = iclmodbus.ParseChannelData(respCh1)
	} else {
		log.Printf("[%s] ❌ Ch1 讀取失敗: %v", stationID, err1)
	}

	// 讀取 Ch2
	log.Printf("[%s] 📡 4. 讀取 Ch2 暫存器...", stationID)
	cmdCh2 := iclmodbus.BuildReadCh2Cmd(0x01)
	respCh2, err2 := mqttClient.SendStationCommand(stationID, cmdCh2, 5*time.Second)
	var ch2Metrics *iclmodbus.ChannelMetrics
	if err2 == nil {
		ch2Metrics = iclmodbus.ParseChannelData(respCh2)
	} else {
		log.Printf("[%s] ⚠️ Ch2 讀取失敗: %v", stationID, err2)
	}

	// 寫入儲存層
	if ch1Metrics != nil || ch2Metrics != nil {
		storage.SaveMeasurementData(stationID, ch1Metrics, ch2Metrics)
		log.Printf("[%s] 🎉 流程完全結束，數據已成功入庫！", stationID)
	}
}
