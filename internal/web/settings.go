package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/hwangkyungha/blockstep/internal/store"
)

func (srv *server) settingsPage(w http.ResponseWriter, r *http.Request) {
	staleDays, err := srv.store.StaleDays()
	if err != nil {
		fail(w, err)
		return
	}
	webhook, err := srv.store.Setting("slack_webhook_url")
	if err != nil {
		fail(w, err)
		return
	}
	srv.render(w, "settings.html", map[string]any{
		"StaleDays": staleDays, "SlackWebhookURL": webhook, "Saved": r.URL.Query().Get("saved") == "1",
	})
}

func (srv *server) saveSettings(w http.ResponseWriter, r *http.Request) {
	days, err := strconv.Atoi(strings.TrimSpace(r.FormValue("stale_days")))
	if err != nil || days < 0 {
		fail(w, store.ValidationError("확인일 경과 기준은 0 이상의 정수여야 합니다"))
		return
	}
	if err := srv.store.SetSetting("stale_days", strconv.Itoa(days)); err != nil {
		fail(w, err)
		return
	}
	if err := srv.store.SetSetting("slack_webhook_url", strings.TrimSpace(r.FormValue("slack_webhook_url"))); err != nil {
		fail(w, err)
		return
	}
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}
