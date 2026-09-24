package iclmodbus

// CalculateCRC16 計算 Modbus RTU CRC16 校驗碼
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

// VerifyCRC16 驗證 Modbus RTU 封包的 CRC16
func VerifyCRC16(data []byte) bool {
	if len(data) < 4 {
		return false
	}
	payloadLen := len(data) - 2
	expectedCRC := CalculateCRC16(data[:payloadLen])
	actualCRC := uint16(data[payloadLen]) | (uint16(data[payloadLen+1]) << 8)
	return expectedCRC == actualCRC
}