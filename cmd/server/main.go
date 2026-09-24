package main

import (
	"log"
	"net/http"
	"time"
)

func main() {
	broker := StartEmbeddedMQTTBroker()
	defer broker.Close()

	// 等待 Broker 監聽埠口建立完畢
	time.Sleep(500 * time.Millisecond)

	storage := NewStorage()
	mqttSvc := NewMQTTService()
	router := NewRouter(mqttSvc, storage)

	serverAddr := ":8080"
	log.Printf("[Server] 啟動 REST API & Dashboard Server 於 http://localhost%s\n", serverAddr)
	if err := http.ListenAndServe(serverAddr, router.SetupRoutes()); err != nil {
		log.Fatalf("[Server] 服務器啟動失敗: %v", err)
	}
}
