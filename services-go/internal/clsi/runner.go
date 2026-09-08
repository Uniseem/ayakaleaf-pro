package clsi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Running the compiler.
//
// Two ways, and which one a deployment uses is the difference between trusting
// its users and not. LaTeX can read and write files, and \write18 can run
// programs; on a site where anybody can sign up, a compile has to happen
// somewhere it can only reach one project.

// runRequest is one run.
type runRequest struct {
	ProjectID  string
	UserID     string
	CompileDir string
	Command    []string
	Image      string
	Timeout    time.Duration
}

// runResult is how it went.
type runResult struct {
	ExitCode int
	TimedOut bool
}

// watcher hands the caller a way to stop the run, and takes back a function to
// call when it is over.
type watcher func(context.CancelFunc) func()

type runner interface {
	Run(ctx context.Context, request runRequest, watch watcher) (*runResult, error)
}

// --- in this container -----------------------------------------------------

type localRunner struct {
	options Options
}

func newLocalRunner(options Options) *localRunner { return &localRunner{options: options} }

// Run starts latexmk here, as an unprivileged user.
//
// The whole compile directory is what it can see, which on a deployment where
// everybody knows each other is enough. It is not isolation: a document that
// asks to read another project's file will be told where that file is by the
// filesystem, which is why the other runner exists.
func (r *localRunner) Run(ctx context.Context, request runRequest, watch watcher) (*runResult, error) {
	ctx, stop := context.WithTimeout(ctx, request.Timeout)
	defer stop()
	if watch != nil {
		defer watch(stop)()
	}

	command := replaceAll(request.Command, "$COMPILE_DIR", request.CompileDir)
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = request.CompileDir
	cmd.Env = append(os.Environ(),
		"HOME="+request.CompileDir,
		// A compile that shells out has nothing to shell out to. This is not
		// a boundary, but it removes the easiest way through one.
		"openout_any=p",
		"openin_any=p",
		"shell_escape=f",
	)

	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	// Written next to the document, because when a compile goes wrong in a way
	// LaTeX does not explain, this is what does.
	_ = os.WriteFile(filepath.Join(request.CompileDir, "output.stdout"), stdout.Bytes(), 0o644)
	_ = os.WriteFile(filepath.Join(request.CompileDir, "output.stderr"), stderr.Bytes(), 0o644)

	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &runResult{TimedOut: true}, nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return &runResult{ExitCode: exit.ExitCode()}, nil
	}
	if err != nil {
		return nil, err
	}
	return &runResult{}, nil
}

// --- in a container of its own ---------------------------------------------

// dockerRunner starts a container per compile, with only that project's
// directory mounted.
//
// It talks to the daemon over its socket rather than through a library: four
// calls, and a dependency that can start containers is a large thing to take
// on for four calls.
type dockerRunner struct {
	options Options
	http    *http.Client
}

func newDockerRunner(options Options) *dockerRunner {
	socket := os.Getenv("DOCKER_HOST")
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	socket = strings.TrimPrefix(socket, "unix://")
	return &dockerRunner{
		options: options,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

func (r *dockerRunner) Run(ctx context.Context, request runRequest, watch watcher) (*runResult, error) {
	ctx, stop := context.WithTimeout(ctx, request.Timeout+30*time.Second)
	defer stop()
	if watch != nil {
		defer watch(stop)()
	}

	// The path inside the container is fixed, so the command does not depend
	// on where this service keeps its directories.
	const inside = "/compile"
	command := replaceAll(request.Command, "$COMPILE_DIR", inside)

	// The daemon is on the host, so the mount has to name the directory as the
	// host sees it. Getting this wrong mounts nothing and every compile fails
	// with a missing file.
	hostDir := filepath.Join(r.options.HostCompilesDir, compileName(request.ProjectID, request.UserID))

	binds := []string{hostDir + ":" + inside}
	config := map[string]any{
		"Image":      request.Image,
		"Cmd":        command,
		"WorkingDir": inside,
		"Env": []string{
			"HOME=" + inside,
			"openout_any=p", "openin_any=p", "shell_escape=f",
		},
		"User":            r.options.User,
		"NetworkDisabled": true,
		"HostConfig": map[string]any{
			"Binds": binds,
			// A compile is a computation with an end. Without these one
			// document can take the machine down for everybody.
			"Memory":     int64(1) << 30,
			"CpuShares":  512,
			"PidsLimit":  512,
			"AutoRemove": false,
			"CapDrop":    []string{"ALL"},
			"SecurityOpt": func() []string {
				if r.options.SeccompProfile != "" {
					return []string{"seccomp=" + r.options.SeccompProfile}
				}
				return []string{"no-new-privileges"}
			}(),
		},
	}

	created, err := r.call(ctx, http.MethodPost, "/containers/create", config)
	if err != nil {
		return nil, err
	}
	var container struct {
		ID string `json:"Id"`
	}
	if err := json.Unmarshal(created, &container); err != nil {
		return nil, err
	}
	// However this ends, the container goes.
	defer func() {
		cleanup, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_, _ = r.call(cleanup, http.MethodDelete,
			"/containers/"+container.ID+"?force=true&v=true", nil)
	}()

	if _, err := r.call(ctx, http.MethodPost, "/containers/"+container.ID+"/start", nil); err != nil {
		return nil, err
	}

	waited, err := r.call(ctx, http.MethodPost, "/containers/"+container.ID+"/wait", nil)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		killing, done := context.WithTimeout(context.Background(), 30*time.Second)
		defer done()
		_, _ = r.call(killing, http.MethodPost, "/containers/"+container.ID+"/kill", nil)
		return &runResult{TimedOut: true}, nil
	}
	if err != nil {
		return nil, err
	}
	var status struct {
		StatusCode int `json:"StatusCode"`
	}
	if err := json.Unmarshal(waited, &status); err != nil {
		return nil, err
	}
	return &runResult{ExitCode: status.StatusCode}, nil
}

// call makes one request to the docker daemon.
func (r *dockerRunner) call(ctx context.Context, method, path string, body any) ([]byte, error) {
	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		payload = bytes.NewReader(encoded)
	}
	// The host is ignored: the connection is a unix socket. It has to be
	// something, so it is this.
	request, err := http.NewRequestWithContext(ctx, method, "http://docker/v1.41"+path, payload)
	if err != nil {
		return nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := r.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	answer, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 300 {
		return nil, fmt.Errorf("docker answered %s: %s", response.Status, summary(answer))
	}
	return answer, nil
}

func summary(body []byte) string {
	var answer struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &answer) == nil && answer.Message != "" {
		return answer.Message
	}
	if len(body) > 200 {
		return string(body[:200])
	}
	return string(body)
}

func replaceAll(command []string, from, to string) []string {
	out := make([]string, len(command))
	for i, part := range command {
		out[i] = strings.ReplaceAll(part, from, to)
	}
	return out
}

// documentClass and documentClassPlain find the line draft mode is added to.
var (
	documentClass      = regexp.MustCompile(`\\documentclass\[([^\]]*)\]\{`)
	documentClassPlain = regexp.MustCompile(`\\documentclass\{`)
	// markupExtension is the files the editor generates a .tex from.
	markupExtension = regexp.MustCompile(`\.(Rtex|md|Rmd|Rnw)$`)
)
