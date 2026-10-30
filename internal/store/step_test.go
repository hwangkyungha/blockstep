package store

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hwangkyungha/blockstep/internal/outline"
)

const sampleOutline = `설치 실행
라이선스 확인 [install-SCN-03]
  ? 라이선스 없음
    평가판 안내
    평가판 모드 진입
설정
`

func mustParse(t *testing.T, text string) []outline.Step {
	t.Helper()
	steps, err := outline.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return steps
}

// insertIssue는 테스트용으로 이슈를 넣고 스텝에 연결한다.
func insertIssue(t *testing.T, s *Store, status string, stepIDs ...int64) int64 {
	t.Helper()
	res, err := s.db.Exec("INSERT INTO issue (title, reporter, status, priority, found_package) VALUES ('이슈', '홍길동', ?, 'P1', 'pkg')", status)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	for _, sid := range stepIDs {
		if _, err := s.db.Exec("INSERT INTO issue_step (issue_id, step_id) VALUES (?, ?)", id, sid); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func TestSaveOutlineBuildsTree(t *testing.T) {
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	if err := s.SaveOutline(scID, mustParse(t, sampleOutline), "홍길동"); err != nil {
		t.Fatal(err)
	}
	tree, err := s.ScenarioTree(scID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 3 || tree[1].Name != "라이선스 확인" || tree[1].Refs != "install-SCN-03" {
		t.Fatalf("본선 = %+v", tree)
	}
	if len(tree[1].Branches) != 1 || tree[1].Branches[0].Label != "라이선스 없음" || len(tree[1].Branches[0].Steps) != 2 {
		t.Fatalf("분기 = %+v", tree[1].Branches)
	}
	if tree[1].Branches[0].Steps[1].Name != "평가판 모드 진입" || tree[1].Branches[0].Steps[1].Status != StepUnchecked {
		t.Errorf("분기 스텝 = %+v", tree[1].Branches[0].Steps[1])
	}
	if got := outline.Format(ToOutline(tree)); got != sampleOutline {
		t.Errorf("되돌린 아웃라인:\n%s", got)
	}
}

func TestSaveOutlineKeepsStepsByName(t *testing.T) {
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	if err := s.SaveOutline(scID, mustParse(t, sampleOutline), "홍길동"); err != nil {
		t.Fatal(err)
	}
	before, _ := s.ScenarioSteps(scID)
	var setup, license Step
	for _, st := range before {
		switch st.Name {
		case "설정":
			setup = st
		case "라이선스 확인":
			license = st
		}
	}
	if _, err := s.SetStepStatus(setup.ID, StepSupported, "2026-10-29", "홍길동"); err != nil {
		t.Fatal(err)
	}
	issueID := insertIssue(t, s, IssueOpen, setup.ID, license.ID)

	// 라이선스 확인을 지우고, 설정을 분기 아래로 옮기고, 새 스텝을 넣는다.
	changed := "설치 실행\n새 스텝\n  ? 조건\n    설정\n"
	if err := s.SaveOutline(scID, mustParse(t, changed), "홍길동"); err != nil {
		t.Fatal(err)
	}
	after, _ := s.ScenarioSteps(scID)
	if len(after) != 3 {
		t.Fatalf("스텝 수 = %d, want 3: %+v", len(after), after)
	}
	for _, st := range after {
		if st.Name == "설정" {
			if st.ID != setup.ID || st.Status != StepSupported || st.CheckedPackage != "2026-10-29" || !st.ParentBranchID.Valid {
				t.Errorf("설정 스텝이 유지되지 않았다: %+v", st)
			}
		}
		if st.Name == "라이선스 확인" {
			t.Errorf("지운 스텝이 남아 있다")
		}
	}
	var linked int
	s.db.QueryRow("SELECT COUNT(*) FROM issue_step WHERE issue_id = ?", issueID).Scan(&linked)
	if linked != 1 {
		t.Errorf("이슈 연결 수 = %d, want 1 (지운 스텝의 연결만 해제)", linked)
	}
	logs, _ := s.ChangeLogs("step", license.ID)
	if len(logs) == 0 || logs[len(logs)-1].Note != "스텝 삭제" {
		t.Errorf("삭제 이력이 없다: %+v", logs)
	}
}

func TestSetStepStatusRules(t *testing.T) {
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	if err := s.SaveOutline(scID, mustParse(t, "하나\n둘\n"), "홍길동"); err != nil {
		t.Fatal(err)
	}
	steps, _ := s.ScenarioSteps(scID)
	one, two := steps[0], steps[1]

	if _, err := s.SetStepStatus(one.ID, StepSupported, "", "홍길동"); err == nil {
		t.Error("확인 패키지 없이 지원으로 바뀌었다")
	} else if _, ok := err.(ValidationError); !ok {
		t.Errorf("오류 종류 = %T", err)
	}
	if _, err := s.SetStepStatus(one.ID, StepBlocked, "", ""); err == nil {
		t.Error("미해결 이슈 없이 블록으로 바뀌었다")
	}
	if _, err := s.SetStepStatus(one.ID, "weird", "", ""); err == nil {
		t.Error("모르는 상태가 허용되었다")
	}

	insertIssue(t, s, IssueRejected, one.ID)
	if _, err := s.SetStepStatus(one.ID, StepBlocked, "", ""); err == nil {
		t.Error("거절된 이슈만 있는데 블록으로 바뀌었다")
	}
	issueID := insertIssue(t, s, IssueInProgress, one.ID, two.ID)
	if _, err := s.SetStepStatus(one.ID, StepBlocked, "", "홍길동"); err != nil {
		t.Fatal(err)
	}
	st, _ := s.GetStep(one.ID)
	if st.Status != StepBlocked {
		t.Errorf("status = %s", st.Status)
	}
	logs, _ := s.ChangeLogs("step", one.ID)
	last := logs[len(logs)-1]
	if last.Field != "status" || last.OldValue != StepUnchecked || last.NewValue != StepBlocked || last.Actor != "홍길동" {
		t.Errorf("이력 = %+v", last)
	}

	// 규칙 3: 조치 완료 이슈는 연결 스텝이 모두 지원이 될 때 검증 완료로 바뀐다.
	s.db.Exec("UPDATE issue SET status = 'fixed' WHERE id = ?", issueID)
	verified, err := s.SetStepStatus(one.ID, StepSupported, "2026-10-29", "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != 0 {
		t.Errorf("둘이 아직 지원이 아닌데 검증 완료로 바뀌었다: %v", verified)
	}
	verified, err = s.SetStepStatus(two.ID, StepSupported, "2026-10-29", "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	if len(verified) != 1 || verified[0] != issueID {
		t.Fatalf("검증 완료 목록 = %v", verified)
	}
	var status string
	s.db.QueryRow("SELECT status FROM issue WHERE id = ?", issueID).Scan(&status)
	if status != IssueVerified {
		t.Errorf("이슈 상태 = %s", status)
	}
	st, _ = s.GetStep(two.ID)
	if st.CheckedPackage != "2026-10-29" || st.CheckedBy != "홍길동" || !st.CheckedAt.Valid || st.CheckedTime().IsZero() {
		t.Errorf("확인 기록 = %+v", st)
	}
}

func TestScenarioStepsOrderedFollowsOutline(t *testing.T) {
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	text := "하나\n  ? 조건A\n    A1\n    A2\n  ? 조건B\n    B1\n둘\n  ? 조건C\n    C1\n셋\n"
	if err := s.SaveOutline(scID, mustParse(t, text), "홍길동"); err != nil {
		t.Fatal(err)
	}
	flat, err := s.ScenarioStepsOrdered(scID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range flat {
		got = append(got, fmt.Sprintf("%d:%s:%s", f.Depth, f.Branch, f.Name))
	}
	want := "0::하나,1:조건A:A1,1:조건A:A2,1:조건B:B1,0::둘,1:조건C:C1,0::셋"
	if strings.Join(got, ",") != want {
		t.Errorf("순서 = %v", got)
	}
}

func TestSetStepStatusesIsAllOrNothing(t *testing.T) {
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	if err := s.SaveOutline(scID, mustParse(t, "하나\n둘\n"), "홍길동"); err != nil {
		t.Fatal(err)
	}
	steps, _ := s.ScenarioSteps(scID)
	changes := []StepStatusChange{
		{StepID: steps[0].ID, Status: StepSupported, CheckedPackage: "2026-10-29", CheckedBy: "홍길동"},
		{StepID: steps[1].ID, Status: StepSupported}, // 확인 패키지 없음 → 거부
	}
	if _, err := s.SetStepStatuses(changes); err == nil {
		t.Fatal("한 줄이 어긋났는데 저장되었다")
	}
	st, _ := s.GetStep(steps[0].ID)
	if st.Status != StepUnchecked {
		t.Errorf("거부되었는데 첫 줄이 저장되었다: %s", st.Status)
	}
	changes[1] = StepStatusChange{StepID: steps[1].ID, Status: StepNotImplemented, Note: "계획만 있음"}
	if _, err := s.SetStepStatuses(changes); err != nil {
		t.Fatal(err)
	}
	a, _ := s.GetStep(steps[0].ID)
	b, _ := s.GetStep(steps[1].ID)
	if a.Status != StepSupported || b.Status != StepNotImplemented {
		t.Errorf("상태 = %s %s", a.Status, b.Status)
	}
	logs, _ := s.ChangeLogs("step", steps[1].ID)
	if last := logs[len(logs)-1]; last.Note != "계획만 있음" {
		t.Errorf("이력 사유 = %+v", last)
	}
	same := StepStatusChange{StepID: a.ID, Status: StepSupported, CheckedPackage: "2026-10-29", CheckedBy: "홍길동"}
	if same.Changed(a) {
		t.Error("같은 값인데 바뀐 것으로 본다")
	}
	same.CheckedBy = "김엔진"
	if !same.Changed(a) {
		t.Error("확인자가 바뀌었는데 같은 것으로 본다")
	}
}
