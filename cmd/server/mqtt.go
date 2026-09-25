package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

const MqttBroker = "tcp://127.0.0.1:8888"

type MQTTService struct {
	client       mqtt.Client            //[cite: 7]
	storage      *Storage               //[cite: 7]
	waitChans    map[string]chan []byte // Key: station_id[cite: 7]
	stationLocks map[string]*sync.Mutex // 補上：測站專屬併發鎖
	mu           sync.Mutex             //[cite: 7]
}

func NewMQTTService(storage *Storage) *MQTTService {
	s := &MQTTService{
		storage:      storage,                       //[cite: 7]
		waitChans:    make(map[string]chan []byte),  //[cite: 7]
		stationLocks: make(map[string]*sync.Mutex), // 初始化 map
	}

	opts := mqtt.NewClientOptions().AddBroker(MqttBroker).SetClientID("CP_Onshore_Backend_Server") //[cite: 7]

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("[Server] MQTT 已連線，開始監聽 cp-gateway/+/tx 所有測站...") //[cite: 7]

		c.Subscribe("cp-gateway/+/tx", 0, func(client mqtt.Client, msg mqtt.Message) { //[cite: 7]
			topic := msg.Topic()               //[cite: 7]
			parts := strings.Split(topic, "/") //[cite: 7]
			if len(parts) < 3 {
				return //[cite: 7]
			}
			stationID := parts[1] //[cite: 7]

			s.storage.RegisterStationIfAbsent(stationID) //[cite: 7]
			payload := msg.Payload()                     //[cite: 7]

			s.mu.Lock() //[cite: 7]
			if ch, exists := s.waitChans[stationID]; exists { //[cite: 7]
				select {
				case ch <- payload:
				default:
				}
			}
			s.mu.Unlock() //[cite: 7]
		})
	})

	s.client = mqtt.NewClient(opts) //[cite: 7]
	if token := s.client.Connect(); token.Wait() && token.Error() != nil { //[cite: 7]
		log.Fatalf("[Server] MQTT 連線失敗: %v", token.Error()) //[cite: 7]
	}

	return s //[cite: 7]
}

// 補上：取得或建立特定測站的獨立互斥鎖
func (s *MQTTService) getStationLock(stationID string) *sync.Mutex {
	s.mu.Lock()
	defer s.mu.Unlock()
	lock, exists := s.stationLocks[stationID]
	if !exists {
		lock = &sync.Mutex{}
		s.stationLocks[stationID] = lock
	}
	return lock
}

func (s *MQTTService) SendStationCommand(stationID string, cmdFrame []byte, timeout time.Duration) ([]byte, error) {
	// 同一測站順序執行採集，避免通道碰撞
	stLock := s.getStationLock(stationID)
	stLock.Lock()
	defer stLock.Unlock()

	respChan := make(chan []byte, 1) //[cite: 7]

	s.mu.Lock()                       //[cite: 7]
	s.waitChans[stationID] = respChan //[cite: 7]
	s.mu.Unlock()                     //[cite: 7]

	defer func() {
		s.mu.Lock()                    //[cite: 7]
		delete(s.waitChans, stationID) //[cite: 7]
		s.mu.Unlock()                  //[cite: 7]
	}()

	targetTopic := fmt.Sprintf("cp-gateway/%s/rx", stationID) //[cite: 7]
	s.client.Publish(targetTopic, 0, false, cmdFrame)         //[cite: 7]

	select {
	case resp := <-respChan: //[cite: 7]
		if !iclmodbus.VerifyCRC16(resp) { //[cite: 7, 12]
			return nil, fmt.Errorf("測站 %s Modbus CRC16 校驗失敗", stationID) //[cite: 7]
		}
		return resp, nil //[cite: 7]
	case <-time.After(timeout): //[cite: 7]
		return nil, fmt.Errorf("測站 %s 通訊超時", stationID) //[cite: 7]
	}
}