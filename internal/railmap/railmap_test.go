package railmap

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hwangkyungha/blockstep/internal/store"
)

func node(id int64, name, status string, branch int64) store.StepNode {
	return store.StepNode{Step: store.Step{ID: id, Name: name, Status: status,
		ParentBranchID: sql.NullInt64{Int64: branch, Valid: branch != 0}}}
}

func positions(l *layout) map[int64]cell {
	pos := map[int64]cell{}
	for _, p := range l.steps {
		pos[p.node.ID] = cell{p.row, p.col}
	}
	return pos
}

func TestLayoutAssignsRowsTopDownInOrder(t *testing.T) {
	// 본선 1-2-3-4. 1에서 분기 A(스텝 둘, 첫 스텝에 중첩 분기 N), 1에서 분기 C(스텝 하나), 3에서 분기 B(스텝 하나).
	// 순서대로 빈 행을 받는다: A는 1행 1~2열, N은 A 아래 2행 2열, C는 1행 1열이 차서 2행 1열, B는 1행 3열.
	tree := []store.StepNode{node(1, "하나", store.StepSupported, 0), node(2, "둘", store.StepSupported, 0),
		node(3, "셋", store.StepSupported, 0), node(4, "넷", store.StepSupported, 0)}
	tree[0].Branches = []store.BranchNode{
		{Branch: store.Branch{ID: 10, Label: "A"}, Steps: []store.StepNode{node(11, "A1", store.StepUnchecked, 10), node(12, "A2", store.StepUnchecked, 10)}},
		{Branch: store.Branch{ID: 30, Label: "C"}, Steps: []store.StepNode{node(31, "C1", store.StepUnchecked, 30)}},
	}
	tree[0].Branches[0].Steps[0].Branches = []store.BranchNode{
		{Branch: store.Branch{ID: 40, Label: "N"}, Steps: []store.StepNode{node(41, "N1", store.StepUnchecked, 40)}},
	}
	tree[2].Branches = []store.BranchNode{
		{Branch: store.Branch{ID: 20, Label: "B"}, Steps: []store.StepNode{node(21, "B1", store.StepUnchecked, 20)}},
	}
	l := &layout{occupied: map[cell]bool{}}
	l.place(tree, 0, 0, false)
	pos := positions(l)
	want := map[int64]cell{11: {1, 1}, 12: {1, 2}, 41: {2, 2}, 31: {2, 1}, 21: {1, 3}}
	for id, c := range want {
		if pos[id] != c {
			t.Errorf("스텝 %d = %v, want %v", id, pos[id], c)
		}
	}
	if l.maxRow != 2 {
		t.Errorf("maxRow = %d, want 2", l.maxRow)
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

// bigTree는 스텝 19개 본선에 분기 11개(중첩 하나 포함)를 단 입력이다. 라벨은 길고 짧은 것을 섞는다.
func bigTree() []store.StepNode {
	labels := []string{"설정이 준비되지 않았거나 지침이 충돌함", "라이선스 없음", "기존에 설치된 버전 있음", "네트워크 없음", "-installDir 옵션으로 다른 경로 지정하고 설치",
		"빌드 캡처 실패", "모듈 정의가 비어 있음", "TEU 생성 실패", "생성기 LLM 응답 없음", "결과 파일 없음", "요구사항 출처가 둘 이상"}
	var tree []store.StepNode
	id := int64(1)
	for i := 0; i < 19; i++ {
		n := node(id, fmt.Sprintf("본선 스텝 %d (cmd%d)", i+1, i+1), store.StepSupported, 0)
		id++
		if i%2 == 0 && i/2 < len(labels) {
			br := store.BranchNode{Branch: store.Branch{ID: int64(100 + i), Label: labels[i/2]}}
			for k := 0; k < 1+i%3; k++ {
				br.Steps = append(br.Steps, node(id, fmt.Sprintf("분기 %d-%d", i+1, k+1), store.StepUnchecked, br.ID))
				id++
			}
			if i == 4 {
				br.Steps[0].Branches = []store.BranchNode{{Branch: store.Branch{ID: 900, Label: "중첩 조건"},
					Steps: []store.StepNode{node(id, "중첩 스텝", store.StepUnchecked, 900)}}}
				id++
			}
			n.Branches = []store.BranchNode{br}
		}
		tree = append(tree, n)
	}
	return tree
}

func TestLabelsDoNotOverlap(t *testing.T) {
	l := &layout{occupied: map[cell]bool{}}
	l.place(bigTree(), 0, 0, false)
	if len(l.branches) != 11 {
		t.Fatalf("분기 수 = %d, want 11", len(l.branches))
	}
	var rects []rect
	for _, br := range l.branches {
		r, _ := labelChip(br)
		if r.w > labelMaxW {
			t.Errorf("라벨 %q 너비 %d가 한도 %d를 넘는다", br.label, r.w, labelMaxW)
		}
		rects = append(rects, r)
	}
	for i := range rects {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].intersects(rects[j]) {
				t.Errorf("라벨 %q와 %q가 겹친다: %v %v", l.branches[i].label, l.branches[j].label, rects[i], rects[j])
			}
		}
		// 라벨은 스텝 원과도 겹치지 않는다.
		for _, p := range l.steps {
			circle := rect{x: x(p.col) - radius, y: y(p.row) - radius, w: 2 * radius, h: 2 * radius}
			if rects[i].intersects(circle) {
				t.Errorf("라벨 %q가 스텝 %s의 원과 겹친다", l.branches[i].label, p.node.Name)
			}
		}
	}
	svg := string(Render(Input{Tree: bigTree(), StaleDays: 14, Now: time.Now()}))
	if !strings.Contains(svg, `style="max-width: 100%; height: auto; min-width:`) || !strings.Contains(svg, `viewBox="0 0 `) {
		t.Errorf("viewBox와 축소 스타일이 없다")
	}
	if !strings.Contains(svg, `class="paren">(cmd1)</text>`) || strings.Contains(svg, `>본선 스텝 1 (cmd1)</text>`) {
		t.Errorf("괄호 부분이 작은 글씨 줄로 내려가지 않았다")
	}
}

