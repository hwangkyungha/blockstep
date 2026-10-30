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

// statusForm은 상태 일괄 저장 폼의 값이다. 스텝 하나 분량만 담는다.
func statusForm(stepID int64, status, pkg, by string) url.Values {
	key := itoa(stepID)
	return url.Values{"status_" + key: {status}, "checked_package_" + key: {pkg}, "checked_by_" + key: {by}}
}

// setStatus는 스텝 하나의 상태를 일괄 저장 폼으로 바꾼다.
func (e *env) setStatus(stepID int64, status, pkg, by string) *httptest.ResponseRecorder {
	e.t.Helper()
	st, err := e.store.GetStep(stepID)
	if err != nil {
		e.t.Fatal(err)
	}
	return e.do("POST", "/scenarios/"+itoa(st.ScenarioID)+"/steps/status", statusForm(stepID, status, pkg, by))
}

func TestCreateScenarioAndList(t *testing.T) {
	e := newEnv(t)
	rec := e.do("POST", "/scenarios", url.Values{"name": {"설치"}, "description": {"설치부터 라이선스 확인까지"}})
	e.mustRedirect(rec, "/scenarios/1")
	body := e.get("/")
	if !strings.Contains(body, `href="/scenarios/1"`) || !strings.Contains(body, "설치부터 라이선스 확인까지") {
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
	for _, want := range []string{"라이선스 확인\n  ? 라이선스 없음\n    평가판 안내", `<small>라이선스 없음 ›</small> 평가판 안내`, `id="status-form"`, `"name":"평가판 안내"`} {
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

func TestBulkStepStatus(t *testing.T) {
	e := newEnv(t)
	id := e.newScenario("설치", "하나\n둘\n셋\n")
	steps, _ := e.store.ScenarioSteps(id)
	one, two, three := steps[0], steps[1], steps[2]
	path := "/scenarios/" + itoa(id) + "/steps/status"

	// 한 줄이 어긋나면 전체를 저장하지 않고, 화면을 다시 그리며 입력 값과 줄별 오류를 보여 준다.
	form := statusForm(one.ID, "supported", "2026-10-29", "홍길동")
	for k, v := range statusForm(two.ID, "supported", "", "김엔진") {
		form[k] = v
	}
	rec := e.do("POST", path, form)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("응답 = %d, want 400", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{`id="step-row-` + itoa(two.ID) + `" class="has-error"`, "확인 패키지와 확인자를 적어야 합니다", `value="김엔진"`, `value="2026-10-29"`, `class="error-box"`} {
		if !strings.Contains(body, want) {
			t.Errorf("거부 화면에 %q가 없다", want)
		}
	}
	if st, _ := e.store.GetStep(one.ID); st.Status != "unchecked" {
		t.Errorf("거부되었는데 첫 줄이 저장되었다: %s", st.Status)
	}

	// 바뀐 줄만 저장하고, 바뀌지 않은 줄은 이력을 남기지 않는다.
	form = statusForm(one.ID, "supported", "2026-10-29", "홍길동")
	for k, v := range statusForm(two.ID, "not_implemented", "", "") {
		form[k] = v
	}
	for k, v := range statusForm(three.ID, "unchecked", "", "") {
		form[k] = v
	}
	e.mustRedirect(e.do("POST", path, form), "/scenarios/"+itoa(id)+"?tab=edit")
	a, _ := e.store.GetStep(one.ID)
	b, _ := e.store.GetStep(two.ID)
	if a.Status != "supported" || a.CheckedPackage != "2026-10-29" || b.Status != "not_implemented" {
		t.Errorf("스텝 = %+v %+v", a, b)
	}
	if logs, _ := e.store.ChangeLogs("step", three.ID); len(logs) != 1 {
		t.Errorf("바뀌지 않은 줄에 이력이 남았다: %+v", logs)
	}
	// 같은 값을 다시 저장해도 이력이 늘지 않는다.
	before, _ := e.store.ChangeLogs("step", one.ID)
	e.do("POST", path, statusForm(one.ID, "supported", "2026-10-29", "홍길동"))
	if after, _ := e.store.ChangeLogs("step", one.ID); len(after) != len(before) {
		t.Errorf("같은 값인데 이력이 늘었다: %d → %d", len(before), len(after))
	}

	// 미해결 이슈가 없는 스텝을 블록으로 고르면 나머지를 저장한 뒤 이슈 등록으로 보낸다.
	form = statusForm(three.ID, "blocked", "", "")
	for k, v := range statusForm(two.ID, "needs_test", "", "") {
		form[k] = v
	}
	e.mustRedirect(e.do("POST", path, form), "/issues/new?step_id="+itoa(three.ID)+"&then=block")
	b, _ = e.store.GetStep(two.ID)
	c, _ := e.store.GetStep(three.ID)
	if b.Status != "needs_test" || c.Status != "unchecked" {
		t.Errorf("나머지 줄 저장 = %s, 블록 줄 = %s", b.Status, c.Status)
	}
}

func TestScenarioPageHasMapAndListHasSummary(t *testing.T) {
	e := newEnv(t)
	id := e.newScenario("설치", "설치 실행\n라이선스 확인\n  ? 라이선스 없음\n    평가판 안내\n설정\n")
	steps, _ := e.store.ScenarioSteps(id)
	e.setStatus(steps[0].ID, "supported", "2026-10-29", "홍길동")

	body := e.get("/scenarios/" + itoa(id))
	for _, want := range []string{`<svg class="railmap"`, `data-step-id="` + itoa(steps[0].ID) + `"`, `>라이선스 없음</text>`} {
		if !strings.Contains(body, want) {
			t.Errorf("노선도에 %q가 없다", want)
		}
	}
	list := e.get("/")
	for _, want := range []string{`<th class="num">14일 지남</th>`, `title="지원 1"`, `title="미확인 3"`, `<td class="num">4</td>`} {
		if !strings.Contains(list, want) {
			t.Errorf("목록에 %q가 없다:\n%s", want, list)
		}
	}
}
