package main

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

// Modbus RTU CRC16 計算邏輯 (Polynomial: 0xA001)
func calculateCRC16(data []byte) uint16 {
	crc := uint16(0xFFFF)
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

// 驗證二進位數據的 CRC16 (低位元組在前，高位元組在後)
func verifyCRC16(data []byte) bool {
	if len(data) < 3 {
		return false
	}
	payload := data[:len(data)-2]
	expectedCRC := calculateCRC16(payload)

	receivedLow := uint16(data[len(data)-2])
	receivedHigh := uint16(data[len(data)-1])
	receivedCRC := receivedLow | (receivedHigh << 8)

	return expectedCRC == receivedCRC
}

func main() {
	portName := "COM3"

	// 串口通訊參數 (預設 115200 8N1，若設備為 19200 請修改 BaudRate)
	mode := &serial.Mode{
		BaudRate: 115200,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}

	fmt.Printf("🔌 正在開啟 %s (115200 8N1)...\n", portName)
	port, err := serial.Open(portName, mode)
	if err != nil {
		log.Fatalf("❌ 無法開啟 %s: %v\n請確認該 COM Port 未被其他軟體開啟！", portName, err)
	}
	defer port.Close()

	if err := port.SetReadTimeout(3 * time.Second); err != nil {
		log.Fatalf("❌ 設定 ReadTimeout 失敗: %v", err)
	}

	// 1. 下發接回指令: 01 03 04 C3 00 1C B5 0F
	// 2. 下發探採指令: 01 06 04 AF 00 01 79 1B 
	//reqHex := "010304C3001CB50F"
	reqHex := "010604AF0001791B" // 探採指令
	reqBytes, err := hex.DecodeString(reqHex)
	if err != nil {
		log.Fatalf("❌ Hex 字串轉換失敗: %v", err)
	}

	// 下發前的 CRC16 校驗
	if verifyCRC16(reqBytes) {
		fmt.Printf("📤 發送指令 Hex: %X (CRC16 驗證通過)\n", reqBytes)
	} else {
		fmt.Printf("⚠️ 警告：要發送的指令 CRC16 校驗不匹配！Hex: %X\n", reqBytes)
	}

	n, err := port.Write(reqBytes)
	if err != nil {
		log.Fatalf("❌ 寫入串口失敗: %v", err)
	}
	fmt.Printf("✅ 已送出 %d 位元組，等待 ICL-M 設備回應...\n\n", n)

	// 2. 接收回應 (預期長度: 3 Header + 56 Data + 2 CRC = 61 位元組)
	buf := make([]byte, 128)
	received := make([]byte, 0)
	startTime := time.Now()

	for time.Since(startTime) < 3*time.Second {
		readNum, err := port.Read(buf)
		if err != nil {
			fmt.Printf("⚠️ 讀取時發生錯誤: %v\n", err)
			break
		}
		if readNum > 0 {
			received = append(received, buf[:readNum]...)
			if len(received) >= 61 {
				break
			}
		}
	}

	// 3. 檢查回應與進行 CRC16 校驗
	if len(received) == 0 {
		fmt.Println("❌ 超時：未收到 COM3 的任何資料。")
		return
	}

	fmt.Printf("📥 成功接收 [%d Bytes] 回應 Hex:\n%X\n\n", len(received), received)

	fmt.Println("🔍 數據拆解與 CRC16 驗證：")
	if len(received) >= 3 {
		fmt.Printf(" - Slave ID   : 0x%02X\n", received[0])
		fmt.Printf(" - Function   : 0x%02X\n", received[1])
		fmt.Printf(" - Byte Count : %d 位元組\n", received[2])
	}

	if len(received) >= 3 {
		recvCRC := uint16(received[len(received)-2]) | (uint16(received[len(received)-1]) << 8)
		calcCRC := calculateCRC16(received[:len(received)-2])

		fmt.Printf(" - 封包末端 CRC: 0x%04X (Low: 0x%02X, High: 0x%02X)\n", recvCRC, received[len(received)-2], received[len(received)-1])
		fmt.Printf(" - 本地計算 CRC: 0x%04X\n", calcCRC)

		if verifyCRC16(received) {
			fmt.Println("🟢 CRC16 校驗結果: 正確 (PASS)")
		} else {
			fmt.Println("🔴 CRC16 校驗結果: 錯誤 (FAIL) - 數據可能傳輸中受干擾受損！")
		}
	}
}