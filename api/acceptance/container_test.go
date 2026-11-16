//go:build container

package acceptance

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

var backupFilePattern = regexp.MustCompile(`^temper-[0-9]{4}-[0-9]{2}-[0-9]{2}\.db$`)

func TestContainerBackupRestoreRetention(t *testing.T) {
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

	image := fmt.Sprintf("temper-acceptance:%d", time.Now().UnixNano())
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
	sourcePort := freePort(t)
	sourceName := fmt.Sprintf("temper-acceptance-%d", time.Now().UnixNano())
	startContainer(t, sourceName, image, dataDir, sourcePort, token)
	sourceURL := fmt.Sprintf("http://127.0.0.1:%d", sourcePort)
	waitForHealth(t, sourceURL, sourceName, 30*time.Second)

	insert := `PRAGMA busy_timeout=5000; INSERT INTO idempotency_request (operation, key, request_hash, result_json, created_at) VALUES ('acceptance', 'marker', 'hash', '{}', 0);`
	if output, err := exec.Command("docker", "exec", sourceName, "sqlite3", "/data/temper.db", insert).CombinedOutput(); err != nil {
		t.Fatalf("insert marker row: %v\n%s", err, output)
	}

	walPath := filepath.Join(dataDir, "temper.db-wal")
	if _, err := os.Stat(walPath); err != nil {
		t.Fatalf("the live database is not in WAL mode, %s is missing: %v", walPath, err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	backupsDir := filepath.Join(dataDir, "backups")
	backupName := fmt.Sprintf("temper-%s.db", today)
	backupPath := filepath.Join(backupsDir, backupName)

	output, err := exec.Command("docker", "exec", sourceName, "/app/backup.sh").CombinedOutput()
	if err != nil {
		t.Fatalf("backup script: %v\n%s", err, output)
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup %s does not exist: %v\n%s", backupPath, err, output)
	}
	printed := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(printed) != 1 || !strings.Contains(printed[0], "/data/backups/"+backupName) || !strings.HasSuffix(printed[0], " bytes") {
		t.Fatalf("backup output is not one line naming %s and its size:\n%s", backupName, output)
	}
	requireIntegrity(t, backupPath)
	if count := queryInt(t, backupPath, `SELECT count(*) FROM idempotency_request WHERE operation = 'acceptance' AND key = 'marker'`); count != 1 {
		t.Fatalf("marker rows in backup = %d, want 1", count)
	}

	restoreDir := tempDirectory(t, repository)
	if err := os.Chmod(restoreDir, 0o777); err != nil {
		t.Fatalf("make restore directory writable: %v", err)
	}
	restoredPath := filepath.Join(restoreDir, "temper.db")
	copyFile(t, backupPath, restoredPath)
	if err := os.Chmod(restoredPath, 0o666); err != nil {
		t.Fatalf("make restored database writable: %v", err)
	}

	restorePort := freePort(t)
	restoreName := sourceName + "-restore"
	startContainer(t, restoreName, image, restoreDir, restorePort, token)
	restoreURL := fmt.Sprintf("http://127.0.0.1:%d", restorePort)
	waitForHealth(t, restoreURL, restoreName, 30*time.Second)

	sourceMigrations := migrationPairs(t, filepath.Join(dataDir, "temper.db"))
	restoredMigrations := migrationPairs(t, restoredPath)
	if sourceMigrations != restoredMigrations {
		t.Fatalf("restored schema_migrations = %q, source = %q", restoredMigrations, sourceMigrations)
	}

	if err := os.Chmod(backupsDir, 0o777); err != nil {
		t.Fatalf("make backup directory writable: %v", err)
	}
	now := time.Now().UTC()
	for days := 0; days < 10; days++ {
		name := fmt.Sprintf("temper-%s.db", now.AddDate(0, 0, -days).Format("2006-01-02"))
		if err := os.WriteFile(filepath.Join(backupsDir, name), nil, 0o644); err != nil {
			t.Fatalf("create fake backup %s: %v", name, err)
		}
	}

	output, err = exec.Command("docker", "exec", sourceName, "/app/backup.sh").CombinedOutput()
	if err != nil {
		t.Fatalf("backup script during retention run: %v\n%s", err, output)
	}

	remaining := listBackups(t, backupsDir)
	if len(remaining) != 7 {
		t.Fatalf("backups after retention = %v, want 7 files", remaining)
	}
	if remaining[len(remaining)-1] != backupName {
		t.Fatalf("newest backup = %q, want %q", remaining[len(remaining)-1], backupName)
	}
	requireIntegrity(t, backupPath)
	if count := queryInt(t, backupPath, `SELECT count(*) FROM idempotency_request WHERE operation = 'acceptance' AND key = 'marker'`); count != 1 {
		t.Fatalf("marker rows in today's retained backup = %d, want 1", count)
	}

	beforeFailure := backupSnapshot(t, backupsDir)
	output, err = exec.Command("docker", "exec", "-e", "TEMPER_DB=/data/missing.db", sourceName, "/app/backup.sh").CombinedOutput()
	if err == nil {
		t.Fatalf("backup with a missing database succeeded:\n%s", output)
	}
	afterFailure := backupSnapshot(t, backupsDir)
	if beforeFailure != afterFailure {
		t.Fatalf("a failed backup changed the retained files:\nbefore:\n%s\nafter:\n%s", beforeFailure, afterFailure)
	}
}

func tempDirectory(t *testing.T, repository string) string {
	t.Helper()

	base := filepath.Join(repository, "api", "build", "acceptance")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatalf("create acceptance build directory: %v", err)
	}
	directory, err := os.MkdirTemp(base, "run-")
	if err != nil {
		t.Fatalf("create temporary host directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(directory)
		_ = os.Remove(base)
		_ = os.Remove(filepath.Dir(base))
	})
	return directory
}

func startContainer(t *testing.T, name, image, dataDir string, port int, token string) {
	t.Helper()

	t.Cleanup(func() {
		_ = exec.Command("docker", "rm", "-f", name).Run()
	})

	command := exec.Command("docker", "run", "-d",
		"--name", name,
		"--network", "host",
		"--no-healthcheck",
		"-v", dataDir+":/data",
		"-e", "TEMPER_ADDR=127.0.0.1:"+strconv.Itoa(port),
		"-e", "TEMPER_TOKEN="+token,
		image,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("start container %s: %v\n%s", name, err, output)
	}
}

func waitForHealth(t *testing.T, baseURL, container string, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := client.Get(baseURL + "/api/health")
		if err == nil {
			body, readErr := io.ReadAll(response.Body)
			response.Body.Close()
			if readErr == nil && response.StatusCode == http.StatusOK && strings.TrimSpace(string(body)) == `{"ok":true}` {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}

	logs, _ := exec.Command("docker", "logs", container).CombinedOutput()
	t.Fatalf("health at %s did not become 200 {\"ok\":true}\ncontainer logs:\n%s", baseURL, logs)
}

func freePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("find a free loopback port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("close the free-port listener: %v", err)
	}
	return port
}

func requireIntegrity(t *testing.T, path string) {
	t.Helper()

	database := openDatabase(t, path)
	defer database.Close()

	var result string
	if err := database.QueryRow(`PRAGMA integrity_check`).Scan(&result); err != nil {
		t.Fatalf("integrity check %s: %v", path, err)
	}
	if result != "ok" {
		t.Fatalf("integrity check %s = %q, want ok", path, result)
	}
}

func queryInt(t *testing.T, path, query string) int {
	t.Helper()

	database := openDatabase(t, path)
	defer database.Close()

	var value int
	if err := database.QueryRow(query).Scan(&value); err != nil {
		t.Fatalf("query %q on %s: %v", query, path, err)
	}
	return value
}

func migrationPairs(t *testing.T, path string) string {
	t.Helper()

	database := openDatabase(t, path)
	defer database.Close()

	rows := readMigrations(t, database)
	pairs := make([]string, 0, len(rows))
	for _, row := range rows {
		pairs = append(pairs, fmt.Sprintf("%d:%s", row.version, row.checksum))
	}
	return strings.Join(pairs, ",")
}

func listBackups(t *testing.T, directory string) []string {
	t.Helper()

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read backup directory %s: %v", directory, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if backupFilePattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func backupSnapshot(t *testing.T, directory string) string {
	t.Helper()

	lines := make([]string, 0)
	for _, name := range listBackups(t, directory) {
		info, err := os.Stat(filepath.Join(directory, name))
		if err != nil {
			t.Fatalf("stat backup %s: %v", name, err)
		}
		lines = append(lines, fmt.Sprintf("%s:%d", name, info.Size()))
	}
	return strings.Join(lines, "\n")
}

func copyFile(t *testing.T, source, destination string) {
	t.Helper()

	input, err := os.Open(source)
	if err != nil {
		t.Fatalf("open %s: %v", source, err)
	}
	defer input.Close()

	output, err := os.Create(destination)
	if err != nil {
		t.Fatalf("create %s: %v", destination, err)
	}

	if _, err := io.Copy(output, input); err != nil {
		output.Close()
		t.Fatalf("copy %s to %s: %v", source, destination, err)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("close %s: %v", destination, err)
	}
}
