package store

import (
	"strings"
	"testing"
)

func setupIssueEnv(t *testing.T) (*Store, []Step) {
	t.Helper()
	s := openTemp(t)
	scID, _ := s.CreateScenario("설치", "")
	if err := s.SaveOutline(scID, mustParse(t, "설치 실행\n라이선스 확인\n설정\n"), "홍길동"); err != nil {
		t.Fatal(err)
	}
	steps, _ := s.ScenarioSteps(scID)
	return s, steps
}

func baseInput(stepIDs ...int64) IssueInput {
	return IssueInput{Title: "라이선스 서버 오류", Description: "재현: lct install", Reporter: "홍길동",
		Priority: "P1", FoundPackage: "2026-10-28", StepIDs: stepIDs}
}

func TestCreateIssueValidation(t *testing.T) {
	s, steps := setupIssueEnv(t)
	cases := map[string]IssueInput{
		"제목":     {Description: "d", Reporter: "r", Priority: "P1", FoundPackage: "p", StepIDs: []int64{steps[0].ID}},
		"연결 스텝":  {Title: "t", Description: "d", Reporter: "r", Priority: "P1", FoundPackage: "p"},
		"발견 패키지": {Title: "t", Description: "d", Reporter: "r", Priority: "P1", StepIDs: []int64{steps[0].ID}},
		"리포터":    {Title: "t", Description: "d", Priority: "P1", FoundPackage: "p", StepIDs: []int64{steps[0].ID}},
		"긴급도":    {Title: "t", Description: "d", Reporter: "r", Priority: "P9", FoundPackage: "p", StepIDs: []int64{steps[0].ID}},
		"반영 패키지": {Title: "t", Description: "d", Reporter: "r", Priority: "P1", FoundPackage: "p", Status: IssueFixed, StepIDs: []int64{steps[0].ID}},
	}
	for want, in := range cases {
		_, _, err := s.CreateIssue(in, "홍길동")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v", want, err)
		}
	}
	if _, _, err := s.CreateIssue(baseInput(999), "홍길동"); err == nil {
		t.Error("없는 스텝에 연결되었다")
	}
}

func TestCreateIssueAssignsAndLinks(t *testing.T) {
	s, steps := setupIssueEnv(t)
	in := baseInput(steps[1].ID, steps[2].ID)
	in.Assignee = "김엔진"
	in.DueDate = "2026-11-05"
	id, res, err := s.CreateIssue(in, "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.BlockedSteps) != 0 {
		t.Errorf("블록 스텝이 없는데 %v", res.BlockedSteps)
	}
	got, err := s.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != IssueAssigned {
		t.Errorf("담당자가 있는데 상태 = %s", got.Status)
	}
	if len(got.Steps) != 2 || got.Steps[0].StepName != "라이선스 확인" || got.Steps[0].ScenarioName != "설치" {
		t.Errorf("연결 스텝 = %+v", got.Steps)
	}
	if got.DueDate != "2026-11-05" || got.Blocked {
		t.Errorf("이슈 = %+v", got)
	}
	people, _ := s.People()
	if strings.Join(people, ",") != "김엔진,홍길동" {
		t.Errorf("사람 목록 = %v", people)
	}
	pkgs, _ := s.Packages()
	if len(pkgs) != 1 || pkgs[0] != "2026-10-28" {
		t.Errorf("패키지 목록 = %v", pkgs)
	}

	// 블록 스텝에 새 이슈를 연결하면 알림 대상 스텝으로 돌려준다.
	if _, err := s.SetStepStatus(steps[1].ID, StepBlocked, "", "홍길동"); err != nil {
		t.Fatal(err)
	}
	_, res, err = s.CreateIssue(baseInput(steps[1].ID), "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.BlockedSteps) != 1 || res.BlockedSteps[0].ID != steps[1].ID {
		t.Errorf("블록 스텝 = %+v", res.BlockedSteps)
	}
	got, _ = s.GetIssue(id)
	if !got.Blocked {
		t.Error("연결 스텝이 블록인데 Blocked가 false다")
	}
}

