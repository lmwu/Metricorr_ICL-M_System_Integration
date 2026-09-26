package main

import (
	"fmt"
	"log"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTService struct {
	client   mqtt.Client
	respMap  sync.Map // key: stationID, value: chan []byte
	cmdLocks sync.Map // key: stationID, value: *sync.Mutex (防止請求併發覆蓋)
}

func NewMQTTService(brokerURL string) (*MQTTService, error) {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(brokerURL)
	opts.SetClientID("metricorr-server-" + fmt.Sprintf("%d", time.Now().UnixNano()))
	opts.SetAutoReconnect(true)
	opts.SetConnectRetry(true)

	svc := &MQTTService{}

	opts.SetDefaultPublishHandler(func(client mqtt.Client, msg mqtt.Message) {
		var stationID string
		if _, err := fmt.Sscanf(msg.Topic(), "cp-gateway/%s/tx", &stationID); err == nil {
			if val, ok := svc.respMap.Load(stationID); ok {
				ch := val.(chan []byte)
				select {
				case ch <- msg.Payload():
				default:
				}
			}
		}
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	svc.client = client

	// 訂閱回應 Topic
	if token := client.Subscribe("cp-gateway/+/tx", 0, nil); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	log.Println("[MQTT] 🔌 已成功連線至 Broker 並完成 Topic 訂閱")
	return svc, nil
}

// SendStationCommand 發送指令給特定測站（具備測站等級防併發覆蓋鎖）
func (s *MQTTService) SendStationCommand(stationID string, payload []byte, timeout time.Duration) ([]byte, error) {
	// 取得或初始化測站專屬 Mutex
	lockVal, _ := s.cmdLocks.LoadOrStore(stationID, &sync.Mutex{})
	stationLock := lockVal.(*sync.Mutex)
	
	stationLock.Lock()
	defer stationLock.Unlock()

	respCh := make(chan []byte, 1)
	s.respMap.Store(stationID, respCh)
	defer s.respMap.Delete(stationID)

	topicRx := fmt.Sprintf("cp-gateway/%s/rx", stationID)
	token := s.client.Publish(topicRx, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		return nil, fmt.Errorf("MQTT 發送失敗: %v", token.Error())
	}

	select {
	case data := <-respCh:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("[%s] 指令回應超時", stationID)
	}
}

func (s *MQTTService) Close() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
	}
}