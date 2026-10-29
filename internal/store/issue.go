package store

import (
	"database/sql"
	"fmt"
	"strings"
)

// Issue는 issue 테이블의 한 줄이다. Blocked는 저장하지 않고 연결 스텝의 상태로 계산한다.
type Issue struct {
	ID             int64
	Title          string
	Description    string
	Reporter       string
	Assignee       string
	Status         string
	Priority       string
	FoundPackage   string
	AppliedPackage string
	DueDate        string // YYYY-MM-DD, 없으면 빈 문자열
	ResolutionNote string
	JiraKey        string
	CreatedAt      string
	UpdatedAt      string
	Blocked        bool
	Steps          []StepRef
}

// StepRef는 이슈에 연결된 스텝을 시나리오 이름과 함께 가리킨다.
type StepRef struct {
	StepID       int64
	StepName     string
	StepStatus   string
	ScenarioID   int64
	ScenarioName string
}

// IssueInput은 이슈 등록·수정 폼의 값이다.
type IssueInput struct {
	Title          string
	Description    string
	Reporter       string
	Assignee       string
	Status         string
	Priority       string
	FoundPackage   string
	AppliedPackage string
	DueDate        string
	ResolutionNote string
	JiraKey        string
	StepIDs        []int64
}

// Priorities는 긴급도 값이다. P1은 다음 패키지에 반드시 반영, P2는 이번 주 안, P3는 일정 없음이다.
var Priorities = []string{"P1", "P2", "P3"}

// AutoResult는 이슈 저장으로 일어난 자동 전환이다.
type AutoResult struct {
	NeedsTest    []Step // 테스트 필요로 바뀐 스텝
	BlockedSteps []Step // 새 이슈가 연결된 블록 스텝
}

func (in *IssueInput) validate() error {
	in.Title = strings.TrimSpace(in.Title)
	in.Description = strings.TrimSpace(in.Description)
	in.Reporter = strings.TrimSpace(in.Reporter)
	in.Assignee = strings.TrimSpace(in.Assignee)
	in.FoundPackage = strings.TrimSpace(in.FoundPackage)
	in.AppliedPackage = strings.TrimSpace(in.AppliedPackage)
	in.ResolutionNote = strings.TrimSpace(in.ResolutionNote)
	in.JiraKey = strings.TrimSpace(in.JiraKey)
	in.DueDate = strings.TrimSpace(in.DueDate)
	switch {
	case in.Title == "":
		return ValidationError("제목을 적어야 합니다")
	case len(in.StepIDs) == 0:
		return ValidationError("연결 스텝을 하나 이상 골라야 합니다")
	case in.FoundPackage == "":
		return ValidationError("발견 패키지를 적어야 합니다")
	case in.Description == "":
		return ValidationError("설명을 적어야 합니다")
	case in.Reporter == "":
		return ValidationError("리포터를 골라야 합니다")
	}
	if _, ok := IssueStatusLabel[in.Status]; !ok {
		return ValidationError("알 수 없는 처리 상태입니다: " + in.Status)
	}
	okPriority := false
	for _, p := range Priorities {
		okPriority = okPriority || p == in.Priority
	}
	if !okPriority {
		return ValidationError("긴급도는 P1, P2, P3 가운데 하나여야 합니다")
	}
	if in.Status == IssueFixed && (in.AppliedPackage == "" || in.ResolutionNote == "") {
		return ValidationError("조치 완료로 바꾸려면 반영 패키지와 처리 후 내용을 적어야 합니다")
	}
	return nil
}