func TestUpdateIssueLogsAndReleasesBlockedStep(t *testing.T) {
	s, steps := setupIssueEnv(t)
	id, _, err := s.CreateIssue(baseInput(steps[1].ID), "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetStepStatus(steps[1].ID, StepBlocked, "", "홍길동"); err != nil {
		t.Fatal(err)
	}
	in := baseInput(steps[1].ID)
	in.Assignee = "김엔진"
	in.Status = IssueOpen
	res, err := s.UpdateIssue(id, in, "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetIssue(id)
	if got.Status != IssueAssigned || len(res.NeedsTest) != 0 {
		t.Errorf("상태 = %s, 자동 전환 = %v", got.Status, res.NeedsTest)
	}

	in.Status = IssueFixed
	if _, err := s.UpdateIssue(id, in, "홍길동"); err == nil {
		t.Error("반영 패키지 없이 조치 완료가 되었다")
	}
	in.AppliedPackage = "2026-10-30"
	in.ResolutionNote = "새 바이너리 적용"
	res, err = s.UpdateIssue(id, in, "김엔진")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.NeedsTest) != 1 || res.NeedsTest[0].ID != steps[1].ID {
		t.Fatalf("테스트 필요 전환 = %+v", res.NeedsTest)
	}
	st, _ := s.GetStep(steps[1].ID)
	if st.Status != StepNeedsTest {
		t.Errorf("스텝 상태 = %s", st.Status)
	}
	stepLogs, _ := s.ChangeLogs("step", steps[1].ID)
	last := stepLogs[len(stepLogs)-1]
	if last.Note != "이슈 #1 조치 완료로 자동 전환" || last.OldValue != StepBlocked || last.NewValue != StepNeedsTest {
		t.Errorf("스텝 이력 = %+v", last)
	}
	logs, _ := s.ChangeLogs("issue", id)
	fields := map[string]bool{}
	for _, l := range logs {
		fields[l.Field] = true
	}
	for _, f := range []string{"created", "assignee", "status", "applied_package", "resolution_note"} {
		if !fields[f] {
			t.Errorf("이력에 %s가 없다: %+v", f, logs)
		}
	}
}

func TestUpdateIssueKeepsBlockWhileOthersOpen(t *testing.T) {
	s, steps := setupIssueEnv(t)
	id1, _, _ := s.CreateIssue(baseInput(steps[1].ID), "홍길동")
	s.CreateIssue(baseInput(steps[1].ID), "홍길동")
	s.SetStepStatus(steps[1].ID, StepBlocked, "", "홍길동")
	in := baseInput(steps[1].ID)
	in.Status = IssueRejected
	res, err := s.UpdateIssue(id1, in, "홍길동")
	if err != nil {
		t.Fatal(err)
	}
	st, _ := s.GetStep(steps[1].ID)
	if st.Status != StepBlocked || len(res.NeedsTest) != 0 {
		t.Errorf("다른 미해결 이슈가 있는데 블록이 풀렸다: %s %v", st.Status, res.NeedsTest)
	}
}

func TestListIssuesSortAndFilter(t *testing.T) {
	s, steps := setupIssueEnv(t)
	mk := func(title, priority, due string, stepID int64, assignee string) int64 {
		in := baseInput(stepID)
		in.Title, in.Priority, in.DueDate, in.Assignee = title, priority, due, assignee
		id, _, err := s.CreateIssue(in, "홍길동")
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	mk("P2 기한 없음", "P2", "", steps[0].ID, "")
	mk("P1 늦은 기한", "P1", "2026-11-20", steps[0].ID, "김엔진")
	mk("P1 이른 기한", "P1", "2026-11-01", steps[2].ID, "")
	mk("P3 블록", "P3", "", steps[1].ID, "")
	mk("P1 블록", "P1", "", steps[1].ID, "김엔진")
	if _, err := s.SetStepStatus(steps[1].ID, StepBlocked, "", "홍길동"); err != nil {
		t.Fatal(err)
	}

	list, err := s.ListIssues(IssueFilter{})
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, i := range list {
		order = append(order, i.Title)
	}
	want := "P1 블록,P3 블록,P1 이른 기한,P1 늦은 기한,P2 기한 없음"
	if strings.Join(order, ",") != want {
		t.Errorf("정렬 = %v", order)
	}

	check := func(f IssueFilter, want ...string) {
		t.Helper()
		list, err := s.ListIssues(f)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, i := range list {
			got = append(got, i.Title)
		}
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("필터 %+v = %v, want %v", f, got, want)
		}
	}
	check(IssueFilter{Blocked: "1"}, "P1 블록", "P3 블록")
	check(IssueFilter{Blocked: "0"}, "P1 이른 기한", "P1 늦은 기한", "P2 기한 없음")
	check(IssueFilter{Assignee: "김엔진"}, "P1 블록", "P1 늦은 기한")
	check(IssueFilter{Status: IssueAssigned}, "P1 블록", "P1 늦은 기한")
	check(IssueFilter{StepID: steps[2].ID}, "P1 이른 기한")
	check(IssueFilter{ScenarioID: steps[0].ScenarioID, Status: IssueOpen}, "P3 블록", "P1 이른 기한", "P2 기한 없음")
}

func TestNotes(t *testing.T) {
	s, steps := setupIssueEnv(t)
	id, _, _ := s.CreateIssue(baseInput(steps[0].ID), "홍길동")
	if err := s.AddNote(id, "김엔진", ""); err == nil {
		t.Error("빈 기록이 저장되었다")
	}
	if err := s.AddNote(id, "김엔진", "원인 파악 중"); err != nil {
		t.Fatal(err)
	}
	notes, _ := s.Notes(id)
	if len(notes) != 1 || notes[0].Author != "김엔진" || notes[0].Body != "원인 파악 중" || notes[0].CreatedAt == "" {
		t.Errorf("기록 = %+v", notes)
	}
	logs, _ := s.ChangeLogs("issue", id)
	if logs[len(logs)-1].Field != "note" {
		t.Errorf("기록 이력이 없다: %+v", logs)
	}
}
