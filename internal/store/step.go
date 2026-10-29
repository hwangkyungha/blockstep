package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/hwangkyungha/blockstep/internal/outline"
)

// Step은 step 테이블의 한 줄이다.
type Step struct {
	ID             int64
	ScenarioID     int64
	ParentBranchID sql.NullInt64
	SortOrder      int
	Name           string
	Refs           string
	Status         string
	CheckedPackage string
	CheckedBy      string
	CheckedAt      sql.NullString // RFC3339
}

// CheckedTime은 확인일을 time.Time으로 돌려준다. 없으면 영(零) 값이다.
func (s Step) CheckedTime() time.Time {
	if !s.CheckedAt.Valid {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339, s.CheckedAt.String)
	return t
}

// Branch는 branch 테이블의 한 줄이다.
type Branch struct {
	ID         int64
	FromStepID int64
	Label      string
	SortOrder  int
}

// StepNode는 분기를 포함한 스텝 트리의 마디다.
type StepNode struct {
	Step
	Branches []BranchNode
}

// BranchNode는 스텝 트리의 분기 마디다.
type BranchNode struct {
	Branch
	Steps []StepNode
}

// GetScenario는 시나리오 하나를 돌려준다. 없으면 sql.ErrNoRows다.
func (s *Store) GetScenario(id int64) (Scenario, error) {
	var sc Scenario
	err := s.db.QueryRow("SELECT id, name, description, sort_order FROM scenario WHERE id = ?", id).
		Scan(&sc.ID, &sc.Name, &sc.Description, &sc.SortOrder)
	return sc, err
}

const stepColumns = "id, scenario_id, parent_branch_id, sort_order, name, refs, status, checked_package, checked_by, checked_at"

func scanStep(row interface{ Scan(...any) error }) (Step, error) {
	var st Step
	err := row.Scan(&st.ID, &st.ScenarioID, &st.ParentBranchID, &st.SortOrder, &st.Name, &st.Refs,
		&st.Status, &st.CheckedPackage, &st.CheckedBy, &st.CheckedAt)
	return st, err
}

func (s *Store) querySteps(query string, args ...any) ([]Step, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Step
	for rows.Next() {
		st, err := scanStep(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, st)
	}
	return list, rows.Err()
}

// GetStep은 스텝 하나를 돌려준다.
func (s *Store) GetStep(id int64) (Step, error) {
	return scanStep(s.db.QueryRow("SELECT "+stepColumns+" FROM step WHERE id = ?", id))
}

// ScenarioSteps는 시나리오의 모든 스텝을 sort_order 순으로 돌려준다.
func (s *Store) ScenarioSteps(scenarioID int64) ([]Step, error) {
	return s.querySteps("SELECT "+stepColumns+" FROM step WHERE scenario_id = ? ORDER BY sort_order, id", scenarioID)
}

// AllSteps는 모든 시나리오의 스텝을 시나리오, sort_order 순으로 돌려준다.
func (s *Store) AllSteps() ([]Step, error) {
	return s.querySteps("SELECT " + stepColumns + " FROM step ORDER BY scenario_id, sort_order, id")
}

// ScenarioTree는 시나리오의 스텝과 분기를 트리로 돌려준다.
func (s *Store) ScenarioTree(scenarioID int64) ([]StepNode, error) {
	steps, err := s.ScenarioSteps(scenarioID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		"SELECT b.id, b.from_step_id, b.label, b.sort_order FROM branch b JOIN step st ON st.id = b.from_step_id WHERE st.scenario_id = ? ORDER BY b.sort_order, b.id",
		scenarioID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	branchesOf := map[int64][]Branch{}
	for rows.Next() {
		var b Branch
		if err := rows.Scan(&b.ID, &b.FromStepID, &b.Label, &b.SortOrder); err != nil {
			return nil, err
		}
		branchesOf[b.FromStepID] = append(branchesOf[b.FromStepID], b)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	stepsOf := map[int64][]Step{} // 분기 id별 스텝. 본선은 0
	for _, st := range steps {
		stepsOf[st.ParentBranchID.Int64] = append(stepsOf[st.ParentBranchID.Int64], st)
	}
	var build func(branchID int64) []StepNode
	build = func(branchID int64) []StepNode {
		var nodes []StepNode
		for _, st := range stepsOf[branchID] {
			n := StepNode{Step: st}
			for _, b := range branchesOf[st.ID] {
				n.Branches = append(n.Branches, BranchNode{Branch: b, Steps: build(b.ID)})
			}
			nodes = append(nodes, n)
		}
		return nodes
	}
	return build(0), nil
}

// ToOutline은 스텝 트리를 아웃라인 트리로 바꾼다.
func ToOutline(nodes []StepNode) []outline.Step {
	var steps []outline.Step
	for _, n := range nodes {
		st := outline.Step{Name: n.Name, Refs: n.Refs}
		for _, b := range n.Branches {
			st.Branches = append(st.Branches, outline.Branch{Label: b.Label, Steps: ToOutline(b.Steps)})
		}
		steps = append(steps, st)
	}
	return steps
}

// SaveOutline은 아웃라인 트리를 시나리오에 저장한다.
// 이름이 같은 기존 스텝은 id, 상태, 이슈 연결을 유지하고, 사라진 스텝은 이슈 연결을 풀고 지운다.
func (s *Store) SaveOutline(scenarioID int64, steps []outline.Step, actor string) error {
	old, err := s.ScenarioSteps(scenarioID)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	byName := map[string][]Step{}
	for _, st := range old {
		byName[st.Name] = append(byName[st.Name], st)
	}
	if _, err := tx.Exec("UPDATE step SET parent_branch_id = NULL WHERE scenario_id = ?", scenarioID); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM branch WHERE from_step_id IN (SELECT id FROM step WHERE scenario_id = ?)", scenarioID); err != nil {
		return err
	}
	kept := map[int64]bool{}
	var walk func(steps []outline.Step, branchID sql.NullInt64) error
	walk = func(steps []outline.Step, branchID sql.NullInt64) error {
		for i, st := range steps {
			var id int64
			if ex := byName[st.Name]; len(ex) > 0 {
				byName[st.Name] = ex[1:]
				id = ex[0].ID
				if _, err := tx.Exec("UPDATE step SET parent_branch_id = ?, sort_order = ?, refs = ? WHERE id = ?", branchID, i, st.Refs, id); err != nil {
					return err
				}
				if ex[0].Refs != st.Refs {
					if err := logChange(tx, "step", id, "refs", ex[0].Refs, st.Refs, actor, ""); err != nil {
						return err
					}
				}
			} else {
				res, err := tx.Exec("INSERT INTO step (scenario_id, parent_branch_id, sort_order, name, refs) VALUES (?, ?, ?, ?, ?)",
					scenarioID, branchID, i, st.Name, st.Refs)
				if err != nil {
					return err
				}
				if id, err = res.LastInsertId(); err != nil {
					return err
				}
				if err := logChange(tx, "step", id, "name", "", st.Name, actor, "스텝 추가"); err != nil {
					return err
				}
			}
			kept[id] = true
			for j, br := range st.Branches {
				res, err := tx.Exec("INSERT INTO branch (from_step_id, label, sort_order) VALUES (?, ?, ?)", id, br.Label, j)
				if err != nil {
					return err
				}
				bid, err := res.LastInsertId()
				if err != nil {
					return err
				}
				if err := walk(br.Steps, sql.NullInt64{Int64: bid, Valid: true}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(steps, sql.NullInt64{}); err != nil {
		return err
	}
	for _, st := range old {
		if kept[st.ID] {
			continue
		}
		if _, err := tx.Exec("DELETE FROM issue_step WHERE step_id = ?", st.ID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM step WHERE id = ?", st.ID); err != nil {
			return err
		}
		if err := logChange(tx, "step", st.ID, "name", st.Name, "", actor, "스텝 삭제"); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func logChange(db execer, entity string, entityID int64, field, oldValue, newValue, actor, note string) error {
	_, err := db.Exec("INSERT INTO change_log (entity, entity_id, field, old_value, new_value, actor, note) VALUES (?, ?, ?, ?, ?, ?, ?)",
		entity, entityID, field, oldValue, newValue, actor, note)
	return err
}

// SetStepStatus는 스텝 상태를 바꾼다. 규칙은 설계안 5.1과 5.3이다.
//   - 지원: 확인 패키지와 확인자가 필수이고 확인일을 지금으로 적는다.
//   - 블록: 미해결 이슈가 하나 이상 연결되어 있어야 한다.
//   - 지원으로 바꾸면 연결된 조치 완료 이슈 가운데 연결 스텝이 모두 지원인 이슈를 검증 완료로 바꾼다.
//
// 돌려주는 값은 검증 완료로 바뀐 이슈 id 목록이다.
func (s *Store) SetStepStatus(id int64, status, checkedPackage, checkedBy string) ([]int64, error) {
	if _, ok := StepStatusLabel[status]; !ok {
		return nil, ValidationError("알 수 없는 상태입니다: " + status)
	}
	st, err := s.GetStep(id)
	if err != nil {
		return nil, err
	}
	switch status {
	case StepSupported:
		if checkedPackage == "" || checkedBy == "" {
			return nil, ValidationError("지원으로 바꾸려면 확인 패키지와 확인자를 적어야 합니다")
		}
	case StepBlocked:
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM issue_step js JOIN issue i ON i.id = js.issue_id WHERE js.step_id = ? AND "+unresolvedCondition, id).Scan(&n); err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, ValidationError("블록으로 바꾸려면 미해결 이슈를 먼저 연결해야 합니다")
		}
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if status == StepSupported {
		now := time.Now().Format(time.RFC3339)
		if _, err := tx.Exec("UPDATE step SET status = ?, checked_package = ?, checked_by = ?, checked_at = ? WHERE id = ?",
			status, checkedPackage, checkedBy, now, id); err != nil {
			return nil, err
		}
		if st.CheckedPackage != checkedPackage {
			if err := logChange(tx, "step", id, "checked_package", st.CheckedPackage, checkedPackage, checkedBy, ""); err != nil {
				return nil, err
			}
		}
	} else if _, err := tx.Exec("UPDATE step SET status = ? WHERE id = ?", status, id); err != nil {
		return nil, err
	}
	if st.Status != status {
		if err := logChange(tx, "step", id, "status", st.Status, status, checkedBy, ""); err != nil {
			return nil, err
		}
	}

	var verified []int64
	if status == StepSupported {
		rows, err := tx.Query(`SELECT i.id FROM issue i JOIN issue_step js ON js.issue_id = i.id
			WHERE js.step_id = ? AND i.status = 'fixed'
			AND NOT EXISTS (SELECT 1 FROM issue_step js2 JOIN step s2 ON s2.id = js2.step_id WHERE js2.issue_id = i.id AND s2.status <> 'supported')`, id)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var iid int64
			if err := rows.Scan(&iid); err != nil {
				rows.Close()
				return nil, err
			}
			verified = append(verified, iid)
		}
		rows.Close()
		for _, iid := range verified {
			if _, err := tx.Exec("UPDATE issue SET status = 'verified', updated_at = datetime('now') WHERE id = ?", iid); err != nil {
				return nil, err
			}
			if err := logChange(tx, "issue", iid, "status", IssueFixed, IssueVerified, checkedBy,
				fmt.Sprintf("스텝 「%s」 지원 전환으로 자동 전환", st.Name)); err != nil {
				return nil, err
			}
		}
	}
	return verified, tx.Commit()
}

// ChangeLog는 변경 이력의 한 줄이다.
type ChangeLog struct {
	ID       int64
	Entity   string
	EntityID int64
	Field    string
	OldValue string
	NewValue string
	Actor    string
	Note     string
	At       string
}

// ChangeLogs는 어떤 대상의 변경 이력을 오래된 순으로 돌려준다.
func (s *Store) ChangeLogs(entity string, entityID int64) ([]ChangeLog, error) {
	rows, err := s.db.Query("SELECT id, entity, entity_id, field, old_value, new_value, actor, note, at FROM change_log WHERE entity = ? AND entity_id = ? ORDER BY id", entity, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ChangeLog
	for rows.Next() {
		var c ChangeLog
		if err := rows.Scan(&c.ID, &c.Entity, &c.EntityID, &c.Field, &c.OldValue, &c.NewValue, &c.Actor, &c.Note, &c.At); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}
