package iclmodbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// ProbeChannelData 單一探頭通道數據
type ProbeChannelData struct {
	Thickness float32 `json:"thickness"`   // 剩餘厚度 (µm)
	Uac       float32 `json:"u_ac"`        // 結構交流電壓 (V)
	Iac       float32 `json:"i_ac"`        // 試片交流電流 (mA)
	Jac       float32 `json:"j_ac"`        // 交流電流密度 (A/m²)
	Eon       float32 `json:"e_on"`        // 通電電位 (V)
	Eoff      float32 `json:"e_off"`       // 斷電電位 (V)
	Temp      float32 `json:"temperature"` // 探頭溫度 (°C)
	MetalLoss float32 `json:"metal_loss"`  // 金屬損失率 (%)
}

// MeasurementData 代表單台 ICL-M 的完整測量回覆
type MeasurementData struct {
	DeviceID  string            `json:"device_id"`
	SlaveID   byte              `json:"slave_id"`
	Timestamp string            `json:"timestamp"`
	Channel1  *ProbeChannelData `json:"channel_1"` // 探頭 1
	Channel2  *ProbeChannelData `json:"channel_2"` // 探頭 2
}

func BytesToFloat32BigEndian(b []byte) float32 {
	if len(b) < 4 {
		return 0.0
	}
	bits := binary.BigEndian.Uint32(b[:4])
	return math.Float32frombits(bits)
}

func parseProbeBlock(b []byte) *ProbeChannelData {
	if len(b) < 56 {
		return nil
	}
	return &ProbeChannelData{
		Thickness: BytesToFloat32BigEndian(b[0:4]),   // Reg 1219 / 1263
		Uac:       BytesToFloat32BigEndian(b[4:8]),   // Reg 1221 / 1265
		Iac:       BytesToFloat32BigEndian(b[8:12]),  // Reg 1223 / 1267
		Jac:       BytesToFloat32BigEndian(b[12:16]), // Reg 1225 / 1269
		Eon:       BytesToFloat32BigEndian(b[28:32]), // Reg 1233 / 1277
		Eoff:      BytesToFloat32BigEndian(b[32:36]), // Reg 1235 / 1279
		Temp:      BytesToFloat32BigEndian(b[40:44]), // Reg 1239 / 1283
		MetalLoss: BytesToFloat32BigEndian(b[52:56]), // Reg 1245 / 1289
	}
}

// ParseDualChannelResponse 解析 Reg 1219~1290 (長度 72 Registers = 144 Bytes)
func ParseDualChannelResponse(deviceID string, slaveID byte, resp []byte) (*MeasurementData, error) {
	if len(resp) < 3 || resp[1] != 0x03 {
		return nil, fmt.Errorf("無效的 Modbus 響應格式")
	}

	byteCount := int(resp[2])
	if byteCount < 144 || len(resp) < 3+byteCount+2 {
		return nil, fmt.Errorf("長度不足以解析雙通道數據 (需要 144 Bytes，實際 %d Bytes)", byteCount)
	}

	dataBytes := resp[3 : 3+byteCount]

	// Channel 1: Reg 1219 起始 (Offset 0)
	ch1Data := parseProbeBlock(dataBytes[0:56])

	// Channel 2: Reg 1263 起始 (Offset 88 Bytes)
	ch2Data := parseProbeBlock(dataBytes[88:144])

	return &MeasurementData{
		DeviceID:  deviceID,
		SlaveID:   slaveID,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Channel1:  ch1Data,
		Channel2:  ch2Data,
	}, nil
}