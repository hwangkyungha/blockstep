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

	rec := e.do("POST", "/issues", url.Values{"title": {"입력 유지 확인"}, "step_ids": {itoa(lic.ID)}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("필수 칸 없는 등록 = %d", rec.Code)
	}
	for _, want := range []string{`class="error-box"`, "발견 패키지를 적어야 합니다", `value="입력 유지 확인"`, `value="` + itoa(lic.ID) + `" checked`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("거부된 등록 화면에 %q가 없다", want)
		}
	}
	rec = e.do("POST", "/issues", issueForm(lic.ID))
	e.mustRedirect(rec, "/issues/1")

	detail := e.get("/issues/1")
	for _, want := range []string{"라이선스 서버 오류", `>라이선스 확인</a>`, "설치 · 미확인", "이슈 등록", `<span class="badge red">P1</span>`, "조치 완료로 바꾸기"} {
		if !strings.Contains(detail, want) {
			t.Errorf("상세에 %q가 없다", want)
		}
	}
	edit := e.get("/issues/1/edit")
	if !strings.Contains(edit, `value="홍길동"`) || !strings.Contains(edit, `value="`+itoa(lic.ID)+`" checked`) {
		t.Errorf("편집 화면에 값이 없다")
	}

	// 블록 스텝으로 바꾸면 목록과 상세에 블록 표시가 붙고, 스텝 필터가 칩으로 보인다.
	e.setStatus(lic.ID, "blocked", "", "홍길동")
	list := e.get("/issues?step=" + itoa(lic.ID))
	for _, want := range []string{`background: var(--blocked)`, `스텝: 라이선스 확인 <a href="/issues"`, `/issues/new?step=` + itoa(lic.ID)} {
		if !strings.Contains(list, want) {
			t.Errorf("목록에 %q가 없다", want)
		}
	}
	if !strings.Contains(e.get("/issues/1"), `<span class="pill red">블록</span>`) {
		t.Errorf("상세에 블록 표시가 없다")
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
	e.setStatus(lic.ID, "blocked", "", "홍길동")

	form := issueForm(lic.ID)
	form.Set("status", "fixed")
	form.Set("actor", "김엔진")
	if rec := e.do("POST", "/issues/1", form); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `class="error-box"`) || !strings.Contains(rec.Body.String(), `action="/issues/1"`) {
		t.Errorf("반영 패키지 없는 조치 완료 = %d, 편집 화면에 오류 상자가 있어야 한다", rec.Code)
	}
	form.Set("applied_package", "2026-10-30")
	form.Set("resolution_note", "새 바이너리 적용")
	e.mustRedirect(e.do("POST", "/issues/1", form), "/issues/1")
	st, _ := e.store.GetStep(lic.ID)
	if st.Status != store.StepNeedsTest {
		t.Errorf("스텝 상태 = %s, want needs_test", st.Status)
	}

	e.mustRedirect(e.do("POST", "/issues/1/notes", url.Values{"author": {"김엔진"}, "body": {"원인 파악 중"}}), "/issues/1")
	if rec := e.do("POST", "/issues/1/notes", url.Values{"author": {"김엔진"}}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "작성자와 내용을 적어야 합니다") || !strings.Contains(rec.Body.String(), `value="김엔진"`) {
		t.Errorf("빈 기록 = %d, 상세 화면의 기록 폼 위에 오류가 있어야 한다", rec.Code)
	}
	detail := e.get("/issues/1")
	for _, want := range []string{"원인 파악 중", "김엔진 · <b>status</b> open → fixed", `id="combo-people">["김엔진","홍길동"]</script>`} {
		if !strings.Contains(detail, want) {
			t.Errorf("상세에 %q가 없다:\n%s", want, detail)
		}
	}
	if strings.Contains(detail, "조치 완료로 바꾸기") {
		t.Errorf("이미 조치 완료인데 조치 완료 폼이 보인다")
	}
	if rec := e.do("GET", "/issues/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("없는 이슈 = %d", rec.Code)
	}
}

