package web

import (
	"fmt"
	"log"
	"strings"

	"github.com/hwangkyungha/blockstep/internal/notify"
	"github.com/hwangkyungha/blockstep/internal/store"
)

// notifyNewIssue는 블록 스텝에 연결된 이슈가 새로 등록되었을 때 Slack에 알린다.
func (srv *server) notifyNewIssue(id int64, res store.AutoResult) {
	if len(res.BlockedSteps) == 0 {
		return
	}
	issue, err := srv.store.GetIssue(id)
	if err != nil {
		log.Println(err)
		return
	}
	var names []string
	for _, st := range res.BlockedSteps {
		names = append(names, st.Name)
	}
	srv.sendSlack(id, fmt.Sprintf("[Blockstep] 블록 스텝에 이슈 등록: #%d %s\n막힌 스텝: %s\n긴급도: %s",
		id, issue.Title, strings.Join(names, ", "), issue.Priority))
}

// notifyIssueUpdate는 이슈가 조치 완료로 바뀌었을 때 Slack에 알린다.
func (srv *server) notifyIssueUpdate(id int64, oldStatus string, res store.AutoResult) {
	issue, err := srv.store.GetIssue(id)
	if err != nil {
		log.Println(err)
		return
	}
	if issue.Status != store.IssueFixed || oldStatus == store.IssueFixed {
		return
	}
	released := "없음"
	if len(res.NeedsTest) > 0 {
		var names []string
		for _, st := range res.NeedsTest {
			names = append(names, st.Name)
		}
		released = strings.Join(names, ", ")
	}
	srv.sendSlack(id, fmt.Sprintf("[Blockstep] 이슈 조치 완료: #%d %s\n반영 패키지: %s\n처리 후 내용: %s\n테스트 필요로 바뀐 스텝: %s",
		id, issue.Title, issue.AppliedPackage, issue.ResolutionNote, released))
}

// sendSlack은 설정에 웹훅 URL이 있을 때만 보낸다. 실패해도 저장은 그대로 두고 변경 이력에 남긴다.
func (srv *server) sendSlack(issueID int64, text string) {
	url, err := srv.store.Setting("slack_webhook_url")
	if err != nil {
		log.Println(err)
		return
	}
	if url == "" {
		return
	}
	if err := notify.Slack(url, text); err != nil {
		log.Printf("slack 전송 실패 (이슈 #%d): %v", issueID, err)
		if err := srv.store.LogNotifyFailure(issueID, err); err != nil {
			log.Println(err)
		}
	}
}
