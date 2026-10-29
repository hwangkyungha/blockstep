// Package store는 SQLite 스키마 적용과 질의를 맡는다.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

// Store는 SQLite 파일 하나에 대한 연결이다.
type Store struct {
	db *sql.DB
}

// Open은 SQLite 파일을 열고 스키마를 적용한다. 파일이 없으면 만든다.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("foreign_keys 설정 실패: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("스키마 적용 실패: %w", err)
	}
	return &Store{db: db}, nil
}

// Close는 연결을 닫는다.
func (s *Store) Close() error {
	return s.db.Close()
}

// Scenario는 scenario 테이블의 한 줄이다.
type Scenario struct {
	ID          int64
	Name        string
	Description string
	SortOrder   int
}

// ListScenarios는 시나리오를 sort_order, id 순으로 돌려준다.
func (s *Store) ListScenarios() ([]Scenario, error) {
	rows, err := s.db.Query("SELECT id, name, description, sort_order FROM scenario ORDER BY sort_order, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Scenario
	for rows.Next() {
		var sc Scenario
		if err := rows.Scan(&sc.ID, &sc.Name, &sc.Description, &sc.SortOrder); err != nil {
			return nil, err
		}
		list = append(list, sc)
	}
	return list, rows.Err()
}

// CreateScenario는 시나리오를 목록 끝에 추가하고 id를 돌려준다.
func (s *Store) CreateScenario(name, description string) (int64, error) {
	res, err := s.db.Exec(
		"INSERT INTO scenario (name, description, sort_order) VALUES (?, ?, (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM scenario))",
		name, description)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SetSetting은 설정 값을 저장한다.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec("INSERT INTO setting (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value", key, value)
	return err
}

// LogNotifyFailure는 Slack 전송 실패를 이슈의 변경 이력에 남긴다.
func (s *Store) LogNotifyFailure(issueID int64, err error) error {
	return logChange(s.db, "issue", issueID, "slack", "", "", "", "Slack 전송 실패: "+err.Error())
}

// Setting은 setting 테이블의 값을 돌려준다. 없으면 빈 문자열이다.
func (s *Store) Setting(key string) (string, error) {
	var v string
	err := s.db.QueryRow("SELECT value FROM setting WHERE key = ?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}