func TestIssueFixAndStatusForms(t *testing.T) {
	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n라이선스 확인\n")
	steps, _ := e.store.ScenarioSteps(scID)
	lic := steps[1]
	e.do("POST", "/issues", issueForm(lic.ID))
	second := issueForm(lic.ID)
	second.Set("title", "두 번째")
	e.do("POST", "/issues", second)
	e.setStatus(lic.ID, "blocked", "", "홍길동")

	detail := e.get("/issues/1")
	if !strings.Contains(detail, "미해결 이슈 #2가 남아 블록이 유지됩니다") {
		t.Errorf("조치 완료 안내가 없다")
	}

	// 상태 바꾸기: 조치 완료는 거부, 그 밖의 상태는 바뀐다.
	if rec := e.do("POST", "/issues/1/status", url.Values{"status": {"fixed"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("상태 폼으로 조치 완료 = %d", rec.Code)
	}
	e.mustRedirect(e.do("POST", "/issues/1/status", url.Values{"status": {"in_progress"}, "actor": {"김엔진"}}), "/issues/1")
	issue, _ := e.store.GetIssue(1)
	if issue.Status != store.IssueInProgress || len(issue.Steps) != 1 {
		t.Errorf("이슈 = %+v", issue)
	}

	// 조치 완료 폼: 두 칸이 비면 거부, 적으면 조치 완료가 되고 다른 칸은 유지된다.
	if rec := e.do("POST", "/issues/1/fix", url.Values{"applied_package": {"2026-10-30"}}); rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "반영 패키지와 처리 후 내용을 적어야 합니다") || !strings.Contains(rec.Body.String(), `name="applied_package" data-combo="packages" autocomplete="off" value="2026-10-30"`) {
		t.Errorf("처리 후 내용 없는 조치 완료 = %d, 조치 완료 폼 위에 오류와 입력 값이 있어야 한다", rec.Code)
	}
	e.mustRedirect(e.do("POST", "/issues/1/fix", url.Values{"applied_package": {"2026-10-30"}, "resolution_note": {"새 바이너리"}, "jira_key": {"CT2612-405"}}), "/issues/1")
	issue, _ = e.store.GetIssue(1)
	if issue.Status != store.IssueFixed || issue.Title != "라이선스 서버 오류" || issue.JiraKey != "CT2612-405" || issue.Priority != "P1" {
		t.Errorf("이슈 = %+v", issue)
	}
	st, _ := e.store.GetStep(lic.ID)
	if st.Status != store.StepBlocked {
		t.Errorf("다른 미해결 이슈가 남았는데 블록이 풀렸다")
	}
	if !strings.Contains(e.get("/issues/2"), "미해결 이슈가 0이 되어 테스트 필요로 바뀌고") {
		t.Errorf("마지막 이슈의 안내가 없다")
	}
}

func TestIssueCreateThenBlock(t *testing.T) {
	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n라이선스 확인\n")
	steps, _ := e.store.ScenarioSteps(scID)
	lic := steps[1]

	page := e.get("/issues/new?step_id=" + itoa(lic.ID) + "&then=block")
	for _, want := range []string{"스텝 「라이선스 확인」이 블록으로 바뀝니다", `name="then" value="block"`, `name="step_id" value="` + itoa(lic.ID) + `"`, `value="` + itoa(lic.ID) + `" checked`} {
		if !strings.Contains(page, want) {
			t.Errorf("폼에 %q가 없다", want)
		}
	}
	form := issueForm(lic.ID)
	form.Set("then", "block")
	form.Set("step_id", itoa(lic.ID))
	e.mustRedirect(e.do("POST", "/issues", form), "/scenarios/"+itoa(scID)+"?tab=edit&step="+itoa(lic.ID))
	st, _ := e.store.GetStep(lic.ID)
	if st.Status != store.StepBlocked {
		t.Fatalf("스텝 상태 = %s, want blocked", st.Status)
	}
	logs, _ := e.store.ChangeLogs("step", lic.ID)
	if last := logs[len(logs)-1]; last.Note != "이슈 #1 등록으로 블록" || last.NewValue != store.StepBlocked {
		t.Errorf("이력 = %+v", last)
	}
	// 블록할 스텝을 연결에서 빼면 거부한다.
	form = issueForm(steps[0].ID)
	form.Set("then", "block")
	form.Set("step_id", itoa(lic.ID))
	if rec := e.do("POST", "/issues", form); rec.Code != http.StatusBadRequest {
		t.Errorf("연결 안 된 스텝 블록 = %d", rec.Code)
	}
}
