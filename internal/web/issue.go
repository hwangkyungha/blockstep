package web

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/hwangkyungha/blockstep/internal/railmap"
	"github.com/hwangkyungha/blockstep/internal/store"
)

// scenarioSteps는 이슈 폼에서 연결 스텝을 시나리오별로 고르기 위한 묶음이다.
type scenarioSteps struct {
	store.Scenario
	Steps []store.FlatStep
}

// formData는 이슈 폼에 공통으로 필요한 선택지다.
func (srv *server) formData() (map[string]any, error) {
	scenarios, err := srv.store.ListScenarios()
	if err != nil {
		return nil, err
	}
	people, err := srv.store.People()
	if err != nil {
		return nil, err
	}
	packages, err := srv.store.Packages()
	if err != nil {
		return nil, err
	}
	groups := make([]*scenarioSteps, len(scenarios))
	for i, sc := range scenarios {
		steps, err := srv.store.ScenarioStepsOrdered(sc.ID)
		if err != nil {
			return nil, err
		}
		groups[i] = &scenarioSteps{Scenario: sc, Steps: steps}
	}
	return map[string]any{
		"Groups":        groups,
		"Scenarios":     scenarios,
		"People":        people,
		"Packages":      packages,
		"IssueStatuses": store.IssueStatuses,
		"Priorities":    store.Priorities,
		"Colors":        railmap.Color,
	}, nil
}

func parseIssueInput(r *http.Request) store.IssueInput {
	in := store.IssueInput{
		Title:          r.FormValue("title"),
		Description:    r.FormValue("description"),
		Reporter:       r.FormValue("reporter"),
		Assignee:       r.FormValue("assignee"),
		Status:         r.FormValue("status"),
		Priority:       r.FormValue("priority"),
		FoundPackage:   r.FormValue("found_package"),
		AppliedPackage: r.FormValue("applied_package"),
		DueDate:        r.FormValue("due_date"),
		ResolutionNote: r.FormValue("resolution_note"),
		JiraKey:        r.FormValue("jira_key"),
	}
	for _, v := range r.Form["step_ids"] {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			in.StepIDs = append(in.StepIDs, id)
		}
	}
	return in
}

// inputFrom은 저장된 이슈를 폼 입력 값으로 되돌린다. 일부 칸만 바꾸는 폼이 쓴다.
func inputFrom(i store.Issue) store.IssueInput {
	in := store.IssueInput{Title: i.Title, Description: i.Description, Reporter: i.Reporter, Assignee: i.Assignee,
		Status: i.Status, Priority: i.Priority, FoundPackage: i.FoundPackage, AppliedPackage: i.AppliedPackage,
		DueDate: i.DueDate, ResolutionNote: i.ResolutionNote, JiraKey: i.JiraKey}
	for _, st := range i.Steps {
		in.StepIDs = append(in.StepIDs, st.StepID)
	}
	return in
}

// issueRow는 목록 한 줄이다. DueSoon은 목표 기한이 지났거나 3일 안이고 아직 해결되지 않았을 때다.
type issueRow struct {
	store.Issue
	DueSoon bool
}

// chip은 걸려 있는 필터 하나와 그것만 푼 주소다.
type chip struct {
	Label  string
	Remove string
}

