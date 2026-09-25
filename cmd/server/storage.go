package main

import (
	"sync"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

type Storage struct {
	mu           sync.RWMutex
	db           *Database
	stationsData map[string]*iclmodbus.FullMeasurementData
	pollInterval time.Duration // 當前輪詢間隔，0 代表停用自動輪詢
}

func NewStorage(db *Database) *Storage {
	return &Storage{
		db:           db,
		stationsData: make(map[string]*iclmodbus.FullMeasurementData),
		pollInterval: 10 * time.Minute, // 預設 10 分鐘自動採集一次
	}
}

func (s *Storage) UpdateStationData(data *iclmodbus.FullMeasurementData) {
	s.mu.Lock()
	s.stationsData[data.StationID] = data
	s.mu.Unlock()

	// 自動異步寫入 SQLite DB 持久化儲存
	go s.db.SaveMeasurement(data)
}

func (s *Storage) GetStationIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := make([]string, 0, len(s.stationsData))
	for k := range s.stationsData {
		ids = append(ids, k)
	}
	return ids
}

func (s *Storage) RegisterStationIfAbsent(stationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.stationsData[stationID]; !exists {
		s.stationsData[stationID] = &iclmodbus.FullMeasurementData{
			StationID: stationID,
			Timestamp: "尚未採集",
		}
	}
}

func (s *Storage) GetAllStationsData() map[string]*iclmodbus.FullMeasurementData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyMap := make(map[string]*iclmodbus.FullMeasurementData)
	for k, v := range s.stationsData {
		copyMap[k] = v
	}
	return copyMap
}

func (s *Storage) SetPollInterval(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pollInterval = d
}

func (s *Storage) GetPollInterval() time.Duration {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pollInterval
}