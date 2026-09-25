package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

func main() {
	stationFlag := flag.String("station", "STATION-001", "測站 ID (可帶入多個以逗號分隔，例如 STATION-001,STATION-002)")
	brokerURL := flag.String("broker", "tcp://localhost:8888", "MQTT Broker 位址")
	portName := flag.String("port", "", "RS485 COM Port (例如 COM3 或 /dev/ttyUSB0)。若不填則自動切換為【純軟體 Mock 模式】")
	baudRate := flag.Int("baud", 19200, "RS485 波特率 (Baud Rate)")
	flag.Parse()

	stations := strings.Split(*stationFlag, ",")
	log.Printf("[sim-gateway] 網關服務啟動中... 測站列表: %v", stations)

	for _, stationID := range stations {
		stationID = strings.TrimSpace(stationID)
		if stationID == "" {
			continue
		}
		go runGatewayWorker(stationID, *brokerURL, *portName, *baudRate)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("[sim-gateway] 模擬網關服務已停止")
}

func runGatewayWorker(stationID, brokerURL, portName string, baudRate int) {
	clientID := fmt.Sprintf("GATEWAY-%s", stationID)
	topicRx := fmt.Sprintf("cp-gateway/%s/rx", stationID)
	topicTx := fmt.Sprintf("cp-gateway/%s/tx", stationID)

	var serialPort serial.Port
	var err error

	if portName != "" {
		mode := &serial.Mode{
			BaudRate: baudRate,
			DataBits: 8,
			Parity:   serial.NoParity,
			StopBits: serial.OneStopBit,
		}
		serialPort, err = serial.Open(portName, mode)
		if err != nil {
			log.Fatalf("[%s] ❌ 無法開啟 COM Port (%s): %v", stationID, portName, err)
		}
		defer serialPort.Close()
		log.Printf("[%s] 🔌 已成功開啟實體 RS485 序列埠 %s (%d bps)", stationID, portName, baudRate)
	} else {
		log.Printf("[%s] 💡 未指定 -port 參數，啟動為【純軟體 Mock 模擬模式】", stationID)
	}

	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(clientID)
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[%s] 🟢 已連接至 MQTT Broker", clientID)

		c.Subscribe(topicRx, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()
			log.Printf("[%s] 📩 [MQTT -> RS485] 收到下發指令 (Hex): %X", stationID, payload)

			if serialPort != nil {
				// 模式 A：寫入 RS485 實體串口並讀取完整 Modbus 封包
				_, writeErr := serialPort.Write(payload)
				if writeErr != nil {
					log.Printf("[%s] ❌ RS485 寫入失敗: %v", stationID, writeErr)
					return
				}

				// 循環讀取以應對 UART 分片到達
				respData := readFullModbusResponse(serialPort, 2*time.Second)
				if len(respData) == 0 {
					log.Printf("[%s] ⚠️ RS485 讀取超時或無數據回傳", stationID)
					return
				}

				log.Printf("[%s] 📤 [RS485 -> MQTT] 收到實機響應 (%d bytes)", stationID, len(respData))
				client.Publish(topicTx, 0, false, respData)
			} else {
				// 模式 B：符合 149 位元組規格的完整 Mock 響應封包
				slaveID := payload[0]
				mockResp := generateMockICLResponse(slaveID)
				client.Publish(topicTx, 0, false, mockResp)
			}
		})
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Printf("[%s] 🔴 MQTT 連線失敗: %v", clientID, token.Error())
	}
}

// 讀取完整 Modbus 響應封包（防 UART 分片）
func readFullModbusResponse(port serial.Port, timeout time.Duration) []byte {
	buf := make([]byte, 256)
	var fullResp []byte
	startTime := time.Now()

	for time.Since(startTime) < timeout {
		n, err := port.Read(buf)
		if err == nil && n > 0 {
			fullResp = append(fullResp, buf[:n]...)
			if len(fullResp) >= 3 {
				expectedLen := 3 + int(fullResp[2]) + 2
				if len(fullResp) >= expectedLen {
					return fullResp[:expectedLen]
				}
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fullResp
}

// 產生符合 ICL-M 72 Registers (144 位元組 Data) 的合法 Modbus 封包
func generateMockICLResponse(slaveID byte) []byte {
	buf := new(bytes.Buffer)
	buf.WriteByte(slaveID)
	buf.WriteByte(0x03)
	buf.WriteByte(144) // 144 bytes data

	dataRaw := make([]byte, 144)

	putFloat := func(offset int, val float32) {
		bits := math.Float32bits(val)
		binary.BigEndian.PutUint32(dataRaw[offset:offset+4], bits)
	}

	// Channel 1 Mock Data
	putFloat(0, 185.5)   // Reg 1219: Thickness_1 (µm)
	putFloat(4, 2.1)     // Reg 1221: Uac_1 (V)
	putFloat(8, 15.3)    // Reg 1223: Iac_1 (mA)
	putFloat(12, 12.8)   // Reg 1225: Jac_1 (A/m²)
	putFloat(28, -1.150) // Reg 1233: Eon_1 (V)
	putFloat(32, -0.920) // Reg 1235: Eoff_1 (V)
	putFloat(40, 26.5)   // Reg 1239: Temp_1 (°C)
	putFloat(52, 7.25)   // Reg 1245: MetalLoss_1 (%)

	// Channel 2 Mock Data
	putFloat(88, 200.0)  // Reg 1263: Thickness_2 (µm)
	putFloat(92, 1.8)    // Reg 1265: Uac_2 (V)
	putFloat(96, 8.4)    // Reg 1267: Iac_2 (mA)
	putFloat(100, 7.0)   // Reg 1269: Jac_2 (A/m²)
	putFloat(116, -1.10) // Reg 1277: Eon_2 (V)
	putFloat(120, -0.88) // Reg 1279: Eoff_2 (V)
	putFloat(128, 26.2)  // Reg 1283: Temp_2 (°C)
	putFloat(140, 0.0)   // Reg 1289: MetalLoss_2 (%)

	buf.Write(dataRaw)

	// 計算 CRC16
	crc := calculateCRC16(buf.Bytes())
	buf.WriteByte(byte(crc & 0xFF))
	buf.WriteByte(byte(crc >> 8))

	return buf.Bytes()
}

func calculateCRC16(data []byte) uint16 {
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
