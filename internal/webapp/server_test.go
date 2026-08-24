package webapp

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aleksandergregersen/opsquest/internal/game"
)

func startTestServer(t *testing.T) *Server {
	t.Helper()
	server, err := Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeContext, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Close(closeContext); err != nil {
			t.Errorf("close companion: %v", err)
		}
	})
	return server
}

func pairedClient(t *testing.T, server *Server) (*http.Client, string) {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	response, err := client.Get(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || response.Request.URL.Path != "/" {
		t.Fatalf("pairing response = %d at %s", response.StatusCode, response.Request.URL)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "OpsQuest Mission Companion") {
		t.Fatalf("paired index missing companion title: %s", body)
	}
	parsed, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	return client, parsed.Scheme + "://" + parsed.Host
}

func TestServerRequiresOneTimePairingAndSetsSecurityHeaders(t *testing.T) {
	server := startTestServer(t)
	parsed, err := url.Parse(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	baseURL := parsed.Scheme + "://" + parsed.Host

	response, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unpaired status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
	response, err = http.Get(baseURL + "/pair?token=wrong")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid pairing status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}

	client, _ := pairedClient(t, server)
	response, err = client.Get(baseURL + "/app.css")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/css") {
		t.Fatalf("authorized CSS response = %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	for _, header := range []string{"Content-Security-Policy", "Referrer-Policy", "X-Content-Type-Options", "X-Frame-Options"} {
		if response.Header.Get(header) == "" {
			t.Errorf("authorized response missing %s", header)
		}
	}

	secondResponse, err := http.Get(server.URL())
	if err != nil {
		t.Fatal(err)
	}
	secondResponse.Body.Close()
	if secondResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("reused pairing URL status = %d, want %d", secondResponse.StatusCode, http.StatusUnauthorized)
	}
}

func TestServerPublishesCurrentStateAndLiveEvents(t *testing.T) {
	server := startTestServer(t)
	client, baseURL := pairedClient(t, server)

	response, err := client.Get(baseURL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("empty state status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	started := game.AttemptEvent{
		Type: game.AttemptStarted,
		Snapshot: game.AttemptSnapshot{
			MissionID:         "linux-orientation",
			Number:            1,
			Title:             "Where Am I?",
			Objective:         "Print the current path.",
			SuggestedCommands: []string{"pwd"},
			Outcomes:          []game.AttemptOutcome{{Description: "Output matches", Satisfied: false}},
			State:             game.AttemptStateActive,
		},
	}
	server.ReportAttempt(started)
	started.Snapshot.SuggestedCommands[0] = "changed-after-publish"
	started.Snapshot.Outcomes[0].Description = "changed-after-publish"

	response, err = client.Get(baseURL + "/api/state")
	if err != nil {
		t.Fatal(err)
	}
	var current game.AttemptEvent
	if err := json.NewDecoder(response.Body).Decode(&current); err != nil {
		response.Body.Close()
		t.Fatal(err)
	}
	response.Body.Close()
	if current.Snapshot.SuggestedCommands[0] != "pwd" || current.Snapshot.Outcomes[0].Description != "Output matches" {
		t.Fatalf("server retained caller-owned slices: %#v", current)
	}
	if response.Header.Get("X-OpsQuest-Event-ID") != "1" {
		t.Fatalf("state event ID = %q, want 1", response.Header.Get("X-OpsQuest-Event-ID"))
	}

	streamContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(streamContext, http.MethodGet, baseURL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	reader := bufio.NewReader(stream.Body)
	first := readSSEEvent(t, reader)
	if !strings.Contains(first, `"type":"attempt_started"`) || !strings.Contains(first, "id: 1") {
		t.Fatalf("initial SSE event = %q", first)
	}

	completed := current
	completed.Type = game.AttemptCompleted
	completed.Snapshot.State = game.AttemptStateCompleted
	completed.Snapshot.Outcomes[0].Satisfied = true
	completed.Snapshot.SatisfiedOutcomes = 1
	server.ReportAttempt(completed)
	second := readSSEEvent(t, reader)
	if !strings.Contains(second, `"type":"attempt_completed"`) || !strings.Contains(second, "id: 2") {
		t.Fatalf("live SSE event = %q", second)
	}
}

func TestServerRejectsConfusedHostsOriginsAndMethods(t *testing.T) {
	server := startTestServer(t)
	client, baseURL := pairedClient(t, server)

	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Host = "attacker.example"
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMisdirectedRequest {
		t.Fatalf("confused Host status = %d, want %d", response.StatusCode, http.StatusMisdirectedRequest)
	}

	request, err = http.NewRequest(http.MethodGet, baseURL+"/api/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", "https://attacker.example")
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-origin status = %d, want %d", response.StatusCode, http.StatusForbidden)
	}

	request, err = http.NewRequest(http.MethodPost, baseURL+"/api/state", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusMethodNotAllowed || response.Header.Get("Allow") != http.MethodGet {
		t.Fatalf("POST state response = %d Allow %q", response.StatusCode, response.Header.Get("Allow"))
	}
}

func TestServerClosesWhenApplicationContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server, err := Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-server.done:
	case <-time.After(3 * time.Second):
		t.Fatal("companion did not close after application context cancellation")
	}
}

func TestEmbeddedAssetsStaySelfContainedAndUseTextOnlyRendering(t *testing.T) {
	html, err := staticFiles.ReadFile("static/index.html")
	if err != nil {
		t.Fatal(err)
	}
	javascript, err := staticFiles.ReadFile("static/app.js")
	if err != nil {
		t.Fatal(err)
	}
	stylesheet, err := staticFiles.ReadFile("static/app.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"http://", "https://", "style="} {
		if strings.Contains(string(html), forbidden) || strings.Contains(string(stylesheet), forbidden) {
			t.Errorf("embedded browser assets contain forbidden external or inline value %q", forbidden)
		}
	}
	for _, forbidden := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", ".style."} {
		if strings.Contains(string(javascript), forbidden) {
			t.Errorf("companion JavaScript uses forbidden rendering sink %q", forbidden)
		}
	}
	if !strings.Contains(string(javascript), "textContent") {
		t.Error("companion JavaScript does not use text-only DOM rendering")
	}
}

func readSSEEvent(t *testing.T, reader *bufio.Reader) string {
	t.Helper()
	var lines []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read SSE event: %v", err)
		}
		line = strings.TrimSuffix(line, "\n")
		if line == "" {
			return strings.Join(lines, "\n")
		}
		lines = append(lines, line)
	}
}
