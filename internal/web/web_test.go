package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hwangkyungha/blockstep/internal/store"
)

type env struct {
	t     *testing.T
	store *store.Store
	h     http.Handler
}

func newEnv(t *testing.T) *env {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &env{t: t, store: s, h: Handler(s)}
}

func (e *env) do(method, path string, form url.Values) *httptest.ResponseRecorder {
	e.t.Helper()
	var req *http.Request
	if form == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, req)
	return rec
}

func (e *env) get(path string) string {
	e.t.Helper()
	rec := e.do("GET", path, nil)
	if rec.Code != http.StatusOK {
		e.t.Fatalf("GET %s = %d: %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

func (e *env) mustRedirect(rec *httptest.ResponseRecorder, to string) {
	e.t.Helper()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != to {
		e.t.Fatalf("응답 = %d %q, want 303 %s: %s", rec.Code, rec.Header().Get("Location"), to, rec.Body.String())
	}
}

// newScenario는 시나리오를 만들고 아웃라인을 저장한 뒤 id를 돌려준다.
func (e *env) newScenario(name, outline string) int64 {
	e.t.Helper()
	id, err := e.store.CreateScenario(name, "")
	if err != nil {
		e.t.Fatal(err)
	}
	if outline != "" {
		rec := e.do("PUT", "/scenarios/"+itoa(id)+"/outline", url.Values{"outline": {outline}, "actor": {"홍길동"}})
		if rec.Code != http.StatusNoContent {
			e.t.Fatalf("PUT outline = %d: %s", rec.Code, rec.Body.String())
		}
	}
	return id
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

func TestCreateScenarioAndList(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/scenarios", url.Values{"name": {"설치"}, "description": {"설치부터 라이선스 확인까지"}})
	e.mustRedirect(rec, "/scenarios/1")
	body := e.get("/")
	if !strings.Contains(body, `<a href="/scenarios/1">설치</a>`) || !strings.Contains(body, "설치부터 라이선스 확인까지") {
		t.Errorf("목록에 만든 시나리오가 없다:\n%s", body)
	}
}

func TestCreateScenarioRejectsEmptyName(t *testing.T) {
	e := newEnv(t)
	if rec := e.do("POST", "/scenarios", url.Values{"name": {" "}}); rec.Code != http.StatusBadRequest {
		t.Errorf("응답 = %d, want 400", rec.Code)
	}
}

func TestOutlineSaveAndPage(t *testing.T) {
	e := newEnv(t)
	id := e.newScenario("설치", "설치 실행\n라이선스 확인\n  ? 라이선스 없음\n    평가판 안내\n")
	body := e.get("/scenarios/" + itoa(id))
	for _, want := range []string{"라이선스 확인\n  ? 라이선스 없음\n    평가판 안내", "<td>평가판 안내</td>", `action="/steps/`} {
		if !strings.Contains(body, want) {
			t.Errorf("화면에 %q가 없다:\n%s", want, body)
		}
	}
	rec := e.do("PUT", "/scenarios/"+itoa(id)+"/outline", url.Values{"outline": {"? 조건\n  스텝"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "1번째 줄") {
		t.Errorf("잘못된 아웃라인 응답 = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do("GET", "/scenarios/999", nil); rec.Code != http.StatusNotFound {
		t.Errorf("없는 시나리오 = %d", rec.Code)
	}
}

func TestStepStatusForm(t *testing.T) {
	e := newEnv(t)
	id := e.newScenario("설치", "설치 실행\n")
	steps, _ := e.store.ScenarioSteps(id)
	path := "/steps/" + itoa(steps[0].ID) + "/status"

	rec := e.do("POST", path, url.Values{"status": {"supported"}})
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "확인 패키지") {
		t.Errorf("확인 패키지 없는 지원 = %d %s", rec.Code, rec.Body.String())
	}
	rec = e.do("POST", path, url.Values{"status": {"blocked"}, "checked_by": {"홍길동"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("이슈 없는 블록 = %d", rec.Code)
	}
	rec = e.do("POST", path, url.Values{"status": {"supported"}, "checked_package": {"2026-10-29"}, "checked_by": {"홍길동"}})
	e.mustRedirect(rec, "/scenarios/"+itoa(id))
	st, _ := e.store.GetStep(steps[0].ID)
	if st.Status != "supported" || st.CheckedPackage != "2026-10-29" {
		t.Errorf("스텝 = %+v", st)
	}
}

func TestScenarioPageHasMapAndListHasSummary(t *testing.T) {
	e := newEnv(t)
	id := e.newScenario("설치", "설치 실행\n라이선스 확인\n  ? 라이선스 없음\n    평가판 안내\n설정\n")
	steps, _ := e.store.ScenarioSteps(id)
	e.do("POST", "/steps/"+itoa(steps[0].ID)+"/status", url.Values{"status": {"supported"}, "checked_package": {"2026-10-29"}, "checked_by": {"홍길동"}})

	body := e.get("/scenarios/" + itoa(id))
	for _, want := range []string{`<svg class="railmap"`, `data-step-id="` + itoa(steps[0].ID) + `"`, `>라이선스 없음</text>`} {
		if !strings.Contains(body, want) {
			t.Errorf("노선도에 %q가 없다", want)
		}
	}
	list := e.get("/")
	for _, want := range []string{`<th>14일 지남</th>`, `title="지원 1"`, `title="미확인 3"`, `<td class="num">4</td>`} {
		if !strings.Contains(list, want) {
			t.Errorf("목록에 %q가 없다:\n%s", want, list)
		}
	}
}
