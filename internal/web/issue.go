package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hwangkyungha/blockstep/internal/store"
)

// scenarioSteps는 이슈 폼에서 연결 스텝을 시나리오별로 고르기 위한 묶음이다.
type scenarioSteps struct {
	store.Scenario
	Steps []store.Step
}

// formData는 이슈 폼에 공통으로 필요한 선택지다.
func (srv *server) formData() (map[string]any, error) {
	scenarios, err := srv.store.ListScenarios()
	if err != nil {
		return nil, err
	}
	steps, err := srv.store.AllSteps()
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
	byID := map[int64]*scenarioSteps{}
	for i, sc := range scenarios {
		groups[i] = &scenarioSteps{Scenario: sc}
		byID[sc.ID] = groups[i]
	}
	for _, st := range steps {
		if g, ok := byID[st.ScenarioID]; ok {
			g.Steps = append(g.Steps, st)
		}
	}
	return map[string]any{
		"Groups":        groups,
		"Scenarios":     scenarios,
		"People":        people,
		"Packages":      packages,
		"IssueStatuses": store.IssueStatuses,
		"Priorities":    store.Priorities,
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
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	data["Issues"] = list
	data["Filter"] = f
	if f.StepID != 0 {
		if st, err := srv.store.GetStep(f.StepID); err == nil {
			data["FilterStep"] = st
		}
	}
	srv.render(w, "issues.html", data)
}

func (srv *server) issueNew(w http.ResponseWriter, r *http.Request) {
	data, err := srv.formData()
	if err != nil {
		fail(w, err)
		return
	}
	in := store.IssueInput{Status: store.IssueOpen, Priority: "P2"}
	if id, err := strconv.ParseInt(r.URL.Query().Get("step"), 10, 64); err == nil {
		in.StepIDs = []int64{id}
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
	id, res, err := srv.store.CreateIssue(in, strings.TrimSpace(r.FormValue("reporter")))
	if err != nil {
		fail(w, err)
		return
	}
	srv.notifyNewIssue(id, res)
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (srv *server) issuePage(w http.ResponseWriter, r *http.Request) {
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
	ids := make([]int64, len(issue.Steps))
	for i, st := range issue.Steps {
		ids[i] = st.StepID
	}
	data["Issue"] = issue
	data["Checked"] = checkedSet(ids)
	data["Notes"] = notes
	data["Logs"] = logs
	srv.render(w, "issue.html", data)
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
	res, err := srv.store.UpdateIssue(id, parseIssueInput(r), strings.TrimSpace(r.FormValue("actor")))
	if err != nil {
		fail(w, err)
		return
	}
	srv.notifyIssueUpdate(id, old.Status, res)
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (srv *server) addNote(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if err := srv.store.AddNote(id, r.FormValue("author"), r.FormValue("body")); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/issues/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}
