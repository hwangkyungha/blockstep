// Package web은 HTTP 핸들러와 템플릿을 맡는다.
package web

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"strings"

	"github.com/hwangkyungha/blockstep/internal/store"
)

//go:embed templates/*.html
var templateFS embed.FS

var templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// Handler는 라우팅이 끝난 http.Handler를 돌려준다.
func Handler(s *store.Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		list, err := s.ListScenarios()
		if err != nil {
			serverError(w, err)
			return
		}
		if err := templates.ExecuteTemplate(w, "scenarios.html", map[string]any{"Scenarios": list}); err != nil {
			log.Println(err)
		}
	})
	mux.HandleFunc("POST /scenarios", func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimSpace(r.FormValue("name"))
		if name == "" {
			http.Error(w, "이름은 비울 수 없습니다", http.StatusBadRequest)
			return
		}
		if _, err := s.CreateScenario(name, strings.TrimSpace(r.FormValue("description"))); err != nil {
			serverError(w, err)
			return
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
	})
	return mux
}

func serverError(w http.ResponseWriter, err error) {
	log.Println(err)
	http.Error(w, "서버 오류", http.StatusInternalServerError)
}