func TestRenderMarksUpstreamStaleAndBadges(t *testing.T) {
	now := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -20).Format(time.RFC3339)
	recent := now.AddDate(0, 0, -3).Format(time.RFC3339)
	tree := []store.StepNode{node(1, "설치 실행", store.StepSupported, 0), node(2, "라이선스 확인", store.StepBlocked, 0),
		node(3, "설정", store.StepSupported, 0), node(4, "아주아주아주아주아주 긴 스텝 이름은 두 줄로 접는다", store.StepUnchecked, 0)}
	tree[0].CheckedAt = sql.NullString{String: old, Valid: true}
	tree[0].CheckedPackage = "2026-10-09 #101"
	tree[2].CheckedAt = sql.NullString{String: recent, Valid: true}
	tree[1].Branches = []store.BranchNode{{Branch: store.Branch{ID: 10, Label: "라이선스 없음"},
		Steps: []store.StepNode{node(11, "평가판", store.StepNeedsTest, 10)}}}
	svg := string(Render(Input{Tree: tree, Unresolved: map[int64]int{2: 2, 3: 1}, StaleDays: 14, Now: now}))

	checks := []string{
		`class="stale">확인 2026-10-09 #101</text>`, // 14일 지난 지원 스텝: 확인일과 패키지
		`class="stale">20일 지남</text>`,
		`class="step step-supported" data-step-id="3"`, // 블록 스텝 뒤라 상류 차단이어야 한다 (포함되면 안 됨)
		`class="step step-supported upstream-blocked" data-step-id="3"`,
		`class="step step-needs_test upstream-blocked" data-step-id="11"`,
		`stroke="url(#hatch)"`,
		`>상류 차단 · 확인 10-26</text>`,
		`<a href="/issues?step=2"><circle cx="265" cy="35" r="9" fill="#D9363E"/>`,
		`<a href="/issues?step=3"><circle cx="415" cy="35" r="9" fill="#8A919A"/>`,
		`<title>라이선스 없음</title>`,
		`>라이선스 없음</text>`,
		`>아주아주아주아주아주</text>`,
		`>긴 스텝 이름은 두…</text>`,
		`fill="#D9363E" stroke="#D9363E" stroke-width="4"`, // 블록 스텝은 속까지 빨강
		`stroke="#D9363E" stroke-width="10" stroke-linecap="round"`,
	}
	for i, c := range checks {
		want := !strings.Contains(c, `step-supported" data-step-id="3"`)
		if strings.Contains(svg, c) != want {
			t.Errorf("%d: %q 포함 = %v, want %v\n%s", i, c, !want, want, svg)
		}
	}
	if Render(Input{}) != "" {
		t.Errorf("빈 트리는 빈 문자열이어야 한다")
	}
}
