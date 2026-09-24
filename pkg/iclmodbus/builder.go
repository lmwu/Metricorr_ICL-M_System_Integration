package iclmodbus

import "encoding/binary"

// BuildReadHoldingRegisters 建立 Modbus Function 0x03 封包
func BuildReadHoldingRegisters(slaveID byte, startReg uint16, count uint16) []byte {
	frame := make([]byte, 6, 8)
	frame[0] = slaveID
	frame[1] = 0x03
	binary.BigEndian.PutUint16(frame[2:4], startReg)
	binary.BigEndian.PutUint16(frame[4:6], count)

	crc := CalculateCRC16(frame)
	return append(frame, byte(crc&0xFF), byte(crc>>8))
}

// BuildWriteSingleRegister 建立 Modbus Function 0x06 封包
func BuildWriteSingleRegister(slaveID byte, reg uint16, value uint16) []byte {
	frame := make([]byte, 6, 8)
	frame[0] = slaveID
	frame[1] = 0x06
	binary.BigEndian.PutUint16(frame[2:4], reg)
	binary.BigEndian.PutUint16(frame[4:6], value)

	crc := CalculateCRC16(frame)
	return append(frame, byte(crc&0xFF), byte(crc>>8))
}