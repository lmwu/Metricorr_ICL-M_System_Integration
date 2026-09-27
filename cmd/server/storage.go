package main

import (
	"log"
	"sync"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"
)

// 💡 第二道防線：網關未收到數據/心跳的自動剔除逾時時間 (120 秒)
const StationTimeoutThreshold = 120 * time.Second

type MeasurementLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	StationID   string    `gorm:"index;type:varchar(64)" json:"station_id"`
	Channel     int       `gorm:"index" json:"channel"`
	Thickness   float32   `json:"thickness"`
	Uac         float32   `json:"uac"`
	Iac         float32   `json:"iac"`
	Jac         float32   `json:"j_ac"`
	Rs          float32   `json:"rs"`
	Idc         float32   `json:"idc"`
	Jdc         float32   `json:"j_dc"`
	Eon         float32   `json:"e_on"`
	Eoff        float32   `json:"e_off"`
	EirFree     float32   `json:"e_ir_free"`
	Temperature float32   `json:"temperature"`
	Rr          float32   `json:"rr"`
	Rc          float32   `json:"rc"`
	MetalLoss   float32   `json:"metal_loss"`
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

type Storage struct {
	db             *gorm.DB
	measuringMap   sync.Map
	onlineStations sync.Map // Key: stationID (string), Value: lastSeen (time.Time)
}

func NewStorage(dbPath string) *Storage {
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("❌ 開啟 SQLite 失敗 (%s): %v", dbPath, err)
	}

	if err := db.AutoMigrate(&MeasurementLog{}); err != nil {
		log.Fatalf("❌ 自動建立 SQLite 表格失敗: %v", err)
	}

	log.Printf("🟢 SQLite 資料庫載入完成 (WAL 模式啟動): %s", dbPath)
	return &Storage{db: db}
}

// 設定/更新網關在線狀態 (online=true 記錄當前時間, online=false 立即移除)
func (s *Storage) SetStationStatus(stationID string, online bool) {
	if online {
		s.onlineStations.Store(stationID, time.Now())
		log.Printf("🟢 [Storage] 測站 [%s] 狀態標記: 在線", stationID)
	} else {
		s.onlineStations.Delete(stationID)
		log.Printf("🔴 [Storage] 測站 [%s] 狀態標記: 離線", stationID)
	}
}

// 💡 重新整理最後活躍時間 (當收到 Modbus 回應或通訊封包時呼叫)
func (s *Storage) TouchStation(stationID string) {
	s.onlineStations.Store(stationID, time.Now())
}

// 💡 檢查特定測站目前是否在線 (同時驗證是否逾時)
func (s *Storage) IsStationOnline(stationID string) bool {
	val, loaded := s.onlineStations.Load(stationID)
	if !loaded {
		return false
	}
	lastSeen := val.(time.Time)
	if time.Since(lastSeen) > StationTimeoutThreshold {
		s.onlineStations.Delete(stationID)
		log.Printf("⚠️ [Storage] 測站 [%s] 逾時 (%v) 未回應，自動判定為離線", stationID, time.Since(lastSeen).Round(time.Second))
		return false
	}
	return true
}

// 💡 取得所有目前線上測站（第二道防線：自動剔除超過 45 秒未更新活力的網關）
func (s *Storage) GetOnlineStations() []string {
	var list []string
	now := time.Now()

	s.onlineStations.Range(func(key, value interface{}) bool {
		stationID := key.(string)
		lastSeen := value.(time.Time)

		if now.Sub(lastSeen) > StationTimeoutThreshold {
			s.onlineStations.Delete(stationID)
			log.Printf("⚠️ [Storage] 測站 [%s] 逾時 (%v) 未回應，自動剔除並標記離線", stationID, now.Sub(lastSeen).Round(time.Second))
		} else {
			list = append(list, stationID)
		}
		return true
	})
	return list
}

func (s *Storage) SaveMeasurementData(stationID string, ch1, ch2 *iclmodbus.ChannelMetrics) {
	now := time.Now()

	if ch1 != nil {
		log1 := convertToModel(stationID, 1, ch1, now)
		if err := s.db.Create(&log1).Error; err != nil {
			log.Printf("[%s DB] ❌ Ch1 寫入失敗: %v", stationID, err)
		} else {
			log.Printf("[%s DB] ✅ Ch1 已存檔 (厚度: %.1f µm, Eoff: %.3f V)", stationID, ch1.Thickness, ch1.Eoff)
		}
	}

	if ch2 != nil {
		log2 := convertToModel(stationID, 2, ch2, now)
		if err := s.db.Create(&log2).Error; err != nil {
			log.Printf("[%s DB] ❌ Ch2 寫入失敗: %v", stationID, err)
		} else {
			log.Printf("[%s DB] ✅ Ch2 已存檔 (厚度: %.1f µm, Eoff: %.3f V)", stationID, ch2.Thickness, ch2.Eoff)
		}
	}
}

func (s *Storage) GetAllLatestData() map[string]map[int]MeasurementLog {
	var stationIDs []string
	s.db.Model(&MeasurementLog{}).Distinct("station_id").Pluck("station_id", &stationIDs)

	result := make(map[string]map[int]MeasurementLog)
	for _, id := range stationIDs {
		result[id] = make(map[int]MeasurementLog)
		for ch := 1; ch <= 2; ch++ {
			var logData MeasurementLog
			if err := s.db.Where("station_id = ? AND channel = ?", id, ch).Order("created_at desc").First(&logData).Error; err == nil {
				result[id][ch] = logData
			}
		}
	}
	return result
}

func (s *Storage) GetHistory(stationID string) []MeasurementLog {
	var logs []MeasurementLog
	s.db.Where("station_id = ?", stationID).Order("created_at desc").Limit(100).Find(&logs)
	return logs
}

func convertToModel(stationID string, channel int, m *iclmodbus.ChannelMetrics, t time.Time) MeasurementLog {
	return MeasurementLog{
		StationID:   stationID,
		Channel:     channel,
		Thickness:   m.Thickness,
		Uac:         m.Uac,
		Iac:         m.Iac,
		Jac:         m.Jac,
		Rs:          m.Rs,
		Idc:         m.Idc,
		Jdc:         m.Jdc,
		Eon:         m.Eon,
		Eoff:        m.Eoff,
		EirFree:     m.EirFree,
		Temperature: m.Temperature,
		Rr:          m.Rr,
		Rc:          m.Rc,
		MetalLoss:   m.MetalLoss,
		CreatedAt:   t,
	}
}

func (s *Storage) TrySetMeasuring(stationID string, measuring bool) bool {
	if measuring {
		_, loaded := s.measuringMap.LoadOrStore(stationID, true)
		return !loaded
	}
	s.measuringMap.Delete(stationID)
	return true
}

func (s *Storage) GetLatestData(stationID string) (map[int]MeasurementLog, bool) {
	result := make(map[int]MeasurementLog)
	for ch := 1; ch <= 2; ch++ {
		var logData MeasurementLog
		if err := s.db.Where("station_id = ? AND channel = ?", stationID, ch).Order("created_at desc").First(&logData).Error; err == nil {
			result[ch] = logData
		}
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}