// CreateIssue는 이슈를 등록한다. 담당자가 있으면 처리 상태는 담당자 배정이 된다.
// 블록 스텝에 연결되면 AutoResult.BlockedSteps에 그 스텝을 담는다.
func (s *Store) CreateIssue(in IssueInput, actor string) (int64, AutoResult, error) {
	var res AutoResult
	if in.Status == "" {
		in.Status = IssueOpen
	}
	if err := in.validate(); err != nil {
		return 0, res, err
	}
	if in.Status == IssueOpen && in.Assignee != "" {
		in.Status = IssueAssigned
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, res, err
	}
	defer tx.Rollback()
	r, err := tx.Exec(`INSERT INTO issue (title, description, reporter, assignee, status, priority, found_package, applied_package, due_date, resolution_note, jira_key)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		in.Title, in.Description, in.Reporter, in.Assignee, in.Status, in.Priority, in.FoundPackage, in.AppliedPackage, nullIfEmpty(in.DueDate), in.ResolutionNote, in.JiraKey)
	if err != nil {
		return 0, res, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return 0, res, err
	}
	if err := setIssueSteps(tx, id, nil, in.StepIDs); err != nil {
		return 0, res, err
	}
	if err := addPeople(tx, in.Reporter, in.Assignee); err != nil {
		return 0, res, err
	}
	if err := logChange(tx, "issue", id, "created", "", in.Title, actor, "이슈 등록"); err != nil {
		return 0, res, err
	}
	if res.BlockedSteps, err = stepsWithStatus(tx, in.StepIDs, StepBlocked); err != nil {
		return 0, res, err
	}
	return id, res, tx.Commit()
}

// UpdateIssue는 이슈를 고친다. 바뀐 칸마다 변경 이력을 남기고, 처리 상태가 바뀌면 규칙 2를 적용한다.
func (s *Store) UpdateIssue(id int64, in IssueInput, actor string) (AutoResult, error) {
	var res AutoResult
	old, err := s.GetIssue(id)
	if err != nil {
		return res, err
	}
	if err := in.validate(); err != nil {
		return res, err
	}
	if old.Status == IssueOpen && in.Status == IssueOpen && old.Assignee == "" && in.Assignee != "" {
		in.Status = IssueAssigned
	}
	tx, err := s.db.Begin()
	if err != nil {
		return res, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE issue SET title = ?, description = ?, reporter = ?, assignee = ?, status = ?, priority = ?, found_package = ?,
		applied_package = ?, due_date = ?, resolution_note = ?, jira_key = ?, updated_at = datetime('now') WHERE id = ?`,
		in.Title, in.Description, in.Reporter, in.Assignee, in.Status, in.Priority, in.FoundPackage,
		in.AppliedPackage, nullIfEmpty(in.DueDate), in.ResolutionNote, in.JiraKey, id); err != nil {
		return res, err
	}
	oldSteps := make([]int64, len(old.Steps))
	for i, st := range old.Steps {
		oldSteps[i] = st.StepID
	}
	if err := setIssueSteps(tx, id, oldSteps, in.StepIDs); err != nil {
		return res, err
	}
	if err := addPeople(tx, in.Reporter, in.Assignee); err != nil {
		return res, err
	}
	changes := [][3]string{
		{"title", old.Title, in.Title}, {"description", old.Description, in.Description},
		{"reporter", old.Reporter, in.Reporter}, {"assignee", old.Assignee, in.Assignee},
		{"status", old.Status, in.Status}, {"priority", old.Priority, in.Priority},
		{"found_package", old.FoundPackage, in.FoundPackage}, {"applied_package", old.AppliedPackage, in.AppliedPackage},
		{"due_date", old.DueDate, in.DueDate}, {"resolution_note", old.ResolutionNote, in.ResolutionNote},
		{"jira_key", old.JiraKey, in.JiraKey},
		{"steps", joinIDs(oldSteps), joinIDs(in.StepIDs)},
	}
	for _, c := range changes {
		if c[1] != c[2] {
			if err := logChange(tx, "issue", id, c[0], c[1], c[2], actor, ""); err != nil {
				return res, err
			}
		}
	}
	if old.Status != in.Status {
		if res.NeedsTest, err = releaseBlockedSteps(tx, id, in.Status, in.StepIDs); err != nil {
			return res, err
		}
	}
	return res, tx.Commit()
}

// releaseBlockedSteps는 규칙 2다. 이슈에 연결된 블록 스텝 가운데 미해결 이슈가 0이 된 스텝을 테스트 필요로 바꾼다.
func releaseBlockedSteps(tx *sql.Tx, issueID int64, newStatus string, stepIDs []int64) ([]Step, error) {
	var released []Step
	for _, sid := range stepIDs {
		st, err := scanStep(tx.QueryRow("SELECT "+stepColumns+" FROM step WHERE id = ?", sid))
		if err != nil {
			return nil, err
		}
		if st.Status != StepBlocked {
			continue
		}
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM issue_step js JOIN issue i ON i.id = js.issue_id WHERE js.step_id = ? AND "+unresolvedCondition, sid).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			continue
		}
		if _, err := tx.Exec("UPDATE step SET status = ? WHERE id = ?", StepNeedsTest, sid); err != nil {
			return nil, err
		}
		if err := logChange(tx, "step", sid, "status", StepBlocked, StepNeedsTest, "",
			fmt.Sprintf("이슈 #%d %s로 자동 전환", issueID, IssueStatusLabel[newStatus])); err != nil {
			return nil, err
		}
		st.Status = StepNeedsTest
		released = append(released, st)
	}
	return released, nil
}

