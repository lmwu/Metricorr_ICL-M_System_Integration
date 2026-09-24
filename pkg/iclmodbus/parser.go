package iclmodbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// MeasurementData 定義 ICL-M 數據結構
type MeasurementData struct {
	Timestamp  string  `json:"timestamp"`
	Thickness1 float32 `json:"thickness_1"`  // Reg 1219 (µm)
	Uac        float32 `json:"u_ac"`         // Reg 1221 (V)
	Iac        float32 `json:"i_ac"`         // Reg 1223 (mA)
	Jac        float32 `json:"j_ac"`         // Reg 1225 (A/m²)
	Eon        float32 `json:"e_on"`         // Reg 1233 (V)
	Eoff       float32 `json:"e_off"`        // Reg 1235 (V)
	Temp       float32 `json:"temperature"`  // Reg 1239 (°C)
	MetalLoss  float32 `json:"metal_loss_1"` // Reg 1245 (%)
}

// BytesToFloat32BigEndian 解析 IEEE-754 浮點數
func BytesToFloat32BigEndian(b []byte) float32 {
	if len(b) < 4 {
		return 0.0
	}
	bits := binary.BigEndian.Uint32(b[:4])
	return math.Float32frombits(bits)
}

// ParseMeasurementResponse 解析 Reg 1219 起始的 Modbus 響應 Payload[cite: 1]
func ParseMeasurementResponse(resp []byte) (*MeasurementData, error) {
	if len(resp) < 3 || resp[1] != 0x03 {
		return nil, fmt.Errorf("無效的 Modbus 響應格式")
	}

	byteCount := int(resp[2])
	if len(resp) < 3+byteCount+2 {
		return nil, fmt.Errorf("長度不足以解析暫存器數據")
	}

	dataBytes := resp[3 : 3+byteCount]

	// 依據 Metricorr Register Map 偏移量進行 IEEE-754 Float32 解析[cite: 1]
	return &MeasurementData{
		Timestamp:  time.Now().Format("2006-01-02 15:04:05"),
		Thickness1: BytesToFloat32BigEndian(dataBytes[0:4]),   // Reg 1219 Offset 0[cite: 1]
		Uac:        BytesToFloat32BigEndian(dataBytes[4:8]),   // Reg 1221 Offset 4[cite: 1]
		Iac:        BytesToFloat32BigEndian(dataBytes[8:12]),  // Reg 1223 Offset 8[cite: 1]
		Jac:        BytesToFloat32BigEndian(dataBytes[12:16]), // Reg 1225 Offset 12[cite: 1]
		Eon:        BytesToFloat32BigEndian(dataBytes[28:32]), // Reg 1233 Offset 28[cite: 1]
		Eoff:       BytesToFloat32BigEndian(dataBytes[32:36]), // Reg 1235 Offset 32[cite: 1]
		Temp:       BytesToFloat32BigEndian(dataBytes[40:44]), // Reg 1239 Offset 40[cite: 1]
		MetalLoss:  BytesToFloat32BigEndian(dataBytes[52:56]), // Reg 1245 Offset 52[cite: 1]
	}, nil
}