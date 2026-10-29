// Package notify는 Slack Incoming Webhook으로 메시지를 보낸다.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Slack은 webhookURL로 text를 보낸다. 2xx가 아니면 오류다.
func Slack(webhookURL, text string) error {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack 응답 %s", resp.Status)
	}
	return nil
}
