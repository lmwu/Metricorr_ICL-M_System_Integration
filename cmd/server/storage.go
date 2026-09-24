package main

import (
	"sync"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

type Storage struct {
	mu           sync.RWMutex
	stationsData map[string]*iclmodbus.MeasurementData // Key: station_id (例如 STATION-001)
}

func NewStorage() *Storage {
	return &Storage{
		stationsData: make(map[string]*iclmodbus.MeasurementData),
	}
}

func (s *Storage) UpdateStationData(stationID string, data *iclmodbus.MeasurementData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stationsData[stationID] = data
}

func (s *Storage) GetStationData(stationID string) (*iclmodbus.MeasurementData, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, exists := s.stationsData[stationID]
	return data, exists
}

func (s *Storage) GetAllStationsData() map[string]*iclmodbus.MeasurementData {
	s.mu.RLock()
	defer s.mu.RUnlock()

	copyMap := make(map[string]*iclmodbus.MeasurementData)
	for k, v := range s.stationsData {
		copyMap[k] = v
	}
	return copyMap
}
