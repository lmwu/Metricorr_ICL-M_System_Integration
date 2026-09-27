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

	// 1. 若有指定實體 COM Port，由主程序統一開啟一次
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

	// 2. 為每個測站啟動獨立的 MQTT Worker 協程
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
		SetAutoReconnect(true).
		SetKeepAlive(15 * time.Second) // 💡 保持適當的 KeepAlive 間隔

	// 💡 【第一道防線】：設定 MQTT LWT (遺言機制)
	// 當網關突發斷電或網路斷開時，MQTT Broker 會自動代發 OFFLINE 至 status 主題
	opts.SetWill(topicStatus, "OFFLINE", 0, false)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[%s] 🟢 MQTT 已成功連線至 Broker", clientID)

		// 發送網關上線狀態廣播
		c.Publish(topicStatus, 0, false, []byte("ONLINE"))
		log.Printf("[%s] 📡 已發送上線狀態廣播至 Topic: %s", clientID, topicStatus)

		// 💡【新增】：啟動背景心跳定時器（每 15 秒自動報到，防止被 Server 逾時剔除）
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				if c.IsConnected() {
					c.Publish(topicStatus, 0, false, "ONLINE")
				} else {
					return
				}
			}
		}()

		// 訂閱來自 Server 的指令 Topic
		token := c.Subscribe(topicRx, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()

			if serialPort != nil {
				serialMutex.Lock()
				defer serialMutex.Unlock()

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

func readFullModbusResponse(port serial.Port, reqPayload []byte, timeout time.Duration) []byte {
	buf := make([]byte, 256)
	var fullResp []byte
	startTime := time.Now()

	expectedLen := 0
	if len(reqPayload) >= 2 {
		funcCode := reqPayload[1]
		if funcCode == 0x06 {
			expectedLen = 8
		}
	}

	for time.Since(startTime) < timeout {
		n, err := port.Read(buf)
		if err == nil && n > 0 {
			fullResp = append(fullResp, buf[:n]...)

			if expectedLen > 0 && len(fullResp) >= expectedLen {
				return fullResp[:expectedLen]
			}

			if len(fullResp) >= 3 && fullResp[1] == 0x03 {
				targetLen := 3 + int(fullResp[2]) + 2
				if len(fullResp) >= targetLen {
					return fullResp[:targetLen]
				}
			}

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

	if funcCode == 0x06 && regAddr == 1199 {
		return payload
	}
	if funcCode == 0x03 && regAddr == 1200 {
		buf := new(bytes.Buffer)
		buf.Write([]byte{slaveID, 0x03, 0x02, 0x00, 0x03})
		crc := iclmodbus.CalculateCRC16(buf.Bytes())
		buf.Write([]byte{byte(crc & 0xFF), byte(crc >> 8)})
		return buf.Bytes()
	}
	if funcCode == 0x03 && regAddr == 1219 {
		return generateChannelMockData(slaveID, 185.5, -0.92, 12.8)
	}
	if funcCode == 0x03 && regAddr == 1263 {
		return generateChannelMockData(slaveID, 200.0, -0.88, 7.0)
	}
	return nil
}

func generateChannelMockData(slaveID byte, thickness, eOff, jAc float32) []byte {
	buf := new(bytes.Buffer)
	buf.Write([]byte{slaveID, 0x03, 56})
	dataRaw := make([]byte, 56)
	putF := func(o int, v float32) { binary.BigEndian.PutUint32(dataRaw[o:o+4], math.Float32bits(v)) }

	putF(0, thickness)
	putF(12, jAc)
	putF(32, eOff)

	buf.Write(dataRaw)
	crc := iclmodbus.CalculateCRC16(buf.Bytes())
	buf.Write([]byte{byte(crc & 0xFF), byte(crc >> 8)})
	return buf.Bytes()
}
