// Package compile turns a project into a PDF.
//
// The work is clsi's; what is here is everything around it: deciding who may
// compile, making sure the files clsi will read are the current ones, and
// giving the editor back a result it can show. clsi is the last Node service
// this talks to, and the boundary is kept narrow so that replacing it later is
// a change to this file.
package compile

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Request is what the editor asks for.
type Request struct {
	// RootDocID overrides the project's own root document for this compile.
	RootDocID string `json:"rootDocId,omitempty"`
	Draft     bool   `json:"draft,omitempty"`
	// StopOnFirstError makes a broken document fail fast rather than produce
	// pages nobody wants.
	StopOnFirstError bool `json:"stopOnFirstError,omitempty"`
	// Incremental compiles reuse what the last one produced.
	Incremental bool `json:"incremental,omitempty"`
}

// OutputFile is one thing a compile produced.
type OutputFile struct {
	Path string `json:"path"`
	URL  string `json:"url"`
	Type string `json:"type,omitempty"`
	Size int64  `json:"size,omitempty"`
	// Build identifies which run produced it, and is part of its address.
	Build string `json:"build,omitempty"`
}

// Result is what a compile answers with.
type Result struct {
	Status string `json:"status"`
	// Error is set when the status is not a success, in words for a person.
	Error string `json:"error,omitempty"`
	// OutputFiles are the PDF and the logs.
	OutputFiles []OutputFile `json:"outputFiles"`
	// Stats and Timings are what the compile cost, for anybody looking.
	Stats   map[string]any `json:"stats,omitempty"`
	Timings map[string]any `json:"timings,omitempty"`
}

// Client talks to clsi.
type Client struct {
	baseURL string
	http    *http.Client
}

// NewClient builds one.
//
// The timeout is generous: a compile is allowed to take as long as the project
// is configured to allow, and a client that gave up first would leave the work
// running and tell the person it failed.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Minute},
	}
}

// Resource is one file clsi is told about.
type Resource struct {
	Path string `json:"path"`
	// Content is the text of a document, for the files being edited.
	Content string `json:"content,omitempty"`
	// URL is where clsi fetches a binary file from instead.
	URL      string `json:"url,omitempty"`
	Modified string `json:"modified,omitempty"`
}

// Options is the whole of what clsi needs to run once.
type Options struct {
	Compiler         string
	Timeout          int
	ImageName        string
	Draft            bool
	StopOnFirstError bool
	Incremental      bool
	RootResourcePath string
}

