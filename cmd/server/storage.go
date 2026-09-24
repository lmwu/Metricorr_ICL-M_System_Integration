package main

import (
	"sync"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

type Storage struct {
	mu         sync.RWMutex
	latestData *iclmodbus.MeasurementData
}

func NewStorage() *Storage {
	return &Storage{}
}

func (s *Storage) SetLatestData(data *iclmodbus.MeasurementData) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.latestData = data
}

func (s *Storage) GetLatestData() *iclmodbus.MeasurementData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.latestData
}