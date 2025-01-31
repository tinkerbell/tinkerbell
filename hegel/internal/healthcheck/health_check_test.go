package healthcheck_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	gomock "github.com/golang/mock/gomock"
	"github.com/tinkerbell/tinkerbell/hegel/internal/ginutil"
	"github.com/tinkerbell/tinkerbell/hegel/internal/healthcheck"
)

func TestHealthCheck(t *testing.T) {
	cases := []struct {
		Name         string
		ExpectedCode int
		GetClient    func(*gomock.Controller) healthcheck.Client
	}{
		{
			Name:         "ClientIsHealthy",
			ExpectedCode: http.StatusOK,
			GetClient: func(ctrl *gomock.Controller) healthcheck.Client {
				client := healthcheck.NewMockClient(ctrl)
				client.EXPECT().IsHealthy(gomock.Any()).Return(true)
				return client
			},
		},
		{
			Name:         "ClientIsUnhealthy",
			ExpectedCode: http.StatusInternalServerError,
			GetClient: func(ctrl *gomock.Controller) healthcheck.Client {
				client := healthcheck.NewMockClient(ctrl)
				client.EXPECT().IsHealthy(gomock.Any()).Return(false)
				return client
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			ctrl := gomock.NewController(t)

			client := tc.GetClient(ctrl)

			w := ginutil.FakeResponseWriter{ResponseRecorder: httptest.NewRecorder()}
			ctx := &gin.Context{Writer: w}

			handler := healthcheck.NewHandler(client)

			handler(ctx)

			if w.Code != tc.ExpectedCode {
				t.Fatalf("Expected status code: %d; Received status code: %d", tc.ExpectedCode, w.Code)
			}
		})
	}
}
