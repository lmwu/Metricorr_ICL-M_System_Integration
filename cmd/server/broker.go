package main

import (
	"bytes"
	"log"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/mochi-mqtt/server/v2/packets"
)

type ClientLoggerHook struct {
	mqtt.HookBase
	storage *Storage
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

func (h *ClientLoggerHook) OnConnect(cl *mqtt.Client, pk packets.Packet) error {
	log.Printf("[Broker] 🟢 Client 連線成功 | ClientID: %s | 來源 IP: %s", cl.ID, cl.Net.Remote)
	
	// 💡 【新增過濾】若為後端服務自身連線，不將其寫入 Storage 作為測站
	if cl.ID == "CP_Onshore_Backend_Server" {
		return nil
	}

	if h.storage != nil {
		h.storage.SetGatewayOnlineStatus(cl.ID, cl.Net.Remote, true)
	}
	return nil
}

func (h *ClientLoggerHook) OnDisconnect(cl *mqtt.Client, err error, expire bool) {
	if err != nil {
		log.Printf("[Broker] 🔴 Client 異常斷線 | ClientID: %s | 原因: %v", cl.ID, err)
	} else {
		log.Printf("[Broker] ⚪ Client 正常離線 | ClientID: %s", cl.ID)
	}

	// 💡 【新增過濾】離線時同樣忽略後端服務自身 ID
	if cl.ID == "CP_Onshore_Backend_Server" {
		return
	}

	if h.storage != nil {
		h.storage.SetGatewayOnlineStatus(cl.ID, cl.Net.Remote, false)
	}
}

func StartEmbeddedMQTTBroker(storage *Storage) *mqtt.Server {
	server := mqtt.New(nil)

	_ = server.AddHook(new(auth.AllowHook), nil)
	_ = server.AddHook(&ClientLoggerHook{storage: storage}, nil)

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