//go:build !windows

package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/double-cloth-studio/PageHub/backend/internal/lifecycle"
	"github.com/double-cloth-studio/PageHub/backend/internal/registry"
)

func TestPlatformJobSurvivesManagerRestart(t *testing.T) {
	store := registry.Store{Root: t.TempDir()}
	job := Job{ID: strings.Repeat("a", 32), AppID: "self", Action: "update-self", State: "running", Result: map[string]string{"request_id": "12345"}, CreatedAt: time.Now().UTC()}
	file := store.Path("state", "manager", "jobs", job.ID+".json")
	if err := registry.WriteJSON(file, job, 0600); err != nil {
		t.Fatal(err)
	}
	if err := registry.WriteJSON(store.Path("state", "self-results", "12345.json"), map[string]string{"state": "succeeded", "commit": strings.Repeat("b", 40)}, 0600); err != nil {
		t.Fatal(err)
	}
	socket := store.Path("runner.sock")
	listener, err := UnixListener(socket, 0600)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: RunnerHandler(lifecycle.Service{Store: store})}
	go server.Serve(listener)
	defer server.Close()
	token := store.Path("token")
	if err = os.WriteFile(token, []byte("test-only-token"), 0600); err != nil {
		t.Fatal(err)
	}
	m := Manager{Store: store, Runner: RunnerClient{Socket: socket}, TokenFile: token, Frontend: store.Root}
	handler := m.Admin()
	req := httptest.NewRequest("GET", "http://localhost/admin/api/jobs/"+job.ID, nil)
	req.Header.Set("Authorization", "Bearer test-only-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	var actual Job
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &actual) != nil || actual.State != "succeeded" {
		t.Fatal("重启后未续读独立平台更新收据", response.Body.String())
	}
}
