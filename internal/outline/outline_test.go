package outline

import (
	"reflect"
	"strings"
	"testing"
)

const sample = `1. lct install 실행
2. 에이전트 선택
3. 라이선스 확인 [install-SCN-03]
   ? 라이선스 없음
     3a. 평가판 라이선스 안내
     3b. 평가판 모드로 진입
       ? 네트워크 없음
         오프라인 안내
4. lct config ct
`

func TestParse(t *testing.T) {
	got, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{
		{Name: "lct install 실행"},
		{Name: "에이전트 선택"},
		{Name: "라이선스 확인", Refs: "install-SCN-03", Branches: []Branch{{
			Label: "라이선스 없음",
			Steps: []Step{
				{Name: "평가판 라이선스 안내"},
				{Name: "평가판 모드로 진입", Branches: []Branch{{
					Label: "네트워크 없음",
					Steps: []Step{{Name: "오프라인 안내"}},
				}}},
			},
		}}},
		{Name: "lct config ct"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestFormatRoundTrip(t *testing.T) {
	steps, err := Parse(sample)
	if err != nil {
		t.Fatal(err)
	}
	text := Format(steps)
	again, err := Parse(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	if !reflect.DeepEqual(steps, again) {
		t.Errorf("되돌린 결과가 다르다:\n%s", text)
	}
	if !strings.Contains(text, "라이선스 확인 [install-SCN-03]\n  ? 라이선스 없음\n    평가판 라이선스 안내\n") {
		t.Errorf("형식이 다르다:\n%s", text)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"? 조건\n  스텝":                 "1번째 줄",
		"스텝\n  들여쓴 스텝":               "2번째 줄",
		"스텝\n  ? 조건\n다음":             "2번째 줄",
		"스텝\n  ?\n    하위":            "2번째 줄",
		"스텝\n  ? 조건\n    하위\n   어긋남": "4번째 줄",
	}
	for in, want := range cases {
		_, err := Parse(in)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: err = %v, want %s", in, err, want)
		}
	}
	if steps, err := Parse("  \n\n"); err != nil || steps != nil {
		t.Errorf("빈 입력: %v %v", steps, err)
	}
}
