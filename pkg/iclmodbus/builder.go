package iclmodbus

import (
	"encoding/binary"
	"math"
)

// 1. 觸發探採指令 (Reg 1199 = 1)
func BuildTriggerMeasurementCmd(slaveID byte) []byte {
	return AppendCRC([]byte{slaveID, 0x06, 0x04, 0xAF, 0x00, 0x01})
}

// 2. 讀取探採狀態指令 (Reg 1200, Len 1)
func BuildReadStatusCmd(slaveID byte) []byte {
	return AppendCRC([]byte{slaveID, 0x03, 0x04, 0xB0, 0x00, 0x01})
}

// 3. 讀取 Ch1 數據指令 (Reg 1219, Len 28)
func BuildReadCh1Cmd(slaveID byte) []byte {
	return AppendCRC([]byte{slaveID, 0x03, 0x04, 0xC3, 0x00, 0x1C})
}

// 4. 讀取 Ch2 數據指令 (Reg 1263, Len 28)
func BuildReadCh2Cmd(slaveID byte) []byte {
	return AppendCRC([]byte{slaveID, 0x03, 0x04, 0xEF, 0x00, 0x1C})
}

type ChannelMetrics struct {
	Thickness   float32 `json:"thickness"`
	Uac         float32 `json:"uac"`
	Iac         float32 `json:"iac"`
	Jac         float32 `json:"j_ac"`
	Rs          float32 `json:"rs"`
	Idc         float32 `json:"idc"`
	Jdc         float32 `json:"j_dc"`
	Eon         float32 `json:"e_on"`
	Eoff        float32 `json:"e_off"`
	EirFree     float32 `json:"e_ir_free"`
	Temperature float32 `json:"temperature"`
	Rr          float32 `json:"rr"`
	Rc          float32 `json:"rc"`
	MetalLoss   float32 `json:"metal_loss"`
}

// 解析 Modbus 功能碼 03 回應 (長度需 >= 59 Bytes)
func ParseChannelData(payload []byte) *ChannelMetrics {
// 完整封包: Header(3) + Data(56) + CRC(2) = 61 Bytes
	if len(payload) < 61 || payload[1] >= 0x80 || payload[2] != 56 {
		return nil
	}
	// 校驗 Modbus CRC16
	if CalculateCRC16(payload[:61]) != 0 {
		return nil
	}

	data := payload[3 : 3+56]

	getFloat := func(offset int) float32 {
		bits := binary.BigEndian.Uint32(data[offset : offset+4])
		return math.Float32frombits(bits)
	}

	return &ChannelMetrics{
		Thickness:   getFloat(0),
		Uac:         getFloat(4),
		Iac:         getFloat(8),
		Jac:         getFloat(12),
		Rs:          getFloat(16),
		Idc:         getFloat(20),
		Jdc:         getFloat(24),
		Eon:         getFloat(28),
		Eoff:        getFloat(32),
		EirFree:     getFloat(36),
		Temperature: getFloat(40),
		Rr:          getFloat(44),
		Rc:          getFloat(48),
		MetalLoss:   getFloat(52),
	}
}