// Run compiles a project.
func (c *Client) Run(
	ctx context.Context,
	projectID, userID bson.ObjectID,
	resources []Resource,
	opts Options,
) (*Result, error) {
	payload := map[string]any{
		"compile": map[string]any{
			"options": map[string]any{
				"compiler":         opts.Compiler,
				"timeout":          opts.Timeout,
				"imageName":        opts.ImageName,
				"draft":            opts.Draft,
				"stopOnFirstError": opts.StopOnFirstError,
				"check":            "silent",
				"syncType":         syncType(opts.Incremental),
			},
			"rootResourcePath": opts.RootResourcePath,
			"resources":        resources,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("%s/project/%s/user/%s/compile",
		c.baseURL, projectID.Hex(), userID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode >= 500 {
		return nil, fmt.Errorf("clsi answered %s", response.Status)
	}

	var answer struct {
		Compile struct {
			Status      string         `json:"status"`
			Error       string         `json:"error"`
			OutputFiles []OutputFile   `json:"outputFiles"`
			Stats       map[string]any `json:"stats"`
			Timings     map[string]any `json:"timings"`
		} `json:"compile"`
	}
	if err := json.Unmarshal(raw, &answer); err != nil {
		return nil, err
	}

	result := &Result{
		Status:      answer.Compile.Status,
		Error:       answer.Compile.Error,
		OutputFiles: answer.Compile.OutputFiles,
		Stats:       answer.Compile.Stats,
		Timings:     answer.Compile.Timings,
	}
	for i := range result.OutputFiles {
		result.OutputFiles[i].URL = sitePath(result.OutputFiles[i].URL)
	}
	return result, nil
}

// sitePath turns an address clsi gave for one of its files into one the
// browser can use.
//
// clsi answers with its own host, which is inside the container and means
// nothing to anybody outside it. The path is the part that matters: nginx has
// a route for exactly these, so a browser asking the site for the same path
// gets the file, and the PDF never travels through this process at all.
func sitePath(raw string) string {
	if raw == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Path == "" {
		return raw
	}
	if parsed.RawQuery != "" {
		return parsed.Path + "?" + parsed.RawQuery
	}
	return parsed.Path
}

// Stop cancels a compile that is still running.
func (c *Client) Stop(ctx context.Context, projectID, userID bson.ObjectID) error {
	endpoint := fmt.Sprintf("%s/project/%s/user/%s/compile/stop",
		c.baseURL, projectID.Hex(), userID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	return nil
}

func syncType(incremental bool) string {
	if incremental {
		return "incremental"
	}
	return "full"
}

// WordCount asks the compiler to count what is in a project.
//
// The compiler rather than this service, because texcount reads the files as
// TeX sees them: it follows \input, skips the preamble, and does not count a
// command name as a word. Counting here would mean reimplementing that, badly.
//
// It reads the last compile's directory, so a project that has not been
// compiled has nothing to count -- which is why the answer can be empty and
// is not an error.
func (c *Client) WordCount(
	ctx context.Context,
	projectID, userID bson.ObjectID,
	file string,
) (map[string]any, error) {
	endpoint := fmt.Sprintf("%s/project/%s/user/%s/wordcount?file=%s",
		c.baseURL, projectID.Hex(), userID.Hex(), url.QueryEscape(file))
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 400 {
		return nil, fmt.Errorf("the compiler answered %d", response.StatusCode)
	}
	var answer struct {
		TexCount map[string]any `json:"texcount"`
	}
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil {
		return nil, err
	}
	return answer.TexCount, nil
}

// PDFPosition is where in the PDF something is: a rectangle on a page, in TeX
// points from the top-left corner.
type PDFPosition struct {
	Page   float64 `json:"page"`
	H      float64 `json:"h"`
	V      float64 `json:"v"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// CodePosition is where in the source something is.
type CodePosition struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Column int    `json:"column"`
}

// SyncFromCode is where in the PDF a line of the source ended up.
//
// The answer is a list because one line can produce several pieces of the
// document -- a line inside a table, or one that a package typesets twice.
func (c *Client) SyncFromCode(
	ctx context.Context,
	projectID, userID bson.ObjectID,
	file string, line, column int,
) ([]PDFPosition, error) {
	endpoint := fmt.Sprintf("%s/project/%s/user/%s/sync/code?file=%s&line=%d&column=%d",
		c.baseURL, projectID.Hex(), userID.Hex(), url.QueryEscape(file), line, column)
	var answer struct {
		PDF []struct {
			Page   float64 `json:"page"`
			H      float64 `json:"h"`
			V      float64 `json:"v"`
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"pdf"`
	}
	if err := c.get(ctx, endpoint, &answer); err != nil {
		return nil, err
	}
	positions := make([]PDFPosition, 0, len(answer.PDF))
	for _, each := range answer.PDF {
		positions = append(positions, PDFPosition{
			Page: each.Page, H: each.H, V: each.V,
			Width: each.Width, Height: each.Height,
		})
	}
	return positions, nil
}

// SyncFromPDF is which line of the source a place in the PDF came from.
func (c *Client) SyncFromPDF(
	ctx context.Context,
	projectID, userID bson.ObjectID,
	page int, h, v float64,
) ([]CodePosition, error) {
	endpoint := fmt.Sprintf("%s/project/%s/user/%s/sync/pdf?page=%d&h=%f&v=%f",
		c.baseURL, projectID.Hex(), userID.Hex(), page, h, v)
	var answer struct {
		Code []struct {
			File   string  `json:"file"`
			Line   float64 `json:"line"`
			Column float64 `json:"column"`
		} `json:"code"`
	}
	if err := c.get(ctx, endpoint, &answer); err != nil {
		return nil, err
	}
	positions := make([]CodePosition, 0, len(answer.Code))
	for _, each := range answer.Code {
		positions = append(positions, CodePosition{
			File:   each.File,
			Line:   int(each.Line),
			Column: int(each.Column),
		})
	}
	return positions, nil
}

// get reads JSON from the compiler.
func (c *Client) get(ctx context.Context, endpoint string, into any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	// A position that maps to nothing is an ordinary answer, not a failure:
	// somebody can click the margin of a page, and a line of the preamble
	// produces no output at all.
	if response.StatusCode == http.StatusNotFound {
		return nil
	}
	if response.StatusCode >= 400 {
		return fmt.Errorf("the compiler answered %d", response.StatusCode)
	}
	return json.NewDecoder(response.Body).Decode(into)
}

// Clear removes what a project left in the compiler.
//
// The compile directory and the output directory: a copy of every file, the
// PDF, the logs, and everything TeX wrote along the way. They are a cache and
// would be rebuilt by the next compile -- but a project that has been deleted
// has no next compile, so they would simply stay, and they are the largest
// thing on the disk after the history.
func (c *Client) Clear(ctx context.Context, projectID bson.ObjectID) error {
	// No user in the address: a project has one directory per person who has
	// compiled it, and all of them go.
	endpoint := fmt.Sprintf("%s/project/%s", c.baseURL, projectID.Hex())
	request, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode >= 300 && response.StatusCode != http.StatusNotFound {
		return fmt.Errorf("the compiler answered %s", response.Status)
	}
	return nil
}
