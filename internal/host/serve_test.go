package host

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A request without a value, the GET of an event stream above all, has no
// body: through the reverse proxy of features/editcli a GET with one lost its
// stream now and then (issues.md, 2026-11-26).
func TestServedCLIRequestWithoutValueHasNoBody(t *testing.T) {
	got := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		got <- fmt.Sprint(r.Method, " ", r.URL.Path, " ", r.ContentLength, " ", string(data))
	}))
	defer server.Close()
	qwen, openCode := &qwenProcess{base: server.URL}, &openCodeProcess{base: server.URL}
	for path, request := range map[string]func(string, string, any) (*http.Response, error){"/events": qwen.request, "/event": openCode.request} {
		res, err := request(http.MethodGet, path, nil)
		codexNoErr(t, err)
		res.Body.Close()
		codexEqual(t, <-got, "GET "+path+" 0 ")
	}
	res, err := qwen.request(http.MethodPost, "/cancel", struct{}{})
	codexNoErr(t, err)
	res.Body.Close()
	codexEqual(t, <-got, "POST /cancel 2 {}")
}
