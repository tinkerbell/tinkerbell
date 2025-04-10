package ginutil_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	. "github.com/tinkerbell/hegel/internal/ginutil"
)

func TestTrailingSlashRouteHelper(t *testing.T) {
	cases := []struct {
		Name      string
		Endpoint  string
		Alternate string
	}{
		{
			Name:      "NoTrailingSlash",
			Endpoint:  "/foo",
			Alternate: "/foo/",
		},
		{
			Name:      "TrailingSlash",
			Endpoint:  "/foo/",
			Alternate: "/foo",
		},
	}

	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			router := TrailingSlashRouteHelper{gin.New()}

			// Create a variable that is the gin.Engine so we can call ServeHTTP on it as IRouter
			// doesn't have that API.
			servable := router.IRouter.(*gin.Engine)

			// Cache the total number of calls.
			var calls int

			// Configure the route. This should result in the alternate route being registered to.
			router.GET(tc.Endpoint, func(ctx *gin.Context) {
				calls++
				ctx.Writer.WriteHeader(http.StatusOK)
			})

			endpointRequest := httptest.NewRequest(http.MethodGet, tc.Endpoint, nil)
			endpointResponse := httptest.NewRecorder()

			servable.ServeHTTP(endpointResponse, endpointRequest)

			if endpointResponse.Code != http.StatusOK {
				t.Fatalf("Expected status code: %d; Received: %d", http.StatusOK, endpointResponse.Code)
			}

			if calls != 1 {
				t.Fatalf("Expected calls: 1; Received: %d", calls)
			}

			alternateRequest := httptest.NewRequest(http.MethodGet, tc.Alternate, nil)
			alternateResponse := httptest.NewRecorder()

			servable.ServeHTTP(alternateResponse, alternateRequest)

			if alternateResponse.Code != http.StatusOK {
				t.Fatalf("Expected status code: %d; Received: %d", http.StatusOK, alternateResponse.Code)
			}

			if calls != 2 {
				t.Fatalf("Expected calls: 2; Received: %d", calls)
			}
		})
	}
}
