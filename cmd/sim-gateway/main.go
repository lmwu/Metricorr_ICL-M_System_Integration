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

	stations := strings.Split(*stationFlag, ",")
	for _, stationID := range stations {
		stationID = strings.TrimSpace(stationID)
		if stationID != "" {
			go runGatewayWorker(stationID, *brokerURL, *portName, *baudRate)
		}
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("[Gateway] 網關服務已終止")
}

func runGatewayWorker(stationID, brokerURL, portName string, baudRate int) {
	clientID := fmt.Sprintf("GATEWAY-%s", stationID)
	topicRx := fmt.Sprintf("cp-gateway/%s/rx", stationID)
	topicTx := fmt.Sprintf("cp-gateway/%s/tx", stationID)

	var serialMutex sync.Mutex
	var serialPort serial.Port
	var err error

	if portName != "" {
		mode := &serial.Mode{BaudRate: baudRate, DataBits: 8, Parity: serial.NoParity, StopBits: serial.OneStopBit}
		serialPort, err = serial.Open(portName, mode)
		if err != nil {
			log.Fatalf("[%s] ❌ RS485 串口開啟失敗 (%s): %v", stationID, portName, err)
		}
		defer serialPort.Close()
		log.Printf("[%s] 🔌 實體 RS485 串口已連線: %s (%d bps)", stationID, portName, baudRate)
	} else {
		log.Printf("[%s] 🤖 未指定實體串口，啟用軟體 Mock 模式", stationID)
	}

	opts := mqtt.NewClientOptions().AddBroker(brokerURL).SetClientID(clientID).SetAutoReconnect(true)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[%s] 🟢 MQTT 已連線", clientID)
		c.Subscribe(topicRx, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()

			if serialPort != nil {
				serialMutex.Lock()
				_, writeErr := serialPort.Write(payload)
				if writeErr == nil {
					respData := readFullModbusResponse(serialPort, payload, 2*time.Second)
					if len(respData) > 0 {
						client.Publish(topicTx, 0, false, respData)
					}
				}
				serialMutex.Unlock()
			} else {
				mockResp := handleMockRequest(payload)
				if mockResp != nil {
					client.Publish(topicTx, 0, false, mockResp)
				}
			}
		})
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[%s] ❌ MQTT 連線失敗: %v", clientID, token.Error())
	}
}

// 修正後的精準 Modbus RTU 串口讀取器
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

			if len(fullResp) >= 5 && fullResp[1] >= 0x80 {
				return fullResp[:5] // 遇到 Exception 立即返回，不浪費 2 秒等待
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
