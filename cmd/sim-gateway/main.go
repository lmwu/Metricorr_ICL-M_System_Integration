package main

import (
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

const (
	MqttBroker = "tcp://localhost:1883"
	TopicCMD   = "gateway/ICL001/rx"
	TopicRESP  = "gateway/ICL001/tx"
	ComPort    = "COM3" // 請修改為電腦實際 COM 埠 (Linux 例: /dev/ttyUSB0)
	BaudRate   = 19200  // 依據 ICL-M 預設值[cite: 1]
)

func main() {
	mode := &serial.Mode{
		BaudRate: BaudRate,
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(ComPort, mode)
	if err != nil {
		log.Fatalf("無法開啟串口 %s: %v", ComPort, err)
	}
	defer port.Close()
	log.Printf("[SimGateway] 成功連線 RS485 串口: %s", ComPort)

	opts := mqtt.NewClientOptions().AddBroker(MqttBroker).SetClientID("Golang_LTE_Sim_Gateway")
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("[SimGateway] 已連線至 MQTT Broker")
		c.Subscribe(TopicCMD, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()
			log.Printf("[SimGateway] [MQTT -> RS485] 寫入: %X", payload)
			port.Write(payload)
		})
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[SimGateway] MQTT 連線失敗: %v", token.Error())
	}

	buf := make([]byte, 512)
	for {
		n, err := port.Read(buf)
		if err != nil {
			log.Printf("[SimGateway] 串口讀取錯誤: %v", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}
		if n > 0 {
			receivedData := make([]byte, n)
			copy(receivedData, buf[:n])
			log.Printf("[SimGateway] [RS485 -> MQTT] 透傳: %X", receivedData)
			client.Publish(TopicRESP, 0, false, receivedData)
		}
	}
}