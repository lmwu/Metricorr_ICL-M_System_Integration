package iclmodbus

// CalculateCRC16 計算 Modbus RTU 的 CRC16 (Poly: 0xA001)
func CalculateCRC16(data []byte) uint16 {
	var crc uint16 = 0xFFFF
	for _, b := range data {
		crc ^= uint16(b)
		for i := 0; i < 8; i++ {
			if (crc & 0x0001) != 0 {
				crc = (crc >> 1) ^ 0xA001
			} else {
				crc >>= 1
			}
		}
	}
	return crc
}

// AppendCRC 為指令自動補上 Modbus RTU CRC16 (Low Byte 在前, High Byte 在後)
func AppendCRC(data []byte) []byte {
	crc := CalculateCRC16(data)
	return append(data, byte(crc&0xFF), byte(crc>>8))
}