// Package railmap은 시나리오 하나를 지하철 노선도 모양의 SVG로 그린다.
//
// 본선은 가로 선 하나이고 스텝은 선 위의 둥근 표시다. 스텝 사이 구간에는 그 구간이 이르는
// 스텝의 상태 색을 칠한다. 분기는 시작 스텝 다음 칸의 중간 지점에서 아래로 내려가 가로로 이어지는 선이다.
// 상류에 블록 스텝이 있는 스텝은 연한 색에 빗금을 덧그린다(상류 차단). 이 표시는 저장하지 않고 계산한다.
package railmap

import (
	"fmt"
	"html"
	"html/template"
	"regexp"
	"strings"
	"time"

	"github.com/hwangkyungha/blockstep/internal/store"
)

const (
	colW       = 150 // 스텝 사이 가로 간격
	rowH       = 156 // 본선과 분기 사이 세로 간격
	marginX    = 100
	marginY    = 50
	radius     = 12
	lineW      = 10
	nameChars  = 11        // 이름 한 줄의 글자 수
	labelChars = 12        // 분기 라벨 한 줄의 글자 수
	labelMaxW  = colW - 14 // 분기 라벨 칩의 최대 너비. 다음 열의 라벨과 겹치지 않는다
	minScale   = 0.6       // 컨테이너에 맞춰 줄일 때의 하한
)

// Color는 상태별 색이다. 미확인은 흰색 바탕에 테두리만 그린다.
var Color = map[string]string{
	store.StepSupported:      "#2E8B57",
	store.StepBlocked:        "#D9363E",
	store.StepNeedsTest:      "#E0A106",
	store.StepNotImplemented: "#B9BEC5",
	store.StepUnchecked:      "#FFFFFF",
}

// strokeColor는 선과 테두리에 쓰는 색이다. 미확인은 흰색이라 연한 회색으로 그린다.
func strokeColor(status string) string {
	if status == store.StepUnchecked {
		return "#B9BEC5"
	}
	return Color[status]
}

// Input은 노선도를 그리는 데 필요한 값이다.
type Input struct {
	Tree       []store.StepNode
	Unresolved map[int64]int // 스텝 id별 미해결 이슈 수
	StaleDays  int
	Now        time.Time
}

type cell struct{ row, col int }

type placed struct {
	node     *store.StepNode
	row, col int
	upstream bool // 상류에 블록 스텝이 있다
	last     bool // 자기 목록(본선 또는 분기)의 마지막 스텝이다
}

type branchLine struct {
	from  *placed
	first *placed
	label string
}

// layout은 스텝을 행과 열에 놓은 결과다. 행 0이 본선이고, 분기는 위에서부터 차례로 빈 행을 받는다.
type layout struct {
	occupied map[cell]bool // 스텝이 놓인 칸
	steps    []*placed
	branches []branchLine
	maxRow   int
	maxCol   int
}

// place는 steps를 row 행의 startCol 열부터 차례로 놓고, 각 스텝의 분기를 순서대로 아래의 첫 빈 행에 놓는다.
// 분기 안의 분기는 그 분기 행 아래에 놓인다. 내려가는 선은 열과 열 사이를 지나므로 다른 행의 스텝 원과 만나지 않는다.
func (l *layout) place(steps []store.StepNode, row, startCol int, upstream bool) {
	var ps []*placed
	for i := range steps {
		n := &steps[i]
		p := &placed{node: n, row: row, col: startCol + i, upstream: upstream, last: i == len(steps)-1}
		l.occupied[cell{row, p.col}] = true
		l.steps = append(l.steps, p)
		ps = append(ps, p)
		if n.Status == store.StepBlocked {
			upstream = true
		}
		if p.col > l.maxCol {
			l.maxCol = p.col
		}
	}
	for i, p := range ps {
		for k := range p.node.Branches {
			br := &p.node.Branches[k]
			r := row + 1
			for !l.fits(r, p.col+1, len(br.Steps)) {
				r++
			}
			if r > l.maxRow {
				l.maxRow = r
			}
			before := len(l.steps)
			l.place(br.Steps, r, p.col+1, p.upstream || l.blockedUpTo(ps, i))
			if len(l.steps) > before {
				l.branches = append(l.branches, branchLine{from: p, first: l.steps[before], label: br.Label})
			}
		}
	}
}

func (l *layout) blockedUpTo(ps []*placed, i int) bool {
	for _, p := range ps[:i+1] {
		if p.node.Status == store.StepBlocked {
			return true
		}
	}
	return false
}

// fits는 r 행의 startCol 열부터 n칸이 비어 있는지 본다.
func (l *layout) fits(r, startCol, n int) bool {
	for c := startCol; c < startCol+n; c++ {
		if l.occupied[cell{r, c}] {
			return false
		}
	}
	return true
}

func x(col int) int { return marginX + col*colW }
func y(row int) int { return marginY + row*rowH }

// rect는 라벨 칩의 자리다. 겹침 검사에 쓴다.
type rect struct{ x, y, w, h int }

func (a rect) intersects(b rect) bool {
	return a.x < b.x+b.w && b.x < a.x+a.w && a.y < b.y+b.h && b.y < a.y+a.h
}

// labelChip은 분기 라벨 칩의 자리와 글 줄이다. 가로 선 시작점(꺾이는 곳) 바로 위에 둔다.
func labelChip(br branchLine) (rect, []string) {
	lines := wrap(br.label, labelChars)
	w := 0
	for _, ln := range lines {
		if tw := textWidth(ln) + 16; tw > w {
			w = tw
		}
	}
	if w > labelMaxW {
		w = labelMaxW
	}
	h := 18
	if len(lines) > 1 {
		h = 32
	}
	cx := x(br.from.col) + colW/2
	ty := y(br.first.row)
	return rect{x: cx + 8, y: ty - 28 - h, w: w, h: h}, lines
}