func setIssueSteps(tx *sql.Tx, issueID int64, oldIDs, newIDs []int64) error {
	if _, err := tx.Exec("DELETE FROM issue_step WHERE issue_id = ?", issueID); err != nil {
		return err
	}
	seen := map[int64]bool{}
	for _, sid := range newIDs {
		if seen[sid] {
			continue
		}
		seen[sid] = true
		var n int
		if err := tx.QueryRow("SELECT COUNT(*) FROM step WHERE id = ?", sid).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return ValidationError(fmt.Sprintf("없는 스텝입니다: %d", sid))
		}
		if _, err := tx.Exec("INSERT INTO issue_step (issue_id, step_id) VALUES (?, ?)", issueID, sid); err != nil {
			return err
		}
	}
	return nil
}

func stepsWithStatus(tx *sql.Tx, ids []int64, status string) ([]Step, error) {
	var out []Step
	for _, sid := range ids {
		st, err := scanStep(tx.QueryRow("SELECT "+stepColumns+" FROM step WHERE id = ?", sid))
		if err != nil {
			return nil, err
		}
		if st.Status == status {
			out = append(out, st)
		}
	}
	return out, nil
}

func addPeople(tx *sql.Tx, names ...string) error {
	for _, n := range names {
		if n == "" {
			continue
		}
		if _, err := tx.Exec("INSERT OR IGNORE INTO person (name) VALUES (?)", n); err != nil {
			return err
		}
	}
	return nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func joinIDs(ids []int64) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprint(id)
	}
	return strings.Join(parts, ",")
}

const issueColumns = `i.id, i.title, i.description, i.reporter, i.assignee, i.status, i.priority, i.found_package, i.applied_package,
	COALESCE(i.due_date, '') AS due_date, i.resolution_note, i.jira_key, i.created_at, i.updated_at,
	EXISTS (SELECT 1 FROM issue_step js JOIN step st ON st.id = js.step_id WHERE js.issue_id = i.id AND st.status = 'blocked') AS blocked`

func scanIssue(row interface{ Scan(...any) error }) (Issue, error) {
	var i Issue
	err := row.Scan(&i.ID, &i.Title, &i.Description, &i.Reporter, &i.Assignee, &i.Status, &i.Priority, &i.FoundPackage, &i.AppliedPackage,
		&i.DueDate, &i.ResolutionNote, &i.JiraKey, &i.CreatedAt, &i.UpdatedAt, &i.Blocked)
	return i, err
}

// GetIssue는 이슈 하나를 연결 스텝과 함께 돌려준다.
func (s *Store) GetIssue(id int64) (Issue, error) {
	i, err := scanIssue(s.db.QueryRow("SELECT "+issueColumns+" FROM issue i WHERE i.id = ?", id))
	if err != nil {
		return i, err
	}
	refs, err := s.stepRefs([]int64{id})
	if err != nil {
		return i, err
	}
	i.Steps = refs[id]
	return i, nil
}

// IssueFilter는 이슈 목록의 필터다. 빈 값은 조건을 걸지 않는다.
type IssueFilter struct {
	Status     string
	Assignee   string
	Blocked    string // "1" 블록만, "0" 블록 아닌 것만
	ScenarioID int64
	StepID     int64
}

