package clsi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// The compiler's HTTP surface.
//
// Its own shapes, unchanged from the service this replaces, because the caller
// was written against them. Nothing here serves a compiled file: nginx does
// that straight off the disk, which is why a hundred-megabyte PDF costs this
// process nothing.

// Handler is the whole API.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /project/{projectId}/compile", s.compile)
	mux.HandleFunc("POST /project/{projectId}/user/{userId}/compile", s.compile)
	mux.HandleFunc("POST /project/{projectId}/compile/stop", s.stop)
	mux.HandleFunc("POST /project/{projectId}/user/{userId}/compile/stop", s.stop)
	mux.HandleFunc("DELETE /project/{projectId}", s.clear)
	mux.HandleFunc("DELETE /project/{projectId}/user/{userId}", s.clear)

	mux.HandleFunc("GET /project/{projectId}/sync/code", s.syncFromCode)
	mux.HandleFunc("GET /project/{projectId}/user/{userId}/sync/code", s.syncFromCode)
	mux.HandleFunc("GET /project/{projectId}/sync/pdf", s.syncFromPDF)
	mux.HandleFunc("GET /project/{projectId}/user/{userId}/sync/pdf", s.syncFromPDF)
	mux.HandleFunc("GET /project/{projectId}/wordcount", s.wordcount)
	mux.HandleFunc("GET /project/{projectId}/user/{userId}/wordcount", s.wordcount)

	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "CLSI is alive\n")
	})
	mux.HandleFunc("GET /health_check", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return mux
}

func (s *Service) compile(w http.ResponseWriter, r *http.Request) {
	var request Request
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 512<<20)).Decode(&request); err != nil {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	result, err := s.Compile(r.Context(), r.PathValue("projectId"), r.PathValue("userId"), &request)
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answerJSON(w, http.StatusOK, map[string]any{"compile": result})
}

func (s *Service) stop(w http.ResponseWriter, r *http.Request) {
	s.Stop(r.PathValue("projectId"), r.PathValue("userId"))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) clear(w http.ResponseWriter, r *http.Request) {
	if err := s.Clear(r.PathValue("projectId"), r.PathValue("userId")); err != nil {
		s.refuse(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// syncFromCode is where in the PDF a line of the source ended up.
func (s *Service) syncFromCode(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	line := r.URL.Query().Get("line")
	column := r.URL.Query().Get("column")
	if file == "" || line == "" {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	if column == "" {
		column = "0"
	}
	if err := safePath(file); err != nil {
		s.refuse(w, r, err)
		return
	}

	// -o names the PDF. Without it synctex refuses the request rather than
	// guessing, and answers with its usage text and a zero exit status --
	// which reads as "this line is not in the document" and is not.
	output, err := s.synctex(r, "view", "-i", line+":"+column+":"+file, "-o", "output.pdf")
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	answerJSON(w, http.StatusOK, map[string]any{"pdf": parseSyncOutput(output,
		syncField{"Page", "page"},
		syncField{"h", "h"},
		syncField{"v", "v"},
		syncField{"W", "width"},
		syncField{"H", "height"},
	)})
}

// syncFromPDF is which line of the source a place in the PDF came from.
func (s *Service) syncFromPDF(w http.ResponseWriter, r *http.Request) {
	page := r.URL.Query().Get("page")
	h := r.URL.Query().Get("h")
	v := r.URL.Query().Get("v")
	if page == "" || h == "" || v == "" {
		s.refuse(w, r, ErrBadRequest)
		return
	}
	output, err := s.synctex(r, "edit", "-o", page+":"+h+":"+v+":output.pdf")
	if err != nil {
		s.refuse(w, r, err)
		return
	}
	records := parseSyncOutput(output,
		syncField{"Input", "file"},
		syncField{"Line", "line"},
		syncField{"Column", "column"},
	)
	// synctex names the file the way TeX saw it, which is the absolute path of
	// the compile directory with the project's own path stuck on the end. The
	// caller has never heard of that directory and knows the file as
	// "chapters/one.tex", so the directory comes off here -- the only place
	// that knows what it was.
	compileDir := s.compileDir(r.PathValue("projectId"), r.PathValue("userId"))
	for _, record := range records {
		if path, ok := record["file"].(string); ok {
			record["file"] = projectPath(path, compileDir)
		}
	}
	answerJSON(w, http.StatusOK, map[string]any{"code": records})
}

// projectPath is a path as TeX printed it, as the project knows it.
func projectPath(path, compileDir string) string {
	path = filepath.ToSlash(path)
	dir := filepath.ToSlash(compileDir)
	if trimmed, inside := strings.CutPrefix(path, dir+"/"); inside {
		path = trimmed
	}
	return strings.TrimPrefix(path, "./")
}

// synctex runs the tool that maps between the two.
func (s *Service) synctex(r *http.Request, mode string, args ...string) (string, error) {
	compileDir := s.compileDir(r.PathValue("projectId"), r.PathValue("userId"))
	if _, err := os.Stat(filepath.Join(compileDir, "output.synctex.gz")); err != nil {
		return "", ErrNotFound
	}
	ctx, stop := context.WithTimeout(r.Context(), 60*time.Second)
	defer stop()

	command := append([]string{mode}, args...)
	cmd := exec.CommandContext(ctx, "synctex", command...)
	cmd.Dir = compileDir
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			// synctex answers non-zero when it cannot map a position, which is
			// an ordinary answer and not a failure.
			return string(output), nil
		}
		return "", err
	}
	return string(output), nil
}

// wordcount is how long the document is.
func (s *Service) wordcount(w http.ResponseWriter, r *http.Request) {
	file := r.URL.Query().Get("file")
	if file == "" {
		file = "main.tex"
	}
	if err := safePath(file); err != nil {
		s.refuse(w, r, err)
		return
	}
	compileDir := s.compileDir(r.PathValue("projectId"), r.PathValue("userId"))

	ctx, stop := context.WithTimeout(r.Context(), 60*time.Second)
	defer stop()
	cmd := exec.CommandContext(ctx, "texcount", "-nocol", "-inc", "-total", file)
	cmd.Dir = compileDir
	output, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			s.refuse(w, r, err)
			return
		}
	}
	answerJSON(w, http.StatusOK, map[string]any{"texcount": parseWordcount(string(output))})
}

