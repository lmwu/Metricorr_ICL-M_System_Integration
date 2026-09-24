package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

const MqttBroker = "tcp://localhost:1883"

type MQTTService struct {
	client    mqtt.Client
	storage   *Storage
	waitChans map[string]chan []byte // Key: station_id
	mu        sync.Mutex
}

func NewMQTTService(storage *Storage) *MQTTService {
	s := &MQTTService{
		storage:   storage,
		waitChans: make(map[string]chan []byte),
	}

	opts := mqtt.NewClientOptions().AddBroker(MqttBroker).SetClientID("CP_Onshore_Backend_Server")

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("[Server] MQTT 已連線，開始訂閱所有陸域 CP 測站通道...")

		// 訂閱所有測站的回傳 Topic: cp-gateway/+/tx
		c.Subscribe("cp-gateway/+/tx", 0, func(client mqtt.Client, msg mqtt.Message) {
			topic := msg.Topic()
			var stationID string
			fmt.Sscanf(topic, "cp-gateway/%s/tx", &stationID)

			payload := msg.Payload()
			log.Printf("[MQTT 接收] 來自測站 %s, 資料長度: %d bytes", stationID, len(payload))

			s.mu.Lock()
			if ch, exists := s.waitChans[stationID]; exists {
				ch <- payload
			}
			s.mu.Unlock()
		})
	})

	s.client = mqtt.NewClient(opts)
	if token := s.client.Connect(); token.Wait() && token.Error() != nil {
		log.Fatalf("[Server] MQTT 連線失敗: %v", token.Error())
	}

	return s
}

// SendStationCommand 下發 Modbus 指令給指定陸域測站網關
func (s *MQTTService) SendStationCommand(stationID string, cmdFrame []byte, timeout time.Duration) ([]byte, error) {
	respChan := make(chan []byte, 1)

	s.mu.Lock()
	s.waitChans[stationID] = respChan
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.waitChans, stationID)
		s.mu.Unlock()
	}()

	targetTopic := fmt.Sprintf("cp-gateway/%s/rx", stationID)
	log.Printf("[MQTT 發送] 至測站 [%s] Topic: %s", stationID, targetTopic)
	s.client.Publish(targetTopic, 0, false, cmdFrame)

	select {
	case resp := <-respChan:
		if !iclmodbus.VerifyCRC16(resp) {
			return nil, fmt.Errorf("測站 %s 回應之 Modbus CRC16 校驗失敗", stationID)
		}
		return resp, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("測站 %s 回應超時 (4G 訊號不良或設備未開機)", stationID)
	}
}

// SendCommandAndWait 舊版相容函式 (預設對 STATION-001 操作)
func (s *MQTTService) SendCommandAndWait(cmdFrame []byte, timeout time.Duration) ([]byte, error) {
	return s.SendStationCommand("STATION-001", cmdFrame, timeout)
}