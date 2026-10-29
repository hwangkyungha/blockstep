package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hwangkyungha/blockstep/internal/store"
)

func TestCreateScenarioAndList(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	h := Handler(s)

	form := url.Values{"name": {"설치"}, "description": {"설치부터 라이선스 확인까지"}}
	req := httptest.NewRequest("POST", "/scenarios", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/" {
		t.Fatalf("POST 응답 = %d %q", rec.Code, rec.Header().Get("Location"))
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET 응답 = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<td>설치</td>") || !strings.Contains(body, "설치부터 라이선스 확인까지") {
		t.Errorf("목록에 만든 시나리오가 없다:\n%s", body)
	}
}

func TestCreateScenarioRejectsEmptyName(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	req := httptest.NewRequest("POST", "/scenarios", strings.NewReader("name=+"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	Handler(s).ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("응답 = %d, want 400", rec.Code)
	}
}
