//go:build container

package acceptance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestContainerKnowsIANAZone(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed; container acceptance requires a Docker daemon")
	}
	if output, err := exec.Command("docker", "info").CombinedOutput(); err != nil {
		t.Fatalf("docker daemon is not available: %v\n%s", err, output)
	}

	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}

	image := fmt.Sprintf("temper-timezone:%d", time.Now().UnixNano())
	buildxConfig := tempDirectory(t, repository)
	build := exec.Command("docker", "build", "--network", "host", "--platform", "linux/amd64", "-t", image, ".")
	build.Dir = repository
	build.Env = append(withoutEnvironment(os.Environ(), "BUILDX_CONFIG"), "BUILDX_CONFIG="+buildxConfig)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("docker build: %v\n%s", err, output)
	}
	t.Cleanup(func() {
		_ = exec.Command("docker", "image", "rm", "-f", image).Run()
	})

	dataDir := tempDirectory(t, repository)
	if err := os.Chmod(dataDir, 0o777); err != nil {
		t.Fatalf("make data directory writable: %v", err)
	}

	token := randomToken(t)
	port := freePort(t)
	name := fmt.Sprintf("temper-timezone-%d", time.Now().UnixNano())
	startContainer(t, name, image, dataDir, port, token)
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	waitForHealth(t, baseURL, name, 30*time.Second)

	body := `{"effective_from":"2026-11-16","timezone":"Europe/Madrid","weekly_frequency":1,"reason":"madrid timezone","sessions":[{"id":"madrid-run","weekday":1,"position":0,"discipline":"run","target":"30 min easy"}]}`
	request, err := http.NewRequest(http.MethodPost, baseURL+"/api/plans", strings.NewReader(body))
	if err != nil {
		t.Fatalf("build plan request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Temper-Token", token)
	request.Header.Set("Idempotency-Key", "madrid-timezone-1")

	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("create plan: %v", err)
	}
	created, readErr := io.ReadAll(response.Body)
	response.Body.Close()
	if readErr != nil {
		t.Fatalf("read plan response: %v", readErr)
	}
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create plan with Europe/Madrid = %d %s, want 201", response.StatusCode, created)
	}
	if !strings.Contains(string(created), `"timezone":"Europe/Madrid"`) {
		t.Fatalf("created plan does not echo the Madrid timezone: %s", created)
	}

	weekRequest, err := http.NewRequest(http.MethodGet, baseURL+"/api/week/2026-11-17", nil)
	if err != nil {
		t.Fatalf("build week request: %v", err)
	}
	weekRequest.Header.Set("X-Temper-Token", token)

	weekResponse, err := http.DefaultClient.Do(weekRequest)
	if err != nil {
		t.Fatalf("read week: %v", err)
	}
	weekBody, readErr := io.ReadAll(weekResponse.Body)
	weekResponse.Body.Close()
	if readErr != nil {
		t.Fatalf("read week response: %v", readErr)
	}
	if weekResponse.StatusCode != http.StatusOK {
		t.Fatalf("week = %d %s, want 200", weekResponse.StatusCode, weekBody)
	}

	var week struct {
		WeekStart string `json:"week_start"`
		WeekEnd   string `json:"week_end"`
		Planned   []struct {
			Date       string `json:"date"`
			TemplateID string `json:"template_id"`
		} `json:"planned"`
	}
	if err := json.Unmarshal(weekBody, &week); err != nil {
		t.Fatalf("decode week: %v\n%s", err, weekBody)
	}
	if week.WeekStart != "2026-11-16" || week.WeekEnd != "2026-11-22" {
		t.Fatalf("week = %s..%s, want 2026-11-16..2026-11-22", week.WeekStart, week.WeekEnd)
	}
	if len(week.Planned) != 1 || week.Planned[0].Date != "2026-11-16" || week.Planned[0].TemplateID != "madrid-run" {
		t.Fatalf("planned = %+v, want madrid-run on 2026-11-16", week.Planned)
	}

	logs, _ := exec.Command("docker", "logs", name).CombinedOutput()
	if strings.Contains(string(logs), "is not a known IANA name") {
		t.Fatalf("container rejected the Madrid timezone:\n%s", logs)
	}
}