func (s *Service) refuse(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrBadRequest):
		w.WriteHeader(http.StatusBadRequest)
	case errors.Is(err, ErrNotFound):
		w.WriteHeader(http.StatusNotFound)
	default:
		if s.log != nil {
			s.log.Error("a compile request failed",
				slog.String("path", r.URL.Path), slog.Any("err", err))
		}
		w.WriteHeader(http.StatusInternalServerError)
	}
}

// syncField is one of synctex's own field names and what to call it here.
//
// The two differ because synctex tells position from size by case alone: h
// and v are where something is, W and H are how large it is. Anything that
// folds case loses half of that, so the names it prints are matched exactly
// and renamed once, here.
type syncField struct {
	from string
	to   string
}

// parseSyncOutput reads the fields synctex prints as "Name:value" lines.
//
// One position per record, and a record ends where a field it already has
// appears again -- synctex prints no other separator, and a line that is
// typeset more than once produces several.
func parseSyncOutput(output string, fields ...syncField) []map[string]any {
	wanted := map[string]string{}
	for _, field := range fields {
		wanted[field.from] = field.to
	}

	var records []map[string]any
	current := map[string]any{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name, value := line[:colon], line[colon+1:]
		key, ok := wanted[name]
		if !ok {
			continue
		}
		if _, repeated := current[key]; repeated {
			records = append(records, current)
			current = map[string]any{}
		}
		if number, err := strconv.ParseFloat(value, 64); err == nil {
			current[key] = number
		} else {
			current[key] = value
		}
	}
	if len(current) > 0 {
		records = append(records, current)
	}
	return records
}

// parseWordcount reads what texcount printed into the shape the editor shows.
func parseWordcount(output string) map[string]any {
	counts := map[string]any{}
	for _, line := range strings.Split(output, "\n") {
		colon := strings.IndexByte(line, ':')
		if colon <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		key, known := wordcountFields[name]
		if !known {
			continue
		}
		if number, err := strconv.Atoi(strings.Fields(value + " ")[0]); err == nil {
			counts[key] = number
		}
	}
	return counts
}

// wordcountFields is what texcount calls each number and what the editor does.
var wordcountFields = map[string]string{
	"Words in text":                       "textWords",
	"Words in headers":                    "headWords",
	"Words outside text (captions, etc.)": "outsideWords",
	"Number of headers":                   "headers",
	"Number of floats/tables/figures":     "elements",
	"Number of math inlines":              "mathInline",
	"Number of math displayed":            "mathDisplay",
}