func (srv *server) issueList(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := store.IssueFilter{Status: q.Get("status"), Assignee: q.Get("assignee"), Blocked: q.Get("blocked")}
	f.ScenarioID, _ = strconv.ParseInt(q.Get("scenario"), 10, 64)
	f.StepID, _ = strconv.ParseInt(q.Get("step"), 10, 64)
	list, err := srv.store.ListIssues(f)
	if err != nil {
		fail(w, err)
		return
	}
	all, err := srv.store.ListIssues(store.IssueFilter{})
	if err != nil {
		fail(w, err)
		return
	}
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	soon := time.Now().AddDate(0, 0, 3).Format("2006-01-02")
	rows := make([]issueRow, len(list))
	for i, is := range list {
		resolved := is.Status == store.IssueFixed || is.Status == store.IssueVerified || is.Status == store.IssueRejected
		rows[i] = issueRow{Issue: is, DueSoon: is.DueDate != "" && is.DueDate <= soon && !resolved}
	}
	open, blocked := 0, 0
	for _, is := range all {
		if is.Status != store.IssueFixed && is.Status != store.IssueVerified && is.Status != store.IssueRejected {
			open++
		}
		if is.Blocked {
			blocked++
		}
	}
	remove := func(key string) string {
		c := url.Values{}
		for k, v := range q {
			if k != key {
				c[k] = v
			}
		}
		if len(c) == 0 {
			return "/issues"
		}
		return "/issues?" + c.Encode()
	}
	var chips []chip
	if f.Status != "" {
		chips = append(chips, chip{"상태: " + store.IssueStatusLabel[f.Status], remove("status")})
	}
	if f.Assignee != "" {
		chips = append(chips, chip{"담당자: " + f.Assignee, remove("assignee")})
	}
	if f.Blocked == "1" {
		chips = append(chips, chip{"블록만", remove("blocked")})
	} else if f.Blocked == "0" {
		chips = append(chips, chip{"블록 아님", remove("blocked")})
	}
	if f.ScenarioID != 0 {
		if sc, err := srv.store.GetScenario(f.ScenarioID); err == nil {
			chips = append(chips, chip{"시나리오: " + sc.Name, remove("scenario")})
		}
	}
	if f.StepID != 0 {
		if st, err := srv.store.GetStep(f.StepID); err == nil {
			data["FilterStep"] = st
			chips = append(chips, chip{"스텝: " + st.Name, remove("step")})
		}
	}
	data["Issues"] = rows
	data["Filter"] = f
	data["Chips"] = chips
	data["Open"] = open
	data["BlockedCount"] = blocked
	srv.render(w, "issues.html", data)
}

func (srv *server) issueNew(w http.ResponseWriter, r *http.Request) {
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	in := store.IssueInput{Status: store.IssueOpen, Priority: "P2"}
	q := r.URL.Query()
	stepID, _ := strconv.ParseInt(q.Get("step_id"), 10, 64)
	if stepID == 0 {
		stepID, _ = strconv.ParseInt(q.Get("step"), 10, 64)
	}
	if stepID != 0 {
		in.StepIDs = []int64{stepID}
	}
	// then=block: 이슈를 저장하면 그 스텝을 블록으로 바꾼다(편집 탭에서 미해결 이슈 없는 스텝을 블록으로 고른 경우).
	if q.Get("then") == "block" && stepID != 0 {
		if st, err := srv.store.GetStep(stepID); err == nil {
			data["BlockStep"] = st
		}
	}
	data["Issue"] = store.Issue{Status: in.Status, Priority: in.Priority}
	data["Checked"] = checkedSet(in.StepIDs)
	srv.render(w, "issue_new.html", data)
}

func checkedSet(ids []int64) map[int64]bool {
	m := map[int64]bool{}
	for _, id := range ids {
		m[id] = true
	}
	return m
}

func (srv *server) issueCreate(w http.ResponseWriter, r *http.Request) {
	in := parseIssueInput(r)
	actor := strings.TrimSpace(r.FormValue("reporter"))
	blockStep, _ := strconv.ParseInt(r.FormValue("step_id"), 10, 64)
	if r.FormValue("then") != "block" {
		blockStep = 0
	}
	if blockStep != 0 {
		linked := false
		for _, sid := range in.StepIDs {
			linked = linked || sid == blockStep
		}
		if !linked {
			srv.renderIssueForm(w, "issue_new.html", in, blockStep, store.ValidationError("블록으로 바꿀 스텝이 연결 스텝에 들어 있어야 합니다"))
			return
		}
	}
	id, res, err := srv.store.CreateIssue(in, actor)
	if err != nil {
		srv.renderIssueForm(w, "issue_new.html", in, blockStep, err)
		return
	}
	if blockStep != 0 {
		st, err := srv.store.GetStep(blockStep)
		if err != nil {
			fail(w, err)
			return
		}
		if _, err := srv.store.SetStepStatuses([]store.StepStatusChange{{StepID: blockStep, Status: store.StepBlocked, CheckedBy: actor,
			Note: fmt.Sprintf("이슈 #%d 등록으로 블록", id)}}); err != nil {
			fail(w, err)
			return
		}
		st.Status = store.StepBlocked
		res.BlockedSteps = append(res.BlockedSteps, st)
		srv.notifyNewIssue(id, res)
		http.Redirect(w, r, "/scenarios/"+strconv.FormatInt(st.ScenarioID, 10)+"?tab=edit&step="+strconv.FormatInt(blockStep, 10), http.StatusSeeOther)
		return
	}
	srv.notifyNewIssue(id, res)
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

// issueView는 상세 화면을 다시 그릴 때 덧붙이는 값이다. ErrorAt은 오류를 보여 줄 폼("status", "fix", "note")이다.
type issueView struct {
	Error      string
	ErrorAt    string
	FixForm    store.IssueInput
	NoteAuthor string
	NoteBody   string
}

func (srv *server) issuePage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	srv.renderIssuePage(w, id, issueView{})
}