// ListIssues는 블록 여부, 긴급도, 목표 기한 순으로 이슈를 돌려준다.
func (s *Store) ListIssues(f IssueFilter) ([]Issue, error) {
	where := []string{"1 = 1"}
	var args []any
	if f.Status != "" {
		where = append(where, "i.status = ?")
		args = append(args, f.Status)
	}
	if f.Assignee != "" {
		where = append(where, "i.assignee = ?")
		args = append(args, f.Assignee)
	}
	if f.StepID != 0 {
		where = append(where, "EXISTS (SELECT 1 FROM issue_step js WHERE js.issue_id = i.id AND js.step_id = ?)")
		args = append(args, f.StepID)
	}
	if f.ScenarioID != 0 {
		where = append(where, "EXISTS (SELECT 1 FROM issue_step js JOIN step st ON st.id = js.step_id WHERE js.issue_id = i.id AND st.scenario_id = ?)")
		args = append(args, f.ScenarioID)
	}
	query := "SELECT * FROM (SELECT " + issueColumns + " FROM issue i WHERE " + strings.Join(where, " AND ") + ")"
	switch f.Blocked {
	case "1":
		query += " WHERE blocked = 1"
	case "0":
		query += " WHERE blocked = 0"
	}
	query += " ORDER BY blocked DESC, priority, due_date = '', due_date, id"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Issue
	var ids []int64
	for rows.Next() {
		i, err := scanIssue(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, i)
		ids = append(ids, i.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	refs, err := s.stepRefs(ids)
	if err != nil {
		return nil, err
	}
	for k := range list {
		list[k].Steps = refs[list[k].ID]
	}
	return list, nil
}

func (s *Store) stepRefs(issueIDs []int64) (map[int64][]StepRef, error) {
	out := map[int64][]StepRef{}
	if len(issueIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(issueIDs))
	for i, id := range issueIDs {
		args[i] = id
	}
	rows, err := s.db.Query(`SELECT js.issue_id, st.id, st.name, st.status, sc.id, sc.name FROM issue_step js
		JOIN step st ON st.id = js.step_id JOIN scenario sc ON sc.id = st.scenario_id
		WHERE js.issue_id IN (?`+strings.Repeat(",?", len(issueIDs)-1)+`) ORDER BY sc.sort_order, st.sort_order, st.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var iid int64
		var r StepRef
		if err := rows.Scan(&iid, &r.StepID, &r.StepName, &r.StepStatus, &r.ScenarioID, &r.ScenarioName); err != nil {
			return nil, err
		}
		out[iid] = append(out[iid], r)
	}
	return out, rows.Err()
}

// IssueNote는 처리 상황 기록 한 줄이다.
type IssueNote struct {
	ID        int64
	IssueID   int64
	Author    string
	Body      string
	CreatedAt string
}

// AddNote는 처리 상황 기록을 덧붙인다.
func (s *Store) AddNote(issueID int64, author, body string) error {
	author, body = strings.TrimSpace(author), strings.TrimSpace(body)
	if author == "" || body == "" {
		return ValidationError("작성자와 내용을 적어야 합니다")
	}
	if _, err := s.GetIssue(issueID); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO issue_note (issue_id, author, body) VALUES (?, ?, ?)", issueID, author, body); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE issue SET updated_at = datetime('now') WHERE id = ?", issueID); err != nil {
		return err
	}
	if err := addPeople(tx, author); err != nil {
		return err
	}
	if err := logChange(tx, "issue", issueID, "note", "", body, author, "처리 상황 기록"); err != nil {
		return err
	}
	return tx.Commit()
}

// Notes는 이슈의 처리 상황 기록을 오래된 순으로 돌려준다.
func (s *Store) Notes(issueID int64) ([]IssueNote, error) {
	rows, err := s.db.Query("SELECT id, issue_id, author, body, created_at FROM issue_note WHERE issue_id = ? ORDER BY id", issueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []IssueNote
	for rows.Next() {
		var n IssueNote
		if err := rows.Scan(&n.ID, &n.IssueID, &n.Author, &n.Body, &n.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, n)
	}
	return list, rows.Err()
}

// People은 등록된 사람 이름을 돌려준다.
func (s *Store) People() ([]string, error) {
	return s.queryStrings("SELECT name FROM person ORDER BY name")
}

// Packages는 스텝과 이슈에 적힌 패키지 값을 모아 돌려준다. 입력 자동 완성에 쓴다.
func (s *Store) Packages() ([]string, error) {
	return s.queryStrings(`SELECT DISTINCT p FROM (SELECT checked_package AS p FROM step UNION SELECT found_package FROM issue UNION SELECT applied_package FROM issue)
		WHERE p <> '' ORDER BY p DESC`)
}

func (s *Store) queryStrings(query string) ([]string, error) {
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, rows.Err()
}
