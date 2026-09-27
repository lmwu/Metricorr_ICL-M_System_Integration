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

// GatewayIdentifier 設備辨識函式型態
// 傳入 ClientID，回傳對應的 StationID 與是否為有效的 RMU/網關設備
type GatewayIdentifier func(clientID string) (stationID string, isGateway bool)

// DefaultGatewayIdentifier 預設的多型態 RMU 辨識器
// 未來若新增不同類型的 RMU，只需在此增加 prefix 判斷即可
func DefaultGatewayIdentifier(clientID string) (string, bool) {
	// 支援的 RMU / 網關 ClientID 前綴清單
	supportedPrefixes := []string{
		"GATEWAY-", // 標準 Modbus 網關
		"RMU-",     // 一般遠端監控單元
		"ICL-",     // Metricorr 專用 RMU
	}

	for _, prefix := range supportedPrefixes {
		if strings.HasPrefix(clientID, prefix) {
			stationID := strings.TrimPrefix(clientID, prefix)
			return stationID, true
		}
	}
	return "", false
}

// ClientLoggerHook 監聽 Mochi-MQTT Broker 的 Client 連線與斷線事件
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

// 設備 Socket 連線成功事件
func (h *ClientLoggerHook) OnConnect(cl *mochi.Client, pk packets.Packet) error {
	log.Printf("[Broker] 🟢 Client 連線成功 | ClientID: %s | Remote IP: %s", cl.ID, cl.Net.Remote)

	if stationID, ok := h.identifier(cl.ID); ok {
		h.storage.SetStationStatus(stationID, true)
		log.Printf("[Broker] 識別到 RMU 設備上線: %s (StationID: %s)", cl.ID, stationID)
	}
	return nil
}

// 設備 Socket 斷開事件 (包含異常斷電、拔網線、網路中斷)
func (h *ClientLoggerHook) OnDisconnect(cl *mochi.Client, err error, expire bool) {
	log.Printf("[Broker] 🔴 Client 斷開連線 | ClientID: %s | 原因: %v", cl.ID, err)

	if stationID, ok := h.identifier(cl.ID); ok {
		h.storage.SetStationStatus(stationID, false)
		log.Printf("[Broker] 識別到 RMU 設備離線: %s (StationID: %s)", cl.ID, stationID)
	}
}

// ========================================================
// MQTT 業務通訊服務 (負責與各類 RMU 下發指令與接收回應)
// ========================================================

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

		// 🌟 萬用主題訂閱：支援通用架構 +/+ /tx (例如: cp-gateway/STATION-01/tx 或 rmu-v2/STATION-02/tx)
		c.Subscribe("+/+/tx", 0, func(client mqtt.Client, msg mqtt.Message) {
			topicParts := strings.Split(msg.Topic(), "/")
			if len(topicParts) < 3 {
				return
			}
			stationID := topicParts[1] // 取得 Topic 中的 StationID

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

// 通用 RMU 指令下發介面 (支援動態傳入不同的 Topic 前綴)
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

// 相容舊版的指令下發介面 (預設 Topic 前綴為 cp-gateway)
func (s *MQTTService) SendStationCommand(stationID string, payload []byte, timeout time.Duration) ([]byte, error) {
	return s.SendRMUCommand("cp-gateway", stationID, payload, timeout)
}

func (s *MQTTService) Close() {
	if s.client != nil && s.client.IsConnected() {
		s.client.Disconnect(250)
	}
}