package main

import (
	"database/sql"
	"log"

	"Metricorr_ICL-M_SI/pkg/iclmodbus"

	_ "modernc.org/sqlite"
)

type Database struct {
	db *sql.DB
}

func NewDatabase(dbPath string) *Database {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		log.Fatalf("[DB] 無法開啟 SQLite 資料庫 %s: %v", dbPath, err)
	}

	createTableSQL := `
	CREATE TABLE IF NOT EXISTS measurements (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		station_id TEXT NOT NULL,
		timestamp DATETIME NOT NULL,
		ch1_thickness REAL,
		ch1_u_ac REAL,
		ch1_i_ac REAL,
		ch1_j_ac REAL,
		ch1_e_on REAL,
		ch1_e_off REAL,
		ch1_temp REAL,
		ch1_metal_loss REAL,
		ch2_thickness REAL,
		ch2_u_ac REAL,
		ch2_i_ac REAL,
		ch2_j_ac REAL,
		ch2_e_on REAL,
		ch2_e_off REAL,
		ch2_temp REAL,
		ch2_metal_loss REAL
	);
	CREATE INDEX IF NOT EXISTS idx_station_time ON measurements (station_id, timestamp);
	`
	if _, err := db.Exec(createTableSQL); err != nil {
		log.Fatalf("[DB] 初始化 Data Schema 失敗: %v", err)
	}

	log.Println("[DB] SQLite 資料庫連線初始化完成（WAL 高併發模式）: " + dbPath)
	return &Database{db: db}
}

func (d *Database) SaveMeasurement(data *iclmodbus.FullMeasurementData) error {
	query := `
	INSERT INTO measurements (
		station_id, timestamp,
		ch1_thickness, ch1_u_ac, ch1_i_ac, ch1_j_ac, ch1_e_on, ch1_e_off, ch1_temp, ch1_metal_loss,
		ch2_thickness, ch2_u_ac, ch2_i_ac, ch2_j_ac, ch2_e_on, ch2_e_off, ch2_temp, ch2_metal_loss
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);
	`
	_, err := d.db.Exec(query,
		data.StationID, data.Timestamp,
		data.Channel1.Thickness, data.Channel1.Uac, data.Channel1.Iac, data.Channel1.Jac, data.Channel1.Eon, data.Channel1.Eoff, data.Channel1.Temp, data.Channel1.MetalLoss,
		data.Channel2.Thickness, data.Channel2.Uac, data.Channel2.Iac, data.Channel2.Jac, data.Channel2.Eon, data.Channel2.Eoff, data.Channel2.Temp, data.Channel2.MetalLoss,
	)
	return err
}

func (d *Database) GetHistory(stationID string, limit int) ([]iclmodbus.FullMeasurementData, error) {
	query := `
	SELECT station_id, timestamp,
		ch1_thickness, ch1_u_ac, ch1_i_ac, ch1_j_ac, ch1_e_on, ch1_e_off, ch1_temp, ch1_metal_loss,
		ch2_thickness, ch2_u_ac, ch2_i_ac, ch2_j_ac, ch2_e_on, ch2_e_off, ch2_temp, ch2_metal_loss
	FROM measurements WHERE station_id = ? ORDER BY id DESC LIMIT ?;
	`
	rows, err := d.db.Query(query, stationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// 確保即使 0 筆資料也回傳 [] 而非 nil (避免 JSON marshal 為 null)
	list := make([]iclmodbus.FullMeasurementData, 0)
	for rows.Next() {
		var m iclmodbus.FullMeasurementData
		err := rows.Scan(
			&m.StationID, &m.Timestamp,
			&m.Channel1.Thickness, &m.Channel1.Uac, &m.Channel1.Iac, &m.Channel1.Jac, &m.Channel1.Eon, &m.Channel1.Eoff, &m.Channel1.Temp, &m.Channel1.MetalLoss,
			&m.Channel2.Thickness, &m.Channel2.Uac, &m.Channel2.Iac, &m.Channel2.Jac, &m.Channel2.Eon, &m.Channel2.Eoff, &m.Channel2.Temp, &m.Channel2.MetalLoss,
		)
		if err != nil {
			return nil, err
		}
		list = append(list, m)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}