// Render는 노선도 SVG를 돌려준다. 스텝이 없으면 빈 문자열이다.
func Render(in Input) template.HTML {
	if len(in.Tree) == 0 {
		return ""
	}
	l := &layout{occupied: map[cell]bool{}}
	l.place(in.Tree, 0, 0, false)

	var b strings.Builder
	width := x(l.maxCol) + colW
	height := y(l.maxRow) + rowH
	// viewBox를 두고 컨테이너 너비에 맞춰 줄인다. minScale보다 작아져야 하면 컨테이너가 가로로 스크롤한다.
	fmt.Fprintf(&b, `<svg class="railmap" xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" style="max-width: 100%%; height: auto; min-width: %dpx" font-family="'Noto Sans KR', system-ui, sans-serif" font-size="13">`,
		width, height, width, height, int(float64(width)*minScale))
	b.WriteString(`<defs><pattern id="hatch" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><line x1="0" y1="0" x2="0" y2="8" stroke="#FFFFFF" stroke-width="4"/></pattern></defs>`)

	// 분기 선을 먼저 그려 본선 구간이 그 위를 덮게 한다. 분기 시작 스텝이 목록의 마지막이면 짧은 연결선이 드러난다.
	for _, br := range l.branches {
		fx, fy := x(br.from.col), y(br.from.row)
		mx := fx + colW/2
		tx, ty := x(br.first.col), y(br.first.row)
		segment(&b, fmt.Sprintf(`M%d %d H%d V%d Q%d %d %d %d H%d`, fx, fy, mx, ty-18, mx, ty, mx+18, ty, tx), br.first)
	}
	// 구간 선. 같은 행에서 앞 스텝과 잇는다. 본선의 첫 스텝은 짧은 시작 선을 둔다.
	prevInRow := map[cell]*placed{}
	for _, p := range l.steps {
		if prev, ok := prevInRow[cell{p.row, p.col - 1}]; ok && sameList(prev, p) {
			segment(&b, fmt.Sprintf(`M%d %d H%d`, x(prev.col), y(prev.row), x(p.col)), p)
		} else if p.row == 0 && p.col == 0 {
			segment(&b, fmt.Sprintf(`M%d %d H%d`, x(0)-36, y(0), x(0)), p)
		}
		prevInRow[cell{p.row, p.col}] = p
	}
	for _, br := range l.branches {
		r, lines := labelChip(br)
		fmt.Fprintf(&b, `<g class="branch-label"><title>%s</title><rect x="%d" y="%d" width="%d" height="%d" rx="4" fill="#FFFFFF" stroke="#D0D4DA"/>`,
			html.EscapeString(br.label), r.x, r.y, r.w, r.h)
		for i, ln := range lines {
			fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#444B55" font-size="11">%s</text>`, r.x+8, r.y+13+i*14, html.EscapeString(ln))
		}
		b.WriteString(`</g>`)
	}

	for _, p := range l.steps {
		st := p.node
		cx, cy := x(p.col), y(p.row)
		stroke := strokeColor(st.Status)
		fill := "#FFFFFF"
		if st.Status == store.StepBlocked {
			fill = Color[store.StepBlocked]
		}
		fmt.Fprintf(&b, `<g class="step step-%s%s" data-step-id="%d" tabindex="0" role="button"><title>%s</title>`,
			st.Status, ternary(p.upstream, " upstream-blocked", ""), st.ID, html.EscapeString(st.Name))
		opacity := ""
		if p.upstream {
			opacity = ` opacity="0.55"`
		}
		fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" stroke="%s" stroke-width="4"%s/>`, cx, cy, radius, fill, stroke, opacity)

		nameFill, nameWeight, subFill := "#1F2328", "500", "#5A616B"
		switch {
		case p.upstream:
			nameFill, subFill = "#8A919A", "#8A919A"
		case st.Status == store.StepBlocked:
			nameFill, nameWeight, subFill = "#B5252C", "700", "#B5252C"
		case st.Status == store.StepNeedsTest:
			nameFill, subFill = "#8A6200", "#8A6200"
		}
		main, paren := splitParen(st.Name)
		dy := cy + 32
		for _, line := range wrap(main, nameChars) {
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="%s" font-weight="%s">%s</text>`, cx, dy, nameFill, nameWeight, html.EscapeString(line))
			dy += 17
		}
		if paren != "" {
			// 괄호 안의 커맨드·스킬 이름은 작은 글씨 줄로 내린다.
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="%s" font-size="11" class="paren">%s</text>`, cx, dy, subFill, html.EscapeString(shorten(paren, nameChars+4)))
			dy += 15
		}
		sub, stale := subline(st, p.upstream, in)
		if stale != "" {
			// 확인일은 굵은 빨강, 「N일 지남」은 연빨강 칩으로 강조한다.
			w1, w2 := textWidth(sub), textWidth(stale)+10
			total := w1 + 4 + w2
			left := cx - total/2
			fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#B5252C" font-weight="700" font-size="11" class="stale">%s</text>`, left, dy, html.EscapeString(sub))
			fmt.Fprintf(&b, `<rect x="%d" y="%d" width="%d" height="16" rx="3" fill="#FDE8E9"/><text x="%d" y="%d" fill="#B5252C" font-weight="700" font-size="11" class="stale">%s</text>`,
				left+w1+4, dy-12, w2, left+w1+9, dy, html.EscapeString(stale))
		} else if sub != "" {
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="%s" font-size="11">%s</text>`, cx, dy, subFill, html.EscapeString(sub))
		}
		if n := in.Unresolved[st.ID]; n > 0 {
			color := "#8A919A"
			if st.Status == store.StepBlocked {
				color = "#D9363E"
			}
			fmt.Fprintf(&b, `<a href="/issues?step=%d"><circle cx="%d" cy="%d" r="9" fill="%s"/><text x="%d" y="%d" text-anchor="middle" fill="#fff" font-size="11" font-weight="700">%d</text></a>`,
				st.ID, cx+15, cy-15, color, cx+15, cy-11, n)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// segment는 스텝 p에 이르는 구간을 그린다. 상류 차단이면 연하게 칠하고 흰 빗금을 덧그린다.
func segment(b *strings.Builder, d string, p *placed) {
	color := strokeColor(p.node.Status)
	if p.node.Status == store.StepUnchecked {
		fmt.Fprintf(b, `<path d="%s" fill="none" stroke="#B9BEC5" stroke-width="%d" stroke-linecap="round"/><path d="%s" fill="none" stroke="#FFFFFF" stroke-width="%d" stroke-linecap="round"/>`, d, lineW, d, lineW-3)
		return
	}
	if p.upstream {
		fmt.Fprintf(b, `<path d="%s" fill="none" stroke="%s" stroke-width="%d" stroke-linecap="round" opacity="0.55"/><path d="%s" fill="none" stroke="url(#hatch)" stroke-width="%d" stroke-linecap="round" class="hatch"/>`, d, color, lineW, d, lineW)
		return
	}
	fmt.Fprintf(b, `<path d="%s" fill="none" stroke="%s" stroke-width="%d" stroke-linecap="round"/>`, d, color, lineW)
}

// subline은 스텝 이름 아래 줄의 글과, 확인일이 오래되었을 때의 「N일 지남」 글을 돌려준다.
func subline(st *store.StepNode, upstream bool, in Input) (string, string) {
	var parts []string
	if upstream {
		parts = append(parts, "상류 차단")
	}
	if st.Status == store.StepBlocked || st.Status == store.StepNeedsTest || st.Status == store.StepNotImplemented {
		parts = append(parts, store.StepStatusLabel[st.Status])
	}
	if st.CheckedAt.Valid {
		t := st.CheckedTime()
		// 확인 패키지가 있으면 그것을, 없으면 확인일을 보여 준다.
		checked := "확인 " + t.Format("01-02")
		if st.CheckedPackage != "" {
			checked = "확인 " + st.CheckedPackage
		}
		days := int(in.Now.Sub(t).Hours() / 24)
		if st.Status == store.StepSupported && !upstream && days >= in.StaleDays {
			return checked, fmt.Sprintf("%d일 지남", days)
		}
		parts = append(parts, checked)
	} else if len(parts) == 0 && st.Status == store.StepUnchecked {
		parts = append(parts, "미확인")
	}
	return strings.Join(parts, " · "), ""
}

// sameList는 두 스텝이 같은 본선 또는 같은 분기에 속하는지 본다.
func sameList(a, b *placed) bool {
	return a.node.ParentBranchID == b.node.ParentBranchID
}

var parenSuffix = regexp.MustCompile(`^(.*\S)\s*(\([^()]*\))$`)

// splitParen은 이름 끝의 괄호 부분(커맨드·스킬 이름)을 떼어 낸다. "분석 실행 (lctpa)" → "분석 실행", "(lctpa)".
func splitParen(name string) (string, string) {
	if m := parenSuffix.FindStringSubmatch(name); m != nil {
		return m[1], m[2]
	}
	return name, ""
}

// textWidth는 글 너비를 어림한다. 한글은 11px, 그 밖의 글자는 6.5px로 본다.
func textWidth(s string) int {
	w := 0.0
	for _, r := range s {
		if r > 0x2E80 {
			w += 11
		} else {
			w += 6.5
		}
	}
	return int(w)
}

// wrap은 글을 최대 두 줄로 나눈다. n글자 안에 띄어쓰기가 있으면 거기서 끊고, 없으면 n글자에서 끊는다.
// 두 줄을 넘는 부분은 줄임표로 줄인다.
func wrap(s string, n int) []string {
	r := []rune(s)
	if len(r) <= n {
		return []string{s}
	}
	cut, next := n, n
	for i := n; i > 0; i-- {
		if r[i] == ' ' {
			cut, next = i, i+1
			break
		}
	}
	return []string{string(r[:cut]), shorten(string(r[next:]), n)}
}

// shorten은 n글자를 넘는 글을 줄임표로 줄인다.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func ternary(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}
