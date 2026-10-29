package store

// 스텝 상태 값. 설계안 5.1의 다섯 가지다.
const (
	StepSupported      = "supported"
	StepBlocked        = "blocked"
	StepNeedsTest      = "needs_test"
	StepNotImplemented = "not_implemented"
	StepUnchecked      = "unchecked"
)

// StepStatuses는 화면에 보여 주는 순서다.
var StepStatuses = []string{StepSupported, StepBlocked, StepNeedsTest, StepNotImplemented, StepUnchecked}

// StepStatusLabel은 스텝 상태의 한국어 이름이다.
var StepStatusLabel = map[string]string{
	StepSupported:      "지원",
	StepBlocked:        "블록",
	StepNeedsTest:      "테스트 필요",
	StepNotImplemented: "미구현",
	StepUnchecked:      "미확인",
}

// 이슈 처리 상태 값. 설계안 5.2의 흐름이다.
const (
	IssueOpen           = "open"
	IssueAssigned       = "assigned"
	IssueInProgress     = "in_progress"
	IssueWaitingPackage = "waiting_package"
	IssueFixed          = "fixed"
	IssueVerified       = "verified"
	IssueOnHold         = "on_hold"
	IssueRejected       = "rejected"
)

// IssueStatuses는 화면에 보여 주는 순서다.
var IssueStatuses = []string{IssueOpen, IssueAssigned, IssueInProgress, IssueWaitingPackage, IssueFixed, IssueVerified, IssueOnHold, IssueRejected}

// IssueStatusLabel은 이슈 처리 상태의 한국어 이름이다.
var IssueStatusLabel = map[string]string{
	IssueOpen:           "등록",
	IssueAssigned:       "담당자 배정",
	IssueInProgress:     "처리 중",
	IssueWaitingPackage: "반영 대기",
	IssueFixed:          "조치 완료",
	IssueVerified:       "검증 완료",
	IssueOnHold:         "보류",
	IssueRejected:       "거절",
}

// unresolvedCondition은 미해결 이슈를 고르는 SQL 조건이다. 조치 완료, 검증 완료, 거절은 해결로 센다.
const unresolvedCondition = "i.status NOT IN ('fixed', 'verified', 'rejected')"

// ValidationError는 입력이 규칙에 맞지 않을 때 돌려주는 오류다. 화면에는 그 문구를 그대로 보여 준다.
type ValidationError string

func (e ValidationError) Error() string { return string(e) }
