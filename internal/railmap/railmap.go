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
	colW    = 150 // 스텝 사이 가로 간격
	rowH    = 120 // 본선과 분기 사이 세로 간격
	marginX = 50
	marginY = 40
	radius  = 11
)

// Color는 상태별 색이다. 미확인은 흰색 바탕에 테두리만 그린다.
var Color = map[string]string{
	store.StepSupported:      "#2E8B57",
	store.StepBlocked:        "#D9363E",
	store.StepNeedsTest:      "#E0A106",
	store.StepNotImplemented: "#B9BEC5",
	store.StepUnchecked:      "#FFFFFF",
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
	fmt.Fprintf(&b, `<svg class="railmap" xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="sans-serif" font-size="12">`, width, height, width, height)
	b.WriteString(`<defs><pattern id="hatch" width="6" height="6" patternUnits="userSpaceOnUse" patternTransform="rotate(45)"><line x1="0" y1="0" x2="0" y2="6" stroke="#555" stroke-width="1.5"/></pattern></defs>`)

	// 구간 선. 같은 행에서 앞 스텝과 잇는다. 행의 첫 스텝은 분기 선이 맡고, 본선의 첫 스텝은 짧은 시작 선을 둔다.
	prevInRow := map[cell]*placed{}
	for _, p := range l.steps {
		if prev, ok := prevInRow[cell{p.row, p.col - 1}]; ok && sameList(prev, p) {
			fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="8"/>`,
				x(prev.col), y(prev.row), x(p.col), y(p.row), lineColor(p))
		} else if p.row == 0 && p.col == 0 {
			fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="%s" stroke-width="8"/>`,
				x(0)-30, y(0), x(0), y(0), lineColor(p))
		}
		prevInRow[cell{p.row, p.col}] = p
	}
	for _, br := range l.branches {
		fx, fy := x(br.from.col), y(br.from.row)
		tx, ty := x(br.first.col), y(br.first.row)
		fmt.Fprintf(&b, `<path d="M%d %d V%d Q%d %d %d %d H%d" fill="none" stroke="%s" stroke-width="8" stroke-linejoin="round"/>`,
			fx, fy, ty-16, fx, ty, fx+16, ty, tx, lineColor(br.first))
		fmt.Fprintf(&b, `<text x="%d" y="%d" fill="#444" font-size="11">%s</text>`,
			fx+8, fy+78+br.index*14, html.EscapeString(br.label))
	}

	for _, p := range l.steps {
		st := p.node
		cx, cy := x(p.col), y(p.row)
		fill, stroke := Color[st.Status], "#333"
		if st.Status == store.StepUnchecked {
			stroke = "#888"
		}
		fmt.Fprintf(&b, `<g class="step step-%s%s" data-step-id="%d"><title>%s</title>`, st.Status, ternary(p.upstream, " upstream-blocked", ""), st.ID, html.EscapeString(st.Name))
		if p.upstream {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" fill-opacity="0.35" stroke="%s" stroke-width="2"/>`, cx, cy, radius, fill, stroke)
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="url(#hatch)" stroke="none"/>`, cx, cy, radius)
		} else {
			fmt.Fprintf(&b, `<circle cx="%d" cy="%d" r="%d" fill="%s" stroke="%s" stroke-width="2"/>`, cx, cy, radius, fill, stroke)
		}
		lines := wrap(st.Name, 11)
		for i, line := range lines {
			fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#222">%s</text>`, cx, cy+28+i*14, html.EscapeString(line))
		}
		if st.CheckedAt.Valid {
			dy := cy + 30 + len(lines)*14
			t := st.CheckedTime()
			days := int(in.Now.Sub(t).Hours() / 24)
			if st.Status == store.StepSupported && days >= in.StaleDays {
				fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#D9363E" font-weight="bold" font-size="11" class="stale">%s · %d일 지남</text>`,
					cx, dy, t.Format("2006-01-02"), days)
			} else {
				fmt.Fprintf(&b, `<text x="%d" y="%d" text-anchor="middle" fill="#666" font-size="11">%s</text>`, cx, dy, t.Format("2006-01-02"))
			}
		}
		if n := in.Unresolved[st.ID]; n > 0 {
			color := "#8A8F98"
			if st.Status == store.StepBlocked {
				color = "#D9363E"
			}
			fmt.Fprintf(&b, `<a href="/issues?step=%d"><circle cx="%d" cy="%d" r="9" fill="%s"/><text x="%d" y="%d" text-anchor="middle" fill="#fff" font-size="11" font-weight="bold">%d</text></a>`,
				st.ID, cx+13, cy-13, color, cx+13, cy-9, n)
		}
		b.WriteString(`</g>`)
	}
	b.WriteString(`</svg>`)
	return template.HTML(b.String())
}

// sameList는 두 스텝이 같은 본선 또는 같은 분기에 속하는지 본다.
func sameList(a, b *placed) bool {
	return a.node.ParentBranchID == b.node.ParentBranchID
}

// lineColor는 스텝에 이르는 구간의 색이다. 미확인은 흰색이라 선에는 회색을 쓴다.
func lineColor(p *placed) string {
	if p.node.Status == store.StepUnchecked {
		return "#D5D8DC"
	}
	return Color[p.node.Status]
}

// wrap은 이름을 n글자씩 최대 두 줄로 나눈다. 두 줄을 넘는 부분은 줄임표로 줄인다.
func wrap(s string, n int) []string {
	r := []rune(s)
	if len(r) <= n {
		return []string{s}
	}
	rest := r[n:]
	if len(rest) > n {
		rest = append(rest[:n-1:n-1], '…')
	}
	return []string{string(r[:n]), string(rest)}
}

func ternary(c bool, a, b string) string {
	if c {
		return a
	}
	return b
}
