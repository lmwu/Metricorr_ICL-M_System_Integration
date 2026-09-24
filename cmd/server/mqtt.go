package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

const (
	MqttBroker = "tcp://localhost:1883"
	TopicCMD   = "gateway/ICL001/rx"
	TopicRESP  = "gateway/ICL001/tx"
)

type MQTTService struct {
	client   mqtt.Client
	respChan chan []byte
	mu       sync.Mutex
}

func NewMQTTService() *MQTTService {
	s := &MQTTService{
		respChan: make(chan []byte, 10),
	}

	opts := mqtt.NewClientOptions().AddBroker(MqttBroker).SetClientID("Backend_API_Server")
	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("[Server] MQTT 已連線")
		c.Subscribe(TopicRESP, 0, func(client mqtt.Client, msg mqtt.Message) {
			s.respChan <- msg.Payload()
		})
	})

	s.client = mqtt.NewClient(opts)
	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[Server] MQTT 連線失敗: %v", token.Error())
	}

	return s
}

func (s *MQTTService) SendCommandAndWait(cmdFrame []byte, timeout time.Duration) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for len(s.respChan) > 0 {
		<-s.respChan
	}

	log.Printf("[Server] 發送 Modbus 指令: %X", cmdFrame)
	s.client.Publish(TopicCMD, 0, false, cmdFrame)

	select {
	case resp := <-s.respChan:
		if !iclmodbus.VerifyCRC16(resp) {
			return nil, fmt.Errorf("CRC16 校驗錯誤")
		}
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("Modbus 指令回應超時")
	}
}