func (srv *server) renderIssuePage(w http.ResponseWriter, id int64, view issueView) {
	issue, err := srv.store.GetIssue(id)
	if err != nil {
		fail(w, err)
		return
	}
	notes, err := srv.store.Notes(id)
	if err != nil {
		fail(w, err)
		return
	}
	logs, err := srv.store.ChangeLogs("issue", id)
	if err != nil {
		fail(w, err)
		return
	}
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	hint, err := srv.fixHint(issue)
	if err != nil {
		fail(w, err)
		return
	}
	fix := issue
	if view.ErrorAt == "fix" {
		fix.AppliedPackage, fix.ResolutionNote, fix.JiraKey = view.FixForm.AppliedPackage, view.FixForm.ResolutionNote, view.FixForm.JiraKey
	}
	data["Issue"] = issue
	data["Fix"] = fix
	data["Notes"] = notes
	data["Logs"] = logs
	data["FixHint"] = hint
	data["Error"] = view.Error
	data["ErrorAt"] = view.ErrorAt
	data["NoteAuthor"] = view.NoteAuthor
	data["NoteBody"] = view.NoteBody
	if view.Error != "" {
		w.WriteHeader(http.StatusBadRequest)
	}
	srv.render(w, "issue.html", data)
}

// detailError는 상세 화면의 폼이 거부되었을 때 그 폼 위에 오류를 보여 주며 화면을 다시 그린다.
func (srv *server) detailError(w http.ResponseWriter, id int64, err error, view issueView) {
	var ve store.ValidationError
	if !errors.As(err, &ve) {
		fail(w, err)
		return
	}
	view.Error = ve.Error()
	srv.renderIssuePage(w, id, view)
}

// fixHint는 조치 완료로 바꾸면 연결된 블록 스텝이 어떻게 되는지 설명하는 글이다.
func (srv *server) fixHint(issue store.Issue) (string, error) {
	var parts []string
	for _, ref := range issue.Steps {
		if ref.StepStatus != store.StepBlocked {
			continue
		}
		others, err := srv.store.ListIssues(store.IssueFilter{StepID: ref.StepID})
		if err != nil {
			return "", err
		}
		var remain []string
		for _, o := range others {
			if o.ID != issue.ID && o.Status != store.IssueFixed && o.Status != store.IssueVerified && o.Status != store.IssueRejected {
				remain = append(remain, fmt.Sprintf("#%d", o.ID))
			}
		}
		if len(remain) == 0 {
			parts = append(parts, fmt.Sprintf("연결 스텝 「%s」의 미해결 이슈가 0이 되어 테스트 필요로 바뀌고 Slack에 알림이 갑니다.", ref.StepName))
		} else {
			parts = append(parts, fmt.Sprintf("연결 스텝 「%s」에는 미해결 이슈 %s가 남아 블록이 유지됩니다.", ref.StepName, strings.Join(remain, ", ")))
		}
	}
	return strings.Join(parts, " "), nil
}

func (srv *server) issueEdit(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	issue, err := srv.store.GetIssue(id)
	if err != nil {
		fail(w, err)
		return
	}
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	data["Issue"] = issue
	data["Checked"] = checkedSet(inputFrom(issue).StepIDs)
	srv.render(w, "issue_edit.html", data)
}

