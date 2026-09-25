package main

import (
	"flag"
	"log"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"go.bug.st/serial"
)

func main() {
	stationID := flag.String("station", "ST-TXG-01", "測站 ID (如 ST-TXG-01)")
	comPort := flag.String("port", "COM3", "RS485 COM 埠")
	brokerAddr := flag.String("broker", "tcp://127.0.0.1:8888", "MQTT Broker 位址")
	flag.Parse()

	mode := &serial.Mode{
		BaudRate: 19200, // 依據 ICL-M 預設值[cite: 1]
		DataBits: 8,
		Parity:   serial.NoParity,
		StopBits: serial.OneStopBit,
	}
	port, err := serial.Open(*comPort, mode)
	if err != nil {
		log.Fatalf("[SimGateway] 無法開啟 RS485 串口 %s: %v", *comPort, err)
	}
	defer port.Close()

	topicCMD := "cp-gateway/" + *stationID + "/rx"
	topicRESP := "cp-gateway/" + *stationID + "/tx"

	opts := mqtt.NewClientOptions().AddBroker(*brokerAddr).SetClientID("SimGateway_" + *stationID)
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("[SimGateway] 測站 %s 已連線至 MQTT Broker", *stationID)
		c.Subscribe(topicCMD, 0, func(client mqtt.Client, msg mqtt.Message) {
			payload := msg.Payload()
			log.Printf("[SimGateway] [%s] 收到下發指令，寫入 RS485: %X", *stationID, payload)
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
		if err == nil && n > 0 {
			resp := make([]byte, n)
			copy(resp, buf[:n])
			log.Printf("[SimGateway] [%s] 透傳 RS485 響應至 MQTT: %X", *stationID, resp)
			client.Publish(topicRESP, 0, false, resp)
		}
		time.Sleep(50 * time.Millisecond)
	}
}