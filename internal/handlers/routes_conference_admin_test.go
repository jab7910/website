package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
)

func TestConferenceAdminRecordingNotificationRoutes(t *testing.T) {
	router := mux.NewRouter()
	registerConferenceAdminRoutes(router, nil)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodGet, path: "/toronto/admin/recordings/notify-speakers"},
		{method: http.MethodPost, path: "/toronto/admin/recordings/notify-speakers"},
		{method: http.MethodPost, path: "/toronto/admin/recordings/notify-speakers/test"},
	}
	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, nil)
			var match mux.RouteMatch
			if !router.Match(request, &match) {
				t.Fatalf("route is not registered")
			}
			template, err := match.Route.GetPathTemplate()
			if err != nil {
				t.Fatalf("route path template: %v", err)
			}
			want := "/{conf}" + test.path[len("/toronto"):]
			if template != want {
				t.Fatalf("matched route template %q, want %q", template, want)
			}
		})
	}
}
