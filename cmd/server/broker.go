package main

import (
	"bytes"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/packets"
)

type GatewayIdentifier func(clientID string) (stationID string, isGateway bool)

func DefaultGatewayIdentifier(clientID string) (string, bool) {
	supportedPrefixes := []string{
		"GATEWAY-",
		"RMU-",
		"ICL-",
	}

	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(clientID, prefix) {
			stationID := strings.TrimPrefix(clientID, prefix)
			return stationID, true
		}
	}
	return "", false
}

type ClientLoggerHook struct {
	mochi.HookBase
	storage    *Storage
	identifier GatewayIdentifier
}

func NewClientLoggerHook(storage *Storage, identifier GatewayIdentifier) *ClientLoggerHook {
	if identifier == nil {
		identifier = DefaultGatewayIdentifier
	}
	return &ClientLoggerHook{
		storage:    storage,
		identifier: identifier,
	}
}

func (h *ClientLoggerHook) ID() string {
	return "rmu-client-logger"
}

func (h *ClientLoggerHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mochi.OnConnect,
		mochi.OnDisconnect,
	}, []byte{b})
}

func (h *ClientLoggerHook) OnConnect(cl *mochi.Client, pk packets.Packet) error {
	log.Printf("[Broker] 🟢 Client 連線成功 | ClientID: %s | Remote IP: %s", cl.ID, cl.Net.Remote)

	if stationID, ok := h.identifier(cl.ID); ok {
		h.storage.SetStationStatus(stationID, true)
		log.Printf("[Broker] 識別到 RMU 設備上線: %s (StationID: %s)", cl.ID, stationID)
	}
	return nil
}

func (h *ClientLoggerHook) OnDisconnect(cl *mochi.Client, err error, expire bool) {
	log.Printf("[Broker] 🔴 Client 斷開連線 | ClientID: %s | 原因: %v", cl.ID, err)

	if stationID, ok := h.identifier(cl.ID); ok {
		h.storage.SetStationStatus(stationID, false)
		log.Printf("[Broker] 識別到 RMU 設備離線: %s (StationID: %s)", cl.ID, stationID)
	}
}

type MQTTService struct {
	client   mqtt.Client
	storage  *Storage
	respMap  sync.Map // key: stationID, value: chan []byte
	cmdLocks sync.Map // key: stationID, value: *sync.Mutex
}

func NewMQTTService(brokerURL string, storage *Storage) (*MQTTService, error) {
	svc := &MQTTService{
		storage: storage,
	}

	opts := mqtt.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID("SERVER-CORE-SERVICE").
		SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Println("🟢 [MQTT Service] 後端 core 服務已連接至 MQTT Broker")

		// 💡 1. 訂閱狀態主題 (處理 LWT 遺言與 ONLINE / OFFLINE 主動廣播)
		c.Subscribe("+/+/status", 0, func(client mqtt.Client, msg mqtt.Message) {
			topicParts := strings.Split(msg.Topic(), "/")
			if len(topicParts) < 3 {
				return
			}
			stationID := topicParts[1]
			payload := strings.TrimSpace(string(msg.Payload()))

			switch payload {
			case "ONLINE":
				svc.storage.SetStationStatus(stationID, true)
			case "OFFLINE":
				svc.storage.SetStationStatus(stationID, false)
			}
		})

		// 💡 2. 訂閱數據回應主題
		c.Subscribe("+/+/tx", 0, func(client mqtt.Client, msg mqtt.Message) {
			topicParts := strings.Split(msg.Topic(), "/")
			if len(topicParts) < 3 {
				return
			}
			stationID := topicParts[1]

			// 只要收到數據回應，即刷新該測站的最後活躍時間
			svc.storage.TouchStation(stationID)

			if ch, ok := svc.respMap.Load(stationID); ok {
				select {
				case ch.(chan []byte) <- msg.Payload():
				default:
				}
			}
		})
	})

	svc.client = mqtt.NewClient(opts)
	if token := svc.client.Connect(); token.Wait() && token.Error() != nil {
		return nil, token.Error()
	}

	return svc, nil
}

func (s *MQTTService) SendRMUCommand(topicPrefix, stationID string, payload []byte, timeout time.Duration) ([]byte, error) {
	lockVal, _ := s.cmdLocks.LoadOrStore(stationID, &sync.Mutex{})
	stationLock := lockVal.(*sync.Mutex)

	stationLock.Lock()
	defer stationLock.Unlock()

	respCh := make(chan []byte, 1)
	s.respMap.Store(stationID, respCh)
	defer s.respMap.Delete(stationID)

	topicRx := fmt.Sprintf("%s/%s/rx", topicPrefix, stationID)
	token := s.client.Publish(topicRx, 0, false, payload)
	token.Wait()
	if token.Error() != nil {
		return nil, fmt.Errorf("MQTT 指令發送失敗: %v", token.Error())
	}

	select {
	case data := <-respCh:
		return data, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("[%s] RMU 回應超時", stationID)
	}
}

func (s *MQTTService) SendStationCommand(stationID string, payload []byte, timeout time.Duration) ([]byte, error) {
	return s.SendRMUCommand("cp-gateway", stationID, payload, timeout)
}

func (s *MQTTService) Close() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
	}
}
