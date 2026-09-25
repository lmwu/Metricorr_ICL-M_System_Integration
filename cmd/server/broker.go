package main

import (
	"bytes"
	"log"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

// 自訂 MQTT 事件監聽 Hook
type ClientLoggerHook struct {
	mqtt.HookBase
}

func (h *ClientLoggerHook) ID() string {
	return "client-logger"
}

func (h *ClientLoggerHook) Provides(b byte) bool {
	return bytes.Contains([]byte{
		mqtt.OnConnect,
		mqtt.OnDisconnect,
	}, []byte{b})
}

// Client 連線成功時觸發（注意：此處需回傳 error 型別，無錯誤時傳回 nil）
func (h *ClientLoggerHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	log.Printf("[Broker] 🟢 Client 連線成功 | ClientID: %s | 來源 IP: %s", cl.ID, cl.Net.Remote)
	return nil
}

// Client 斷線時觸發
func (h *ClientLoggerHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	if err != nil {
		log.Printf("[Broker] 🔴 Client 異常斷線 | ClientID: %s | 原因: %v", cl.ID, err)
	} else {
		log.Printf("[Broker] ⚪ Client 正常離線 | ClientID: %s", cl.ID)
	}
}

func StartEmbeddedMQTTBroker() *mqtt.Server {
	server := mqtt.New(nil)

	// 1. 允許所有連線（無需帳號密碼）
	_ = server.AddHook(new(auth.AllowHook), nil)

	// 2. 加入連線/斷線日誌 Hook
	_ = server.AddHook(new(ClientLoggerHook), nil)

	tcpListener := listeners.NewTCP(listeners.Config{
		ID:      "inline-mqtt-broker",
		Address: ":8888",
	})

	if err := server.AddListener(tcpListener); err != nil {
		log.Fatalf("[Broker] 新增 8888 監聽器失敗: %v", err)
	}

	go func() {
		log.Println("[Broker] 內嵌 MQTT Broker 啟動於 :8888")
		if err := server.Serve(); err != nil {
			log.Fatalf("[Broker] 異常退出: %v", err)
		}
	}()

	return server
}