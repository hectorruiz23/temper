package acceptance

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

var binaryPath string

var listeningPattern = regexp.MustCompile(`^listening (127\.0\.0\.1:\d+)$`)

func TestMain(m *testing.M) {
	apiRoot, err := filepath.Abs("..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "resolve api root: %v\n", err)
		os.Exit(1)
	}

	directory, err := os.MkdirTemp("", "temper-acceptance-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temporary directory: %v\n", err)
		os.Exit(1)
	}

	binaryPath = filepath.Join(directory, "temper")
	build := exec.Command("go", "build", "-trimpath", "-o", binaryPath, "./cmd/temper")
	build.Dir = apiRoot
	build.Env = withoutEnvironment(os.Environ(), "CGO_ENABLED")
	build.Env = append(build.Env, "CGO_ENABLED=0")
	if output, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "build temper: %v\n%s", err, output)
		os.RemoveAll(directory)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(directory)
	os.Exit(code)
}

type lockedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

type process struct {
	cmd      *exec.Cmd
	stdout   *lockedBuffer
	stderr   *lockedBuffer
	exited   chan struct{}
	stopOnce sync.Once
	mu       sync.Mutex
	exitErr  error
}

func withoutEnvironment(environment []string, names ...string) []string {
	filtered := make([]string, 0, len(environment))
	for _, item := range environment {
		drop := false
		for _, name := range names {
			if strings.HasPrefix(item, name+"=") {
				drop = true
				break
			}
		}
		if !drop {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func temperEnvironment(dbPath, token, addr string) []string {
	environment := withoutEnvironment(os.Environ(), "TEMPER_TOKEN", "TEMPER_ADDR", "TEMPER_DB")
	return append(environment,
		"TEMPER_TOKEN="+token,
		"TEMPER_ADDR="+addr,
		"TEMPER_DB="+dbPath,
	)
}

func startProcess(t *testing.T, dbPath, token, addr string) *process {
	t.Helper()

	command := exec.Command(binaryPath)
	command.Env = temperEnvironment(dbPath, token, addr)

	p := &process{
		cmd:    command,
		stdout: &lockedBuffer{},
		stderr: &lockedBuffer{},
		exited: make(chan struct{}),
	}
	command.Stdout = p.stdout
	command.Stderr = p.stderr

	if err := command.Start(); err != nil {
		t.Fatalf("start temper: %v", err)
	}

	go func() {
		err := command.Wait()
		p.mu.Lock()
		p.exitErr = err
		p.mu.Unlock()
		close(p.exited)
	}()

	t.Cleanup(func() { p.stop(t) })

	return p
}

func (p *process) output() string {
	return fmt.Sprintf("stdout:\n%s\nstderr:\n%s", p.stdout.String(), p.stderr.String())
}

func (p *process) logs() string {
	return p.stdout.String() + p.stderr.String()
}

func (p *process) error() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.exitErr
}

func (p *process) waitListening(t *testing.T, timeout time.Duration) string {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if match := listeningPattern.FindStringSubmatch(strings.TrimSpace(p.stdout.String())); match != nil {
			return "http://" + match[1]
		}
		select {
		case <-p.exited:
			t.Fatalf("temper exited before listening: %v\n%s", p.error(), p.output())
		case <-time.After(10 * time.Millisecond):
		}
	}

	t.Fatalf("timed out waiting for the listening line\n%s", p.output())
	return ""
}

func (p *process) waitExit(t *testing.T, timeout time.Duration) error {
	t.Helper()

	select {
	case <-p.exited:
		return p.error()
	case <-time.After(timeout):
		t.Fatalf("temper did not exit within %s\n%s", timeout, p.output())
		return nil
	}
}

func (p *process) stop(t *testing.T) {
	p.stopOnce.Do(func() {
		if p.cmd.Process == nil {
			return
		}
		_ = p.cmd.Process.Signal(os.Interrupt)
		select {
		case <-p.exited:
		case <-time.After(5 * time.Second):
			_ = p.cmd.Process.Kill()
			select {
			case <-p.exited:
			case <-time.After(5 * time.Second):
				t.Errorf("temper did not stop after kill")
			}
		}
	})
}

func apiRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolve api root: %v", err)
	}
	return root
}

func openDatabase(t *testing.T, path string) *sql.DB {
	t.Helper()

	database, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatalf("open database %s: %v", path, err)
	}
	database.SetMaxOpenConns(1)
	if err := database.Ping(); err != nil {
		database.Close()
		t.Fatalf("ping database %s: %v", path, err)
	}
	t.Cleanup(func() { database.Close() })

	return database
}

type migrationRow struct {
	version   int
	appliedAt int64
	checksum  string
}

func readMigrations(t *testing.T, database *sql.DB) []migrationRow {
	t.Helper()

	rows, err := database.Query(`SELECT version, applied_at, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatalf("query schema_migrations: %v", err)
	}
	defer rows.Close()

	var result []migrationRow
	for rows.Next() {
		var row migrationRow
		if err := rows.Scan(&row.version, &row.appliedAt, &row.checksum); err != nil {
			t.Fatalf("scan schema_migrations: %v", err)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema_migrations: %v", err)
	}

	return result
}

func request(t *testing.T, url, token string) (int, string) {
	t.Helper()

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("create request %s: %v", url, err)
	}
	if token != "" {
		req.Header.Set("X-Temper-Token", token)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(req)
	if err != nil {
		t.Fatalf("request %s: %v", url, err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response from %s: %v", url, err)
	}

	return response.StatusCode, strings.TrimSpace(string(body))
}

type errorEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func requireErrorShape(t *testing.T, body, code string) {
	t.Helper()

	var envelope errorEnvelope
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	if envelope.Error.Code != code {
		t.Fatalf("error code = %q, want %q; body = %s", envelope.Error.Code, code, body)
	}
	if envelope.Error.Message == "" {
		t.Fatalf("error message is empty; body = %s", body)
	}
}

func randomToken(t *testing.T) string {
	t.Helper()

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return hex.EncodeToString(raw)
}

func migrationChecksum(t *testing.T) string {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(apiRoot(t), "migrations", "0001_idempotency.sql"))
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}

func requireListeningLineOnce(t *testing.T, stdout string) {
	t.Helper()

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	count := 0
	for _, line := range lines {
		if listeningPattern.MatchString(strings.TrimSpace(line)) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("listening line count = %d, want 1; stdout = %q", count, stdout)
	}
}

func TestEndToEnd(t *testing.T) {
	directory := t.TempDir()
	dbPath := filepath.Join(directory, "temper.db")
	token := randomToken(t)

	first := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL := first.waitListening(t, 15*time.Second)
	requireListeningLineOnce(t, first.stdout.String())

	status, body := request(t, baseURL+"/api/health", "")
	if status != http.StatusOK || body != `{"ok":true}` {
		t.Fatalf("public health = %d %q, want 200 {\"ok\":true}", status, body)
	}

	const unauthorized = `{"error":{"code":"unauthorized","message":"missing or invalid token"}}`

	status, body = request(t, baseURL+"/api/plans/current", "")
	if status != http.StatusUnauthorized || body != unauthorized {
		t.Fatalf("missing token = %d %q, want 401 %s", status, body, unauthorized)
	}

	status, body = request(t, baseURL+"/api/plans/current", "wrong-"+token)
	if status != http.StatusUnauthorized || body != unauthorized {
		t.Fatalf("wrong token = %d %q, want 401 %s", status, body, unauthorized)
	}

	status, body = request(t, baseURL+"/api/plans/current", token)
	if status != http.StatusNotFound {
		t.Fatalf("valid token unknown route = %d %q, want 404", status, body)
	}
	requireErrorShape(t, body, "not_found")

	status, body = request(t, baseURL+"/api/health", token)
	if status != http.StatusOK || body != `{"ok":true}` {
		t.Fatalf("authenticated health = %d %q, want 200 {\"ok\":true}", status, body)
	}

	first.stop(t)
	if strings.Contains(first.logs(), token) {
		t.Fatalf("the token appears in the process output:\n%s", first.output())
	}

	database := openDatabase(t, dbPath)
	rows := readMigrations(t, database)
	if len(rows) != 1 {
		t.Fatalf("schema_migrations rows = %d, want 1: %+v", len(rows), rows)
	}
	if rows[0].version != 1 {
		t.Fatalf("schema_migrations version = %d, want 1", rows[0].version)
	}
	if len(rows[0].checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64: %q", len(rows[0].checksum), rows[0].checksum)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(rows[0].checksum) {
		t.Fatalf("checksum %q is not 64 lowercase hex characters", rows[0].checksum)
	}
	if rows[0].checksum != migrationChecksum(t) {
		t.Fatalf("stored checksum %q does not match the migration bytes", rows[0].checksum)
	}
	if rows[0].appliedAt <= 0 {
		t.Fatalf("applied_at = %d, want a positive timestamp", rows[0].appliedAt)
	}

	var table string
	if err := database.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'idempotency_request'`).Scan(&table); err != nil {
		t.Fatalf("idempotency_request table is missing: %v", err)
	}
	appliedAt := rows[0].appliedAt
	database.Close()

	second := startProcess(t, dbPath, token, "127.0.0.1:0")
	baseURL = second.waitListening(t, 15*time.Second)

	status, body = request(t, baseURL+"/api/health", "")
	if status != http.StatusOK || body != `{"ok":true}` {
		t.Fatalf("health after restart = %d %q, want 200 {\"ok\":true}", status, body)
	}

	second.stop(t)
	if strings.Contains(second.logs(), token) {
		t.Fatalf("the token appears in the process output:\n%s", second.output())
	}

	database = openDatabase(t, dbPath)
	rows = readMigrations(t, database)
	if len(rows) != 1 || rows[0].version != 1 {
		t.Fatalf("schema_migrations after restart = %+v, want exactly version 1", rows)
	}
	if rows[0].appliedAt != appliedAt {
		t.Fatalf("applied_at changed after restart: %d then %d", appliedAt, rows[0].appliedAt)
	}
	if rows[0].checksum != migrationChecksum(t) {
		t.Fatalf("checksum changed after restart: %q", rows[0].checksum)
	}
	database.Close()
}

func TestRefusesTamperedChecksum(t *testing.T) {
	directory := t.TempDir()
	dbPath := filepath.Join(directory, "temper.db")
	token := randomToken(t)

	running := startProcess(t, dbPath, token, "127.0.0.1:0")
	running.waitListening(t, 15*time.Second)
	running.stop(t)

	database := openDatabase(t, dbPath)
	tampered := strings.Repeat("0", 64)
	if _, err := database.Exec(`UPDATE schema_migrations SET checksum = ? WHERE version = 1`, tampered); err != nil {
		t.Fatalf("tamper checksum: %v", err)
	}
	database.Close()

	failing := startProcess(t, dbPath, token, "127.0.0.1:0")
	if err := failing.waitExit(t, 10*time.Second); err == nil {
		t.Fatalf("temper started with a tampered checksum\n%s", failing.output())
	}

	output := failing.logs()
	if strings.Contains(output, "listening") {
		t.Fatalf("temper listened despite the tampered checksum:\n%s", failing.output())
	}
	for _, fragment := range []string{"migration 1", "checksum", tampered, migrationChecksum(t)} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output does not contain %q:\n%s", fragment, failing.output())
		}
	}
}

func TestRefusesFutureMigrationVersion(t *testing.T) {
	directory := t.TempDir()
	dbPath := filepath.Join(directory, "temper.db")
	token := randomToken(t)

	running := startProcess(t, dbPath, token, "127.0.0.1:0")
	running.waitListening(t, 15*time.Second)
	running.stop(t)

	database := openDatabase(t, dbPath)
	if _, err := database.Exec(`INSERT INTO schema_migrations (version, applied_at, checksum) VALUES (999, ?, ?)`, time.Now().Unix(), strings.Repeat("a", 64)); err != nil {
		t.Fatalf("insert future version: %v", err)
	}
	database.Close()

	failing := startProcess(t, dbPath, token, "127.0.0.1:0")
	if err := failing.waitExit(t, 10*time.Second); err == nil {
		t.Fatalf("temper started with a future schema version\n%s", failing.output())
	}

	output := failing.logs()
	if strings.Contains(output, "listening") {
		t.Fatalf("temper listened despite the future schema version:\n%s", failing.output())
	}
	for _, fragment := range []string{"999", "newer"} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("output does not contain %q:\n%s", fragment, failing.output())
		}
	}
}

func TestRefusesEmptyToken(t *testing.T) {
	command := exec.Command(binaryPath)
	command.Env = temperEnvironment(filepath.Join(t.TempDir(), "temper.db"), "", "127.0.0.1:0")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if err := command.Run(); err == nil {
		t.Fatalf("temper started with an empty token\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	if stderr.Len() == 0 {
		t.Fatal("temper failed without writing a message to stderr")
	}
	if strings.Contains(stdout.String(), "listening") {
		t.Fatalf("temper listened with an empty token:\n%s", stdout.String())
	}
}

func runGit(t *testing.T, directory string, args ...string) {
	t.Helper()

	command := exec.Command("git", args...)
	command.Dir = directory
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func TestPreCommitHookRejectsMainAndMaster(t *testing.T) {
	hook := filepath.Join(apiRoot(t), "scripts", "pre-commit")
	info, err := os.Stat(hook)
	if err != nil {
		t.Fatalf("stat hook: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("hook %s is not executable", hook)
	}

	repository := t.TempDir()
	runGit(t, repository, "init", "-q")

	for _, branch := range []string{"main", "master"} {
		runGit(t, repository, "symbolic-ref", "HEAD", "refs/heads/"+branch)

		command := exec.Command(hook)
		command.Dir = repository
		output, err := command.CombinedOutput()
		if err == nil {
			t.Fatalf("hook accepted branch %s:\n%s", branch, output)
		}
		if !strings.Contains(string(output), branch) {
			t.Fatalf("hook refusal does not name branch %s:\n%s", branch, output)
		}
	}

	runGit(t, repository, "symbolic-ref", "HEAD", "refs/heads/feat/skeleton")

	command := exec.Command(hook)
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("hook rejected a feature branch: %v\n%s", err, output)
	}
}
