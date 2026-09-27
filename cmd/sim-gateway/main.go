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
	"sync"
	"syscall"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

func main() {
	stationFlag := flag.String("station", "STATION-001", "測站 ID (多個以逗號分隔)")
	brokerURL := flag.String("broker", "tcp://localhost:8888", "MQTT Broker 位址")
	portName := flag.String("port", "", "實體 RS485 COM Port (不填為純 Mock 模式)")
	baudRate := flag.Int("baud", 115200, "波特率")
	flag.Parse()

	var sharedPort serial.Port
	var sharedMutex sync.Mutex
	var err error

	// 1. 若有指定實體 COM Port，由主程序統一開啟一次，避免多個 Worker 搶佔同一埠號導致崩潰
	if *portName != "" {
		mode := &serial.Mode{
			BaudRate: *baudRate,
			DataBits: 8,
			Parity:   serial.NoParity,
			StopBits: serial.OneStopBit,
		}
		sharedPort, err = serial.Open(*portName, mode)
		if err != nil {
			log.Fatalf("❌ RS485 串口開啟失敗 (%s): %v", *portName, err)
		}
		defer sharedPort.Close()
		log.Printf("🔌 實體 RS485 串口已開啟成功: %s (%d bps)", *portName, *baudRate)
	} else {
		log.Println("🤖 未指定實體串口 (-port)，全數測站啟用軟體 Mock 模式")
	}

	// 2. 為每個測站啟動獨立的 MQTT Worker 協程 (共享同一 RS485 實體線路與 Mutex 鎖)
	stations := strings.Split(*stationFlag, ",")
	for _, stationID := range stations {
		stationID = strings.TrimSpace(stationID)
		if stationID != "" {
			go runGatewayWorker(stationID, *brokerURL, sharedPort, &sharedMutex)
		}
	}

	// 3. 安全關機訊號監聽
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("[Gateway] 收到關機訊號，網關服務已安全終止")
}

func runGatewayWorker(stationID, brokerURL string, serialPort serial.Port, serialMutex *sync.Mutex) {
	clientID := fmt.Sprintf("GATEWAY-%s", stationID)
	topicRx := fmt.Sprintf("cp-gateway/%s/rx", stationID)
	topicTx := fmt.Sprintf("cp-gateway/%s/tx", stationID)
	topicStatus := fmt.Sprintf("cp-gateway/%s/status", stationID)

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[%s] 🟢 MQTT 已成功連線至 Broker", clientID)

		// 發送網關上線狀態廣播
		c.Publish(topicStatus, 0, false, []byte("ONLINE"))
		log.Printf("[%s] 📡 已發送上線狀態廣播至 Topic: %s", clientID, topicStatus)

		// 訂閱來自 Server 的指令 Topic
		token := c.Subscribe(topicRx, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()

			if serialPort != nil {
				serialMutex.Lock()
				defer serialMutex.Unlock()

				// 發送前清空 RX 快取，防止殘留髒數據影響長度與 CRC 判斷
				_ = serialPort.ResetInputBuffer()

				_, writeErr := serialPort.Write(payload)
				if writeErr == nil {
					respData := readFullModbusResponse(serialPort, payload, 2*time.Second)
					if len(respData) > 0 {
						client.Publish(topicTx, 0, false, respData)
					} else {
						log.Printf("[%s] ⚠️ 實體設備回應超時或無效", clientID)
					}
				} else {
					log.Printf("[%s] ❌ 寫入串口失敗: %v", clientID, writeErr)
				}
			} else {
				// 軟體 Mock 模式回應
				mockResp := handleMockRequest(payload)
				if mockResp != nil {
					client.Publish(topicTx, 0, false, mockResp)
				}
			}
		})

		if token.Wait() && token.Error() != nil {
			log.Printf("[%s] ❌ 訂閱 Topic %s 失敗: %v", clientID, topicRx, token.Error())
		}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[%s] ❌ MQTT 連線失敗: %v", clientID, token.Error())
	}
}

// readFullModbusResponse 精準 Modbus RTU 串口讀取器 (支援動態長度計算與 Exception 攔截)
func readFullModbusResponse(port serial.Port, reqPayload []byte, timeout time.Duration) []byte {
	buf := make([]byte, 256)
	var fullResp []byte
	startTime := time.Now()

	expectedLen := 0
	if len(reqPayload) >= 2 {
		funcCode := reqPayload[1]
		if funcCode == 0x06 {
			expectedLen = 8 // Write Single Register 回應固定為 8 Bytes
		}
	}

	for time.Since(startTime) < timeout {
		n, err := port.Read(buf)
		if err == nil && n > 0 {
			fullResp = append(fullResp, buf[:n]...)

			// 1. 如果是 Func 06，滿 8 碼立即返回
			if expectedLen > 0 && len(fullResp) >= expectedLen {
				return fullResp[:expectedLen]
			}

			// 2. 如果是 Func 03，動態計算 Header(3) + ByteCount + CRC(2)
			if len(fullResp) >= 3 && fullResp[1] == 0x03 {
				targetLen := 3 + int(fullResp[2]) + 2
				if len(fullResp) >= targetLen {
					return fullResp[:targetLen]
				}
			}

			// 3. 遇到 Modbus Exception (FuncCode >= 0x80)，回應固定 5 位元組，立即返回
			if len(fullResp) >= 5 && fullResp[1] >= 0x80 {
				return fullResp[:5]
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fullResp
}

func handleMockRequest(payload []byte) []byte {
	if len(payload) < 6 {
		return nil
	}
	slaveID, funcCode := payload[0], payload[1]
	regAddr := binary.BigEndian.Uint16(payload[2:4])

	// Func 06 Write Reg 1199 (觸發深採) -> 原樣回覆作為成功確認
	if funcCode == 0x06 && regAddr == 1199 {
		return payload
	}
	// Func 03 Read Reg 1200 (讀取狀態) -> 回覆 0x0003 (Completed)
	if funcCode == 0x03 && regAddr == 1200 {
		buf := new(bytes.Buffer)
		buf.Write([]byte{slaveID, 0x03, 0x02, 0x00, 0x03})
		crc := iclmodbus.CalculateCRC16(buf.Bytes())
		buf.Write([]byte{byte(crc & 0xFF), byte(crc >> 8)})
		return buf.Bytes()
	}
	// Func 03 Read Reg 1219 (Ch1 數據 28 個暫存器)
	if funcCode == 0x03 && regAddr == 1219 {
		return generateChannelMockData(slaveID, 185.5, -0.92, 12.8)
	}
	// Func 03 Read Reg 1263 (Ch2 數據 28 個暫存器)
	if funcCode == 0x03 && regAddr == 1263 {
		return generateChannelMockData(slaveID, 200.0, -0.88, 7.0)
	}
	return nil
}

func generateChannelMockData(slaveID byte, thickness, eOff, jAc float32) []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{slaveID, 0x03, 56}) // Header + 56 Bytes Data
	dataRaw := make([]byte, 56)
	putF := func(o int, v float32) { binary.BigEndian.PutUint32(dataRaw[o:o+4], math.Float32bits(v)) }

	putF(0, thickness) // Reg 1219 / 1263 (Offset 0)
	putF(12, jAc)      // Reg 1225 / 1269 (Offset 12)
	putF(32, eOff)     // Reg 1235 / 1279 (Offset 32)

	buf.Write(dataRaw)
	crc := iclmodbus.CalculateCRC16(buf.Bytes())
	buf.Write([]byte{byte(crc & 0xFF), byte(crc >> 8)})
	return buf.Bytes()
}