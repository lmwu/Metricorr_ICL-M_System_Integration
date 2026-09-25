package main

import (
	"log"

	mqtt "github.com/mochi-mqtt/server/v2"
    "github.com/mochi-mqtt/server/v2/listeners"
)

func StartEmbeddedMQTTBroker() *mqtt.Server {
	server := mqtt.New(nil)
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