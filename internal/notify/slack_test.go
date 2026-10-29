package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSlack(t *testing.T) {
	var got map[string]string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		json.Unmarshal(b, &got)
	}))
	defer ts.Close()
	if err := Slack(ts.URL, "안녕"); err != nil {
		t.Fatal(err)
	}
	if got["text"] != "안녕" {
		t.Errorf("body = %v", got)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer bad.Close()
	if err := Slack(bad.URL, "x"); err == nil {
		t.Error("500인데 오류가 없다")
	}
}
