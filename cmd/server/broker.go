package main

import (
	"log"

	mqtt "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/listeners"
)

// StartEmbeddedMQTTBroker 在背景啟動純 Golang MQTT Broker
func StartEmbeddedMQTTBroker() *mqtt.Server {
	// 建立 MQTT Server 實例
	server := mqtt.New(nil)

	// 建立 TCP 監聽器，監聽預設 1883 埠
	tcp := listeners.NewTCP(listeners.Config{
		ID:      "inline-mqtt-broker",
		Address: ":1883",
	})

	err := server.AddListener(tcp)
	if err != nil {
		log.Fatalf("[MQTT Broker] 新增 TCP 監聽器失敗: %v", err)
	}

	// 於 Goroutine 異步啟動 Broker
	go func() {
		log.Println("[MQTT Broker] 內建 MQTT Broker 已成功啟動於 :1883 埠...")
		if err := server.Serve(); err != nil {
			log.Fatalf("[MQTT Broker] 服務運行異常: %v", err)
		}
	}()

	return server
}