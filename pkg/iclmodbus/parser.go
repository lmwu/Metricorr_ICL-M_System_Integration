package iclmodbus

import (
	"encoding/binary"
	"fmt"
	"math"
	"time"
)

// ChannelData 單一通道 CP/ER 數據解析結構
type ChannelData struct {
	Thickness float32 `json:"thickness"`   // 剩餘厚度 (µm)[cite: 1]
	Uac       float32 `json:"u_ac"`        // 結構交流電壓 (V)[cite: 1]
	Iac       float32 `json:"i_ac"`        // 試片交流電流 (mA)[cite: 1]
	Jac       float32 `json:"j_ac"`        // 交流電流密度 (A/m²)[cite: 1]
	Eon       float32 `json:"e_on"`        // 通電電位 (V)[cite: 1]
	Eoff      float32 `json:"e_off"`       // 斷電電位 (V)[cite: 1]
	Temp      float32 `json:"temperature"` // 探頭溫度 (°C)[cite: 1]
	MetalLoss float32 `json:"metal_loss"`  // 金屬損失率 (%)[cite: 1]
}

// FullMeasurementData 完整雙通道測量數據[cite: 1]
type FullMeasurementData struct {
	StationID string      `json:"station_id"`
	Timestamp string      `json:"timestamp"`
	Channel1  ChannelData `json:"channel_1"`
	Channel2  ChannelData `json:"channel_2"`
}

func BytesToFloat32(b []byte) float32 {
	if len(b) < 4 {
		return 0.0
	}
	bits := binary.BigEndian.Uint32(b[:4])
	return math.Float32frombits(bits)
}

// ParseDualChannelResponse 解析 Reg 1219 起始讀取 72 個 Registers 的 Modbus 響應[cite: 1]
func ParseDualChannelResponse(stationID string, resp []byte) (*FullMeasurementData, error) {
	if len(resp) < 3 || resp[1] != 0x03 {
		return nil, fmt.Errorf("無效的 Modbus 響應表頭")
	}

	byteCount := int(resp[2])
	if len(resp) < 3+byteCount+2 || byteCount < 144 {
		return nil, fmt.Errorf("Modbus 回傳長度不足以解析雙通道數據")
	}

	d := resp[3 : 3+byteCount]

	// 根據 Register Map 偏移量解碼 Channel 1 (Reg 1219 起) 與 Channel 2 (Reg 1263 起)[cite: 1]
	ch1 := ChannelData{
		Thickness: BytesToFloat32(d[0:4]),   // Reg 1219[cite: 1]
		Uac:       BytesToFloat32(d[4:8]),   // Reg 1221[cite: 1]
		Iac:       BytesToFloat32(d[8:12]),  // Reg 1223[cite: 1]
		Jac:       BytesToFloat32(d[12:16]), // Reg 1225[cite: 1]
		Eon:       BytesToFloat32(d[28:32]), // Reg 1233[cite: 1]
		Eoff:      BytesToFloat32(d[32:36]), // Reg 1235[cite: 1]
		Temp:      BytesToFloat32(d[40:44]), // Reg 1239[cite: 1]
		MetalLoss: BytesToFloat32(d[52:56]), // Reg 1245[cite: 1]
	}

	ch2 := ChannelData{
		Thickness: BytesToFloat32(d[88:92]),   // Reg 1263[cite: 1]
		Uac:       BytesToFloat32(d[92:96]),   // Reg 1265[cite: 1]
		Iac:       BytesToFloat32(d[96:100]),  // Reg 1267[cite: 1]
		Jac:       BytesToFloat32(d[100:104]), // Reg 1269[cite: 1]
		Eon:       BytesToFloat32(d[116:120]), // Reg 1277[cite: 1]
		Eoff:      BytesToFloat32(d[120:124]), // Reg 1279[cite: 1]
		Temp:      BytesToFloat32(d[128:132]), // Reg 1283[cite: 1]
		MetalLoss: BytesToFloat32(d[140:144]), // Reg 1289[cite: 1]
	}

	return &FullMeasurementData{
		StationID: stationID,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
		Channel1:  ch1,
		Channel2:  ch2,
	}, nil
}