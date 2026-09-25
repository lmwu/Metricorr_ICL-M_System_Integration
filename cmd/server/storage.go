package main

import (
	"sync"
	"time"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

type GatewayInfo struct {
	ClientID   string    `json:"client_id"`
	StationID  string    `json:"station_id"`
	Online     bool      `json:"online"`
	RemoteIP   string    `json:"remote_ip"`
	LastSeenAt time.Time `json:"last_seen_at"`
}

type Storage struct {
	mu           sync.RWMutex
	db           *Database
	stationsData map[string]*iclmodbus.FullMeasurementData
	gateways     map[string]*GatewayInfo
	pollInterval time.Duration
}

func NewStorage(db *Database) *Storage {
	return &Storage{
		db:           db,
		stationsData: make(map[string]*iclmodbus.FullMeasurementData),
		gateways:     make(map[string]*GatewayInfo),
		pollInterval: 10 * time.Minute,
	}
}

// 更新網關的上線與離線動態（並自動註冊測站看板）
func (s *Storage) SetGatewayOnlineStatus(clientID, remoteIP string, online bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stationID := clientID
	if len(clientID) > 8 && clientID[:8] == "GATEWAY-" {
		stationID = clientID[8:]
	}

	s.gateways[clientID] = &GatewayInfo{
		ClientID:   clientID,
		StationID:  stationID,
		Online:     online,
		RemoteIP:   remoteIP,
		LastSeenAt: time.Now(),
	}

	// 自動發現機制：網關上線時即自動註冊測站，解除巡檢死鎖
	if online {
		s.registerStationIfAbsentLocked(stationID)
	}
}

func (s *Storage) registerStationIfAbsentLocked(stationID string) {
	if _, exists := s.stationsData[stationID]; !exists {
		s.stationsData[stationID] = &iclmodbus.FullMeasurementData{
			StationID: stationID,
			Timestamp: "已連線（待首採）",
		}
	}
}

func (s *Storage) RegisterStationIfAbsent(stationID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.registerStationIfAbsentLocked(stationID)
}

func (s *Storage) GetGatewaysStatus() []*GatewayInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*GatewayInfo, 0, len(s.gateways))
	for _, gw := range s.gateways {
		list = append(list, gw)
	}
	return list
}

func (s *Storage) UpdateStationData(data *iclmodbus.FullMeasurementData) {
	s.mu.Lock()
	s.stationsData[data.StationID] = data
	s.mu.Unlock()

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