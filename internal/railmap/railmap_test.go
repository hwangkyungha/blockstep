package railmap

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/hwangkyungha/blockstep/internal/store"
)

func node(id int64, name, status string, branch int64) store.StepNode {
	return store.StepNode{Step: store.Step{ID: id, Name: name, Status: status,
		ParentBranchID: sql.NullInt64{Int64: branch, Valid: branch != 0}}}
}

func TestLayoutBranchesDoNotOverlap(t *testing.T) {
	// 본선 1-2-3-4. 1에서 분기 A(스텝 둘), 3에서 분기 B(스텝 하나), 1에서 분기 C(스텝 하나).
	// 뒤쪽 스텝의 분기 B를 먼저 본선 바로 아래 행에 놓는다. B의 꺾이는 칸(1행 2열)이 A의 가로 선과 겹치므로
	// A는 2행으로 밀리고, 1의 두 번째 분기 C는 1행 1열에 들어간다.
	tree := []store.StepNode{node(1, "하나", store.StepSupported, 0), node(2, "둘", store.StepSupported, 0),
		node(3, "셋", store.StepSupported, 0), node(4, "넷", store.StepSupported, 0)}
	tree[0].Branches = []store.BranchNode{
		{Branch: store.Branch{ID: 10, Label: "A"}, Steps: []store.StepNode{node(11, "A1", store.StepUnchecked, 10), node(12, "A2", store.StepUnchecked, 10)}},
		{Branch: store.Branch{ID: 30, Label: "C"}, Steps: []store.StepNode{node(31, "C1", store.StepUnchecked, 30)}},
	}
	tree[2].Branches = []store.BranchNode{
		{Branch: store.Branch{ID: 20, Label: "B"}, Steps: []store.StepNode{node(21, "B1", store.StepUnchecked, 20)}},
	}
	l := &layout{occupied: map[cell]int64{}}
	l.place(tree, 0, 0, false)
	pos := map[int64]cell{}
	for _, p := range l.steps {
		pos[p.node.ID] = cell{p.row, p.col}
	}
	if pos[21] != (cell{1, 3}) {
		t.Errorf("B1 = %v, want {1 3}", pos[21])
	}
	if pos[11] != (cell{2, 1}) || pos[12] != (cell{2, 2}) {
		t.Errorf("A = %v %v, want {2 1} {2 2}", pos[11], pos[12])
	}
	if pos[31] != (cell{1, 1}) {
		t.Errorf("C1 = %v, want {1 1}", pos[31])
	}
	seen := map[cell]int64{}
	for _, p := range l.steps {
		c := cell{p.row, p.col}
		if other, dup := seen[c]; dup {
			t.Errorf("스텝 %d와 %d가 같은 칸 %v", other, p.node.ID, c)
		}
		seen[c] = p.node.ID
	}
}

func TestLayoutPushesEarlierBranchBelowLaterOne(t *testing.T) {
	// 1에서 긴 분기(스텝 셋), 2에서 짧은 분기. 긴 분기가 2열을 지나므로 짧은 분기 아래로 밀린다.
	tree := []store.StepNode{node(1, "하나", store.StepUnchecked, 0), node(2, "둘", store.StepUnchecked, 0)}
	tree[0].Branches = []store.BranchNode{{Branch: store.Branch{ID: 10, Label: "긴"},
		Steps: []store.StepNode{node(11, "a", store.StepUnchecked, 10), node(12, "b", store.StepUnchecked, 10), node(13, "c", store.StepUnchecked, 10)}}}
	tree[1].Branches = []store.BranchNode{{Branch: store.Branch{ID: 20, Label: "짧은"},
		Steps: []store.StepNode{node(21, "d", store.StepUnchecked, 20)}}}
	l := &layout{occupied: map[cell]int64{}}
	l.place(tree, 0, 0, false)
	pos := map[int64]cell{}
	for _, p := range l.steps {
		pos[p.node.ID] = cell{p.row, p.col}
	}
	if pos[21] != (cell{1, 2}) || pos[11] != (cell{2, 1}) {
		t.Errorf("짧은 = %v, 긴 = %v", pos[21], pos[11])
	}
}

func TestRenderMarksUpstreamStaleAndBadges(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -20).Format(time.RFC3339)
	recent := now.AddDate(0, 0, -3).Format(time.RFC3339)
	tree := []store.StepNode{node(1, "설치 실행", store.StepSupported, 0), node(2, "라이선스 확인", store.StepBlocked, 0),
		node(3, "설정", store.StepSupported, 0), node(4, "아주아주아주아주아주 긴 스텝 이름은 두 줄로 접는다", store.StepUnchecked, 0)}
	tree[0].CheckedAt = sql.NullString{String: old, Valid: true}
	tree[2].CheckedAt = sql.NullString{String: recent, Valid: true}
	tree[1].Branches = []store.BranchNode{{Branch: store.Branch{ID: 10, Label: "라이선스 없음"},
		Steps: []store.StepNode{node(11, "평가판", store.StepNeedsTest, 10)}}}
	svg := string(Render(Input{Tree: tree, Unresolved: map[int64]int{2: 2, 3: 1}, StaleDays: 14, Now: now}))

	checks := []string{
		`2026-10-09 · 20일 지남`,                          // 14일 지난 지원 스텝
		`class="step step-supported" data-step-id="3"`, // 상류 차단 아님 (블록 스텝 앞)
		`class="step step-supported upstream-blocked" data-step-id="3"`,
		`class="step step-needs_test upstream-blocked" data-step-id="11"`,
		`fill="url(#hatch)"`,
		`<a href="/issues?step=2"><circle cx="243" cy="27" r="9" fill="#D9363E"/>`,
		`<a href="/issues?step=3"><circle cx="393" cy="27" r="9" fill="#8A8F98"/>`,
		`>라이선스 없음</text>`,
		`>아주아주아주아주아주 </text>`,
		`>긴 스텝 이름은 두…</text>`,
		`stroke="#D9363E" stroke-width="8"`, // 블록 스텝에 이르는 구간
	}
	for i, c := range checks {
		want := !strings.Contains(c, `step-supported" data-step-id="3"`)
		if strings.Contains(svg, c) != want {
			t.Errorf("%d: %q 포함 = %v, want %v\n%s", i, c, !want, want, svg)
		}
	}
	if strings.Contains(svg, "2026-10-26 ·") {
		t.Errorf("3일 지난 스텝에 지남 표시가 붙었다")
	}
	if Render(Input{}) != "" {
		t.Errorf("빈 트리는 빈 문자열이어야 한다")
	}
}
