package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func fixedForm(stepID int64, title string) url.Values {
	f := issueForm(stepID)
	f.Set("title", title)
	f.Set("status", "fixed")
	f.Set("applied_package", "2026-10-30")
	f.Set("resolution_note", "새 바이너리 적용")
	return f
}

func TestSlackNotifications(t *testing.T) {
	var messages []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var m map[string]string
		json.Unmarshal(b, &m)
		messages = append(messages, m["text"])
	}))
	defer ts.Close()

	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n라이선스 확인\n")
	steps, _ := e.store.ScenarioSteps(scID)
	lic := steps[1]

	// 웹훅이 비어 있으면 보내지 않는다.
	e.do("POST", "/issues", issueForm(lic.ID))
	e.do("POST", "/steps/"+itoa(lic.ID)+"/status", url.Values{"status": {"blocked"}, "checked_by": {"홍길동"}})
	if len(messages) != 0 {
		t.Fatalf("웹훅 없이 보냈다: %v", messages)
	}

	e.mustRedirect(e.do("POST", "/settings", url.Values{"stale_days": {"14"}, "slack_webhook_url": {ts.URL}}), "/settings?saved=1")
	if page := e.get("/settings?saved=1"); !strings.Contains(page, ts.URL) || !strings.Contains(page, "저장했습니다") {
		t.Errorf("설정 화면:\n%s", page)
	}

	// 블록 스텝에 이슈가 새로 등록되면 알린다.
	form := issueForm(lic.ID)
	form.Set("title", "두 번째 이슈")
	form.Set("priority", "P2")
	e.mustRedirect(e.do("POST", "/issues", form), "/issues/2")
	if len(messages) != 1 || !strings.Contains(messages[0], "두 번째 이슈") || !strings.Contains(messages[0], "라이선스 확인") || !strings.Contains(messages[0], "P2") {
		t.Fatalf("등록 알림 = %v", messages)
	}
	// 블록이 아닌 스텝의 이슈는 알리지 않는다.
	e.do("POST", "/issues", issueForm(steps[0].ID))
	if len(messages) != 1 {
		t.Fatalf("블록 아닌 스텝 이슈를 알렸다: %v", messages)
	}

	// 조치 완료로 바뀌면 알린다. 두 이슈가 모두 해결되어야 스텝이 풀린다.
	e.mustRedirect(e.do("POST", "/issues/1", fixedForm(lic.ID, "첫 이슈")), "/issues/1")
	if len(messages) != 2 || !strings.Contains(messages[1], "테스트 필요로 바뀐 스텝: 없음") {
		t.Fatalf("첫 조치 완료 알림 = %v", messages)
	}
	e.mustRedirect(e.do("POST", "/issues/2", fixedForm(lic.ID, "두 번째 이슈")), "/issues/2")
	if len(messages) != 3 || !strings.Contains(messages[2], "테스트 필요로 바뀐 스텝: 라이선스 확인") || !strings.Contains(messages[2], "2026-10-30") || !strings.Contains(messages[2], "새 바이너리 적용") {
		t.Fatalf("둘째 조치 완료 알림 = %v", messages)
	}
	// 이미 조치 완료인 이슈를 다시 저장해도 알리지 않는다.
	e.do("POST", "/issues/2", fixedForm(lic.ID, "두 번째 이슈 제목 수정"))
	if len(messages) != 3 {
		t.Errorf("다시 저장했는데 알렸다: %v", messages)
	}
}

func TestSlackFailureIsLoggedNotFatal(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer ts.Close()
	e := newEnv(t)
	scID := e.newScenario("설치", "설치 실행\n")
	steps, _ := e.store.ScenarioSteps(scID)
	e.store.SetSetting("slack_webhook_url", ts.URL)
	e.do("POST", "/issues", issueForm(steps[0].ID))
	e.do("POST", "/steps/"+itoa(steps[0].ID)+"/status", url.Values{"status": {"blocked"}, "checked_by": {"홍길동"}})
	e.mustRedirect(e.do("POST", "/issues/1", fixedForm(steps[0].ID, "첫 이슈")), "/issues/1")
	issue, _ := e.store.GetIssue(1)
	if issue.Status != "fixed" {
		t.Errorf("전송 실패가 저장을 막았다: %s", issue.Status)
	}
	logs, _ := e.store.ChangeLogs("issue", 1)
	last := logs[len(logs)-1]
	if last.Field != "slack" || !strings.Contains(last.Note, "Slack 전송 실패") {
		t.Errorf("실패 이력이 없다: %+v", last)
	}
	if rec := e.do("POST", "/settings", url.Values{"stale_days": {"abc"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("잘못된 stale_days = %d", rec.Code)
	}
}
