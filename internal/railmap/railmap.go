// Package railmap은 시나리오 하나를 지하철 노선도 모양의 SVG로 그린다.
//
// 본선은 가로 선 하나이고 스텝은 선 위의 둥근 표시다. 스텝 사이 구간에는 그 구간이 이르는
// 스텝의 상태 색을 칠한다. 분기는 시작 스텝에서 아래로 내려가 가로로 이어지는 선이다.
// 상류에 블록 스텝이 있는 스텝은 연한 색에 빗금을 덧그린다(상류 차단). 이 표시는 저장하지 않고 계산한다.
package railmap

import (
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"

	"github.com/hwangkyungha/blockstep/internal/store"
)

const (
	colW    = 180 // 스텝 사이 가로 간격
	rowH    = 130 // 본선과 분기 사이 세로 간격
	marginX = 110
	marginY = 50
	radius  = 12
	lineW   = 10
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
}

type branchLine struct {
	from  *placed
	first *placed
	label string
	index int // 같은 스텝에서 몇 번째 분기인지
}

type layout struct {
	// occupied는 칸을 쓰는 주체다. 스텝이 있는 칸은 0, 분기의 꺾이는 칸과 내려가는 선이 지나는 칸은 분기 시작 스텝의 id다.
	occupied map[cell]int64
	steps    []*placed
	branches []branchLine
	maxRow   int
	maxCol   int
}

func (l *layout) free(c cell, owner int64) bool {
	o, ok := l.occupied[c]
	return !ok || (owner != 0 && o == owner)
}

// place는 steps를 row 행의 startCol 열부터 차례로 놓고, 각 스텝의 분기를 아래 행에 놓는다.
func (l *layout) place(steps []store.StepNode, row, startCol int, upstream bool) {
	var ps []*placed
	for i := range steps {
		n := &steps[i]
		p := &placed{node: n, row: row, col: startCol + i, upstream: upstream}
		l.occupied[cell{row, p.col}] = 0
		l.steps = append(l.steps, p)
		ps = append(ps, p)
		if n.Status == store.StepBlocked {
			upstream = true
		}
		if p.col > l.maxCol {
			l.maxCol = p.col
		}
	}
	// 뒤쪽 스텝의 분기를 먼저 놓아 본선에 가까운 행을 차지하게 한다.
	// 앞쪽 스텝의 내려가는 선은 뒤쪽 분기가 쓰는 열보다 왼쪽에 있어 서로 겹치지 않는다.
	for i := len(ps) - 1; i >= 0; i-- {
		p := ps[i]
		for k := range p.node.Branches {
			br := &p.node.Branches[k]
			owner := p.node.ID
			r := row + 1
			for !l.fits(r, row, p.col, len(br.Steps), owner) {
				r++
			}
			for rr := row + 1; rr <= r; rr++ {
				l.occupied[cell{rr, p.col}] = owner
			}
			if r > l.maxRow {
				l.maxRow = r
			}
			before := len(l.steps)
			l.place(br.Steps, r, p.col+1, p.upstream || l.blockedUpTo(ps, i))
			l.branches = append(l.branches, branchLine{from: p, first: l.steps[before], label: br.Label, index: k})
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

// fits는 분기를 r 행에 놓을 수 있는지 본다. 내려가는 선이 지나는 칸, 꺾이는 칸, 분기 스텝이 놓일 칸을 본다.
func (l *layout) fits(r, parentRow, col, n int, owner int64) bool {
	for rr := parentRow + 1; rr <= r; rr++ {
		if !l.free(cell{rr, col}, owner) {
			return false
		}
	}
	for c := col + 1; c <= col+n; c++ {
		if !l.free(cell{r, c}, 0) {
			return false
		}
	}
	return true
}

func x(col int) int { return marginX + col*colW }
func y(row int) int { return marginY + row*rowH }

// Render는 노선도 SVG를 돌려준다. 스텝이 없으면 빈 문자열이다.
func Render(in Input) template.HTML {
	if len(in.Tree) == 0 {
		return ""
	}
	l := &layout{occupied: map[cell]int64{}}
	l.place(in.Tree, 0, 0, false)

	var b strings.Builder
	width := x(l.maxCol) + colW
	height := y(l.maxRow) + rowH
	fmt.Fprintf(&b, `<svg class="railmap" xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="'Noto Sans KR', system-ui, sans-serif" font-size="13">`, width, height, width, height)
	b.WriteString(`<defs><pattern id="hatch" width="8" height="8" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><line x1="0" y1="0" x2="0" y2="8" stroke="#FFFFFF" stroke-width="4"/></pattern></defs>`)

	// 구간 선. 같은 행에서 앞 스텝과 잇는다. 행의 첫 스텝은 분기 선이 맡고, 본선의 첫 스텝은 짧은 시작 선을 둔다.
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
		fx, fy := x(br.from.col), y(br.from.row)
		tx, ty := x(br.first.col), y(br.first.row)
		segment(&b, fmt.Sprintf(`M%d %d V%d Q%d %d %d %d H%d`, fx, fy, ty-18, fx, ty, fx+18, ty, tx), br.first)
		label := "? " + br.label
		lw := textWidth(label) + 16
		ly := fy + 82 + br.index*24
		fmt.Fprintf(&b, `<g class="branch-label"><rect x="%d" y="%d" width="%d" height="20" rx="4" fill="#FFFFFF" stroke="#D0D4DA"/><text x="%d" y="%d" fill="#444B55" font-size="12">%s</text></g>`,
			fx+12, ly, lw, fx+20, ly+14, html.EscapeString(label))
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

		nameFill, nameWeight := "#1F2328", "500"
		switch {
		case p.upstream:
			nameFill = "#8A919A"
		case st.Status == store.StepBlocked:
			nameFill, nameWeight = "#B5252C", "700"
		case st.Status == store.StepNeedsTest:
			nameFill = "#8A6200"
		}
		lines := wrap(st.Name, 12)
		for i, line := range lines {
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="%s" font-weight="%s">%s</text>`, cx, cy+32+i*17, nameFill, nameWeight, html.EscapeString(line))
		}
		dy := cy + 32 + len(lines)*17
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
			subFill := "#5A616B"
			switch {
			case p.upstream:
				subFill = "#8A919A"
			case st.Status == store.StepBlocked:
				subFill = "#B5252C"
			case st.Status == store.StepNeedsTest:
				subFill = "#8A6200"
			}
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

// wrap은 이름을 최대 두 줄로 나눈다. n글자 안에 띄어쓰기가 있으면 거기서 끊고, 없으면 n글자에서 끊는다.
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
	rest := r[next:]
	if len(rest) > n {
		rest = append(rest[:n-1:n-1], '…')
	}
	return []string{string(r[:cut]), string(rest)}
}

func ternary(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}
