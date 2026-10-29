package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/hwangkyungha/blockstep/internal/store"
)

func issueForm(stepID int64) url.Values {
	return url.Values{"title": {"라이선스 서버 오류"}, "description": {"재현: lct install"}, "reporter": {"홍길동"},
		"priority": {"P1"}, "found_package": {"2026-10-28"}, "status": {"open"}, "step_ids": {itoa(stepID)}}
}

func TestIssueCreateListDetail(t *testing.T) {
	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n라이선스 확인\n")
	steps, _ := e.store.ScenarioSteps(scID)
	lic := steps[1]

	page := e.get("/issues/new?step=" + itoa(lic.ID))
	if !strings.Contains(page, `value="`+itoa(lic.ID)+`" checked`) {
		t.Errorf("스텝이 미리 선택되지 않았다")
	}

	rec := e.do("POST", "/issues", url.Values{"title": {"x"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("필수 칸 없는 등록 = %d", rec.Code)
	}
	rec = e.do("POST", "/issues", issueForm(lic.ID))
	e.mustRedirect(rec, "/issues/1")

	detail := e.get("/issues/1")
	for _, want := range []string{"라이선스 서버 오류", "설치</a> › 라이선스 확인", "이슈 등록", `value="홍길동"`} {
		if !strings.Contains(detail, want) {
			t.Errorf("상세에 %q가 없다", want)
		}
	}

	// 블록 스텝으로 바꾸면 목록과 상세에 블록 배지가 붙고, 스텝 필터가 동작한다.
	e.do("POST", "/steps/"+itoa(lic.ID)+"/status", url.Values{"status": {"blocked"}, "checked_by": {"홍길동"}})
	list := e.get("/issues?step=" + itoa(lic.ID))
	if !strings.Contains(list, `<span class="badge blocked">블록</span>`) || !strings.Contains(list, "스텝 「라이선스 확인」의 이슈") {
		t.Errorf("목록:\n%s", list)
	}
	if other := e.get("/issues?step=" + itoa(steps[0].ID)); strings.Contains(other, "라이선스 서버 오류") {
		t.Errorf("다른 스텝 필터에 이슈가 보인다")
	}
	if filtered := e.get("/issues?blocked=0"); strings.Contains(filtered, "라이선스 서버 오류") {
		t.Errorf("블록 아님 필터에 블록 이슈가 보인다")
	}
}

func TestIssueUpdateReleasesStepAndNotes(t *testing.T) {
	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n라이선스 확인\n")
	steps, _ := e.store.ScenarioSteps(scID)
	lic := steps[1]
	e.do("POST", "/issues", issueForm(lic.ID))
	e.do("POST", "/steps/"+itoa(lic.ID)+"/status", url.Values{"status": {"blocked"}, "checked_by": {"홍길동"}})

	form := issueForm(lic.ID)
	form.Set("status", "fixed")
	form.Set("actor", "김엔진")
	if rec := e.do("POST", "/issues/1", form); rec.Code != http.StatusBadRequest {
		t.Errorf("반영 패키지 없는 조치 완료 = %d", rec.Code)
	}
	form.Set("applied_package", "2026-10-30")
	form.Set("resolution_note", "새 바이너리 적용")
	e.mustRedirect(e.do("POST", "/issues/1", form), "/issues/1")
	st, _ := e.store.GetStep(lic.ID)
	if st.Status != store.StepNeedsTest {
		t.Errorf("스텝 상태 = %s, want needs_test", st.Status)
	}

	e.mustRedirect(e.do("POST", "/issues/1/notes", url.Values{"author": {"김엔진"}, "body": {"원인 파악 중"}}), "/issues/1")
	if rec := e.do("POST", "/issues/1/notes", url.Values{"author": {"김엔진"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("빈 기록 = %d", rec.Code)
	}
	detail := e.get("/issues/1")
	for _, want := range []string{"원인 파악 중", "<td>status</td><td>open</td><td>fixed</td><td>김엔진</td>", `<option value="김엔진">`} {
		if !strings.Contains(detail, want) {
			t.Errorf("상세에 %q가 없다:\n%s", want, detail)
		}
	}
	if rec := e.do("GET", "/issues/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("없는 이슈 = %d", rec.Code)
	}
}
