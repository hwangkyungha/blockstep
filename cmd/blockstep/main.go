// blockstep은 lct 사용자 시나리오의 진행 상황과 막힌 이슈를 보여 주는 웹 도구다.
package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/hwangkyungha/blockstep/internal/store"
	"github.com/hwangkyungha/blockstep/internal/web"
)

func main() {
	dbPath := flag.String("db", "blockstep.db", "SQLite 파일 경로")
	addr := flag.String("addr", ":8080", "HTTP 주소")
	flag.Parse()

	s, err := store.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer s.Close()

	log.Printf("Blockstep 시작: http://%s (db: %s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, web.Handler(s)))
}
