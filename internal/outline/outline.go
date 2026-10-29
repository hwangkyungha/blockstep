// Package outline은 들여쓰기 목록 텍스트와 스텝 트리를 서로 바꾼다.
//
//	lct install 실행
//	라이선스 확인 [install-SCN-01]
//	  ? 라이선스 없음
//	    평가판 안내
//	lct config ct
//
// 같은 깊이의 줄은 이어지는 스텝이다. 스텝 아래에 들여쓴 `? 조건` 줄이 분기를 시작하고,
// 그 아래에 더 들여쓴 줄이 분기의 스텝이다. 줄 끝의 `[참조]`는 refs 칸에 들어간다.
// 줄 앞의 `1.`, `3a.` 같은 번호는 무시한다.
package outline

import (
	"fmt"
	"regexp"
	"strings"
)

// Step은 트리의 스텝 하나다.
type Step struct {
	Name     string
	Refs     string
	Branches []Branch
}

// Branch는 스텝에서 갈라지는 분기 하나다.
type Branch struct {
	Label string
	Steps []Step
}

type line struct {
	no     int
	indent int
	text   string
}

var (
	numberPrefix = regexp.MustCompile(`^\d+[a-zA-Z]*[.)]\s+`)
	refsSuffix   = regexp.MustCompile(`\s*\[([^\]]*)\]$`)
)

// Parse는 아웃라인 텍스트를 스텝 트리로 바꾼다.
func Parse(text string) ([]Step, error) {
	var lines []line
	for i, raw := range strings.Split(text, "\n") {
		raw = strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(raw) == "" {
			continue
		}
		indent := 0
		for _, c := range raw {
			if c == ' ' {
				indent++
			} else if c == '\t' {
				indent += 4
			} else {
				break
			}
		}
		lines = append(lines, line{no: i + 1, indent: indent, text: strings.TrimLeft(raw, " \t")})
	}
	if len(lines) == 0 {
		return nil, nil
	}
	p := &parser{lines: lines}
	steps, err := p.steps(lines[0].indent)
	if err != nil {
		return nil, err
	}
	if p.i < len(lines) {
		return nil, p.errorf("들여쓰기가 맞지 않습니다")
	}
	return steps, nil
}

type parser struct {
	lines []line
	i     int
}

func (p *parser) errorf(format string, a ...any) error {
	return fmt.Errorf("%d번째 줄: %s", p.lines[p.i].no, fmt.Sprintf(format, a...))
}

func isBranch(l line) bool { return strings.HasPrefix(l.text, "?") }

// steps는 indent 깊이의 스텝 줄을 모은다. 더 깊은 줄은 분기로 읽는다.
func (p *parser) steps(indent int) ([]Step, error) {
	var steps []Step
	for p.i < len(p.lines) && p.lines[p.i].indent == indent {
		l := p.lines[p.i]
		if isBranch(l) {
			return nil, p.errorf("분기 줄은 스텝보다 들여써야 합니다")
		}
		st := Step{Name: numberPrefix.ReplaceAllString(l.text, "")}
		if m := refsSuffix.FindStringSubmatch(st.Name); m != nil {
			st.Refs = strings.TrimSpace(m[1])
			st.Name = strings.TrimSpace(st.Name[:len(st.Name)-len(m[0])])
		}
		if st.Name == "" {
			return nil, p.errorf("스텝 이름이 비어 있습니다")
		}
		p.i++
		for p.i < len(p.lines) && p.lines[p.i].indent > indent {
			bl := p.lines[p.i]
			if !isBranch(bl) {
				return nil, p.errorf("들여쓴 줄은 `? 조건`으로 시작해야 합니다")
			}
			br := Branch{Label: strings.TrimSpace(bl.text[1:])}
			if br.Label == "" {
				return nil, p.errorf("분기 조건이 비어 있습니다")
			}
			p.i++
			if p.i >= len(p.lines) || p.lines[p.i].indent <= bl.indent {
				p.i--
				return nil, p.errorf("분기 아래에 들여쓴 스텝이 없습니다")
			}
			sub, err := p.steps(p.lines[p.i].indent)
			if err != nil {
				return nil, err
			}
			if p.i < len(p.lines) && p.lines[p.i].indent > bl.indent {
				return nil, p.errorf("들여쓰기가 맞지 않습니다")
			}
			br.Steps = sub
			st.Branches = append(st.Branches, br)
		}
		steps = append(steps, st)
	}
	return steps, nil
}

// Format은 스텝 트리를 아웃라인 텍스트로 바꾼다. Parse와 서로 되돌릴 수 있다.
func Format(steps []Step) string {
	var b strings.Builder
	format(&b, steps, 0)
	return b.String()
}

func format(b *strings.Builder, steps []Step, depth int) {
	pad := strings.Repeat("  ", depth)
	for _, st := range steps {
		b.WriteString(pad + st.Name)
		if st.Refs != "" {
			b.WriteString(" [" + st.Refs + "]")
		}
		b.WriteString("\n")
		for _, br := range st.Branches {
			b.WriteString(pad + "  ? " + br.Label + "\n")
			format(b, br.Steps, depth+2)
		}
	}
}
