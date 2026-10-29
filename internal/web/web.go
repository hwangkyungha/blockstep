// Package web은 HTTP 핸들러와 템플릿을 맡는다.
package web

import (
	"database/sql"
	"embed"
	"errors"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/hwangkyungha/blockstep/internal/outline"
	"github.com/hwangkyungha/blockstep/internal/railmap"
	"github.com/hwangkyungha/blockstep/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

// pages는 화면 이름별 템플릿이다. 모든 화면은 layout.html을 공유한다.
var pages = map[string]*template.Template{}

var funcs = template.FuncMap{
	"stepLabel":  func(s string) string { return store.StepStatusLabel[s] },
	"issueLabel": func(s string) string { return store.IssueStatusLabel[s] },
	"list":       func(a ...any) []any { return a },
}

func init() {
	for _, name := range []string{"scenarios.html", "scenario.html", "issues.html", "issue_new.html", "issue.html"} {
		pages[name] = template.Must(template.New("").Funcs(funcs).ParseFS(templateFS, "templates/layout.html", "templates/_*.html", "templates/"+name))
	}
}

type server struct {
	store *store.Store
}

// Handler는 라우팅이 끝난 http.Handler를 돌려준다.
func Handler(s *store.Store) http.Handler {
	srv := &server{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", srv.scenarioList)
	mux.HandleFunc("POST /scenarios", srv.createScenario)
	mux.HandleFunc("GET /scenarios/{id}", srv.scenarioPage)
	mux.HandleFunc("PUT /scenarios/{id}/outline", srv.saveOutline)
	mux.HandleFunc("POST /steps/{id}/status", srv.setStepStatus)
	mux.HandleFunc("GET /issues", srv.issueList)
	mux.HandleFunc("GET /issues/new", srv.issueNew)
	mux.HandleFunc("POST /issues", srv.issueCreate)
	mux.HandleFunc("GET /issues/{id}", srv.issuePage)
	mux.HandleFunc("POST /issues/{id}", srv.issueUpdate)
	mux.HandleFunc("POST /issues/{id}/notes", srv.addNote)
	return mux
}

func (srv *server) render(w http.ResponseWriter, page string, data any) {
	if err := pages[page].ExecuteTemplate(w, "layout", data); err != nil {
		log.Println(err)
	}
}

// fail은 오류를 응답으로 바꾼다. 입력 오류는 400, 없는 대상은 404, 나머지는 500이다.
func fail(w http.ResponseWriter, err error) {
	var ve store.ValidationError
	switch {
	case errors.As(err, &ve):
		http.Error(w, ve.Error(), http.StatusBadRequest)
	case errors.Is(err, sql.ErrNoRows):
		http.Error(w, "없는 항목입니다", http.StatusNotFound)
	default:
		log.Println(err)
		http.Error(w, "서버 오류", http.StatusInternalServerError)
	}
}

func pathID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		return 0, sql.ErrNoRows
	}
	return id, nil
}

// scenarioStat은 시나리오 목록 한 줄의 요약이다. 상태별 수, 블록 스텝 수, 확인일이 오래된 스텝 수를 센다.
type scenarioStat struct {
	store.Scenario
	Total   int
	Counts  map[string]int
	Blocked int
	Stale   int
}

func (st *scenarioStat) add(step store.Step, staleDays int, now time.Time) {
	st.Total++
	st.Counts[step.Status]++
	if step.Status == store.StepBlocked {
		st.Blocked++
	}
	if step.Status == store.StepSupported && step.CheckedAt.Valid && int(now.Sub(step.CheckedTime()).Hours()/24) >= staleDays {
		st.Stale++
	}
}

// Percent는 상태별 비율 막대의 너비(%)다.
func (st *scenarioStat) Percent(status string) float64 {
	if st.Total == 0 {
		return 0
	}
	return float64(st.Counts[status]) * 100 / float64(st.Total)
}

func (srv *server) scenarioList(w http.ResponseWriter, r *http.Request) {
	list, err := srv.store.ListScenarios()
	if err != nil {
		fail(w, err)
		return
	}
	steps, err := srv.store.AllSteps()
	if err != nil {
		fail(w, err)
		return
	}
	staleDays, err := srv.store.StaleDays()
	if err != nil {
		fail(w, err)
		return
	}
	now := time.Now()
	total := &scenarioStat{Counts: map[string]int{}}
	stats := make([]*scenarioStat, len(list))
	byID := map[int64]*scenarioStat{}
	for i, sc := range list {
		stats[i] = &scenarioStat{Scenario: sc, Counts: map[string]int{}}
		byID[sc.ID] = stats[i]
	}
	for _, step := range steps {
		if st, ok := byID[step.ScenarioID]; ok {
			st.add(step, staleDays, now)
		}
		total.add(step, staleDays, now)
	}
	srv.render(w, "scenarios.html", map[string]any{
		"Scenarios": stats, "Total": total, "StaleDays": staleDays, "StepStatuses": store.StepStatuses, "Colors": railmap.Color,
	})
}

func (srv *server) createScenario(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		fail(w, store.ValidationError("이름은 비울 수 없습니다"))
		return
	}
	id, err := srv.store.CreateScenario(name, strings.TrimSpace(r.FormValue("description")))
	if err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/scenarios/"+strconv.FormatInt(id, 10), http.StatusSeeOther)
}

func (srv *server) scenarioPage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	sc, err := srv.store.GetScenario(id)
	if err != nil {
		fail(w, err)
		return
	}
	tree, err := srv.store.ScenarioTree(id)
	if err != nil {
		fail(w, err)
		return
	}
	steps, err := srv.store.ScenarioSteps(id)
	if err != nil {
		fail(w, err)
		return
	}
	unresolved, err := srv.store.UnresolvedCounts(id)
	if err != nil {
		fail(w, err)
		return
	}
	staleDays, err := srv.store.StaleDays()
	if err != nil {
		fail(w, err)
		return
	}
	people, err := srv.store.People()
	if err != nil {
		fail(w, err)
		return
	}
	packages, err := srv.store.Packages()
	if err != nil {
		fail(w, err)
		return
	}
	srv.render(w, "scenario.html", map[string]any{
		"People":       people,
		"Packages":     packages,
		"Scenario":     sc,
		"Map":          railmap.Render(railmap.Input{Tree: tree, Unresolved: unresolved, StaleDays: staleDays, Now: time.Now()}),
		"Outline":      outline.Format(store.ToOutline(tree)),
		"Steps":        steps,
		"StepStatuses": store.StepStatuses,
	})
}

func (srv *server) saveOutline(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	if _, err := srv.store.GetScenario(id); err != nil {
		fail(w, err)
		return
	}
	steps, err := outline.Parse(r.FormValue("outline"))
	if err != nil {
		fail(w, store.ValidationError(err.Error()))
		return
	}
	if err := srv.store.SaveOutline(id, steps, strings.TrimSpace(r.FormValue("actor"))); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (srv *server) setStepStatus(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		fail(w, err)
		return
	}
	st, err := srv.store.GetStep(id)
	if err != nil {
		fail(w, err)
		return
	}
	_, err = srv.store.SetStepStatus(id, r.FormValue("status"),
		strings.TrimSpace(r.FormValue("checked_package")), strings.TrimSpace(r.FormValue("checked_by")))
	if err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/scenarios/"+strconv.FormatInt(st.ScenarioID, 10), http.StatusSeeOther)
}
