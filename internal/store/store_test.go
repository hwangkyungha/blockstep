package store

import (
	"path/filepath"
	"testing"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenAppliesSchema(t *testing.T) {
	s := openTemp(t)
	want := []string{"scenario", "step", "branch", "issue", "issue_step", "issue_note", "change_log", "person", "setting"}
	for _, name := range want {
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?", name).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("테이블 %s가 없다", name)
		}
	}
	v, err := s.Setting("stale_days")
	if err != nil {
		t.Fatal(err)
	}
	if v != "14" {
		t.Errorf("stale_days = %q, want 14", v)
	}
}

func TestOpenTwiceKeepsData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateScenario("설치", ""); err != nil {
		t.Fatal(err)
	}
	s.Close()

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.ListScenarios()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "설치" {
		t.Errorf("두 번째 열기 뒤 목록 = %+v", list)
	}
}

func TestCreateScenario(t *testing.T) {
	s := openTemp(t)
	id1, err := s.CreateScenario("설치", "설치부터 라이선스 확인까지")
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.CreateScenario("분석", "")
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatalf("id가 같다: %d", id1)
	}
	list, err := s.ListScenarios()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}
	if list[0].Name != "설치" || list[0].Description != "설치부터 라이선스 확인까지" || list[0].SortOrder != 1 {
		t.Errorf("첫 줄 = %+v", list[0])
	}
	if list[1].Name != "분석" || list[1].SortOrder != 2 {
		t.Errorf("둘째 줄 = %+v", list[1])
	}
}
