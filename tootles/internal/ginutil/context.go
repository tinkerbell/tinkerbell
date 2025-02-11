package ginutil

import (
	"bufio"
	"net"
	"net/http"
	"net/http/httptest"
)

// FakeResponseWriter satisfies gin.ResponseWriter.
type FakeResponseWriter struct {
	*httptest.ResponseRecorder
}

func (w FakeResponseWriter) CloseNotify() <-chan bool {
	return nil
}

func (w FakeResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, nil
}

func (w FakeResponseWriter) Pusher() http.Pusher {
	return nil
}

func (w FakeResponseWriter) Size() int {
	return 0
}

func (w FakeResponseWriter) Status() int {
	return 0
}

func (w FakeResponseWriter) Written() bool {
	return false
}

func (w FakeResponseWriter) WriteHeaderNow() {
}