func (srv *server) issueUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	old, err := srv.store.GetIssue(id)
	if err != nil {
		fail(w, err)
		return
	}
	in := parseIssueInput(r)
	if err := srv.applyIssueUpdate(w, r, old, in); err != nil {
		in.Title = firstNonEmpty(in.Title, old.Title)
		srv.renderIssueForm(w, "issue_edit.html", in, 0, err, old.ID)
	}
}

// renderIssueForm은 등록·편집 폼을 입력 값 그대로 다시 그린다. 입력 오류는 400, 그 밖의 오류는 500이다.
func (srv *server) renderIssueForm(w http.ResponseWriter, page string, in store.IssueInput, blockStep int64, err error, id ...int64) {
	var ve store.ValidationError
	if !errors.As(err, &ve) {
		fail(w, err)
		return
	}
	data, derr := srv.formData()
	if derr != nil {
		fail(w, derr)
		return
	}
	issue := issueFromInput(in)
	if len(id) > 0 {
		issue.ID = id[0]
	}
	if blockStep != 0 {
		if st, err := srv.store.GetStep(blockStep); err == nil {
			data["BlockStep"] = st
		}
	}
	data["Issue"] = issue
	data["Checked"] = checkedSet(in.StepIDs)
	data["Error"] = ve.Error()
	w.WriteHeader(http.StatusBadRequest)
	srv.render(w, page, data)
}

// issueFromInput은 폼 입력 값을 화면에 다시 채우기 위한 Issue로 바꾼다.
func issueFromInput(in store.IssueInput) store.Issue {
	return store.Issue{Title: in.Title, Description: in.Description, Reporter: in.Reporter, Assignee: in.Assignee,
		Status: in.Status, Priority: in.Priority, FoundPackage: in.FoundPackage, AppliedPackage: in.AppliedPackage,
		DueDate: in.DueDate, ResolutionNote: in.ResolutionNote, JiraKey: in.JiraKey}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// issueFix는 상세 화면의 「조치 완료로 바꾸기」 폼이다. 반영 패키지, 처리 후 내용, Jira 키만 받는다.
func (srv *server) issueFix(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	old, err := srv.store.GetIssue(id)
	if err != nil {
		fail(w, err)
		return
	}
	in := inputFrom(old)
	in.Status = store.IssueFixed
	in.AppliedPackage = r.FormValue("applied_package")
	in.ResolutionNote = r.FormValue("resolution_note")
	in.JiraKey = r.FormValue("jira_key")
	if err := srv.applyIssueUpdate(w, r, old, in); err != nil {
		srv.detailError(w, id, err, issueView{ErrorAt: "fix", FixForm: in})
	}
}

// issueStatus는 상세 화면의 「상태 바꾸기」다. 조치 완료는 전용 폼으로 보낸다.
func (srv *server) issueStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	old, err := srv.store.GetIssue(id)
	if err != nil {
		fail(w, err)
		return
	}
	in := inputFrom(old)
	in.Status = r.FormValue("status")
	if in.Status == store.IssueFixed {
		srv.detailError(w, id, store.ValidationError("조치 완료는 「조치 완료로 바꾸기」 폼에서 반영 패키지와 처리 후 내용을 적어 바꿉니다"), issueView{ErrorAt: "status"})
		return
	}
	if err := srv.applyIssueUpdate(w, r, old, in); err != nil {
		srv.detailError(w, id, err, issueView{ErrorAt: "status"})
	}
}

// applyIssueUpdate는 이슈를 저장하고 상세 화면으로 보낸다. 거부되면 응답을 쓰지 않고 오류를 돌려준다.
func (srv *server) applyIssueUpdate(w http.ResponseWriter, r *http.Request, old store.Issue, in store.IssueInput) error {
	res, err := srv.store.UpdateIssue(old.ID, in, strings.TrimSpace(r.FormValue("actor")))
	if err != nil {
		return err
	}
	srv.notifyIssueUpdate(old.ID, old.Status, res)
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(old.ID, 10), http.StatusSeeOther)
	return nil
}

func (srv *server) addNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := srv.store.AddNote(id, r.FormValue("author"), r.FormValue("body")); err != nil {
		srv.detailError(w, id, err, issueView{ErrorAt: "note", NoteAuthor: r.FormValue("author"), NoteBody: r.FormValue("body")})
		return
	}
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
