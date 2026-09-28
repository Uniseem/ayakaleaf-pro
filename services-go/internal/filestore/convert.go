package filestore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"
)

const (
	convertTimeout = 40 * time.Second
	optipngTimeout = 30 * time.Second
)

// approvedFormats is the whitelist from FileConverter: only PNG may be asked
// for, so a format parameter cannot name an arbitrary converter target.
var approvedFormats = map[string]bool{"png": true}

var (
	// ErrConversionsDisabled matches ConversionsDisabledError.
	ErrConversionsDisabled = errors.New("image conversions are disabled")
	// ErrConversion matches ConversionError.
	ErrConversion = errors.New("failed to convert file")
)

// ConverterConfig mirrors the conversion-related settings.
type ConverterConfig struct {
	// Enabled is ENABLE_CONVERSIONS.
	Enabled bool
	// Backend is CONVERTER: "pdftocairo" or the imagemagick default.
	Backend string
	// CommandPrefix wraps every conversion command, e.g. ["nice"].
	CommandPrefix []string
}

// Converter renders a source file to PNG by running the same external tools
// the Node service runs.
type Converter struct{ cfg ConverterConfig }

// NewConverter builds a Converter.
func NewConverter(cfg ConverterConfig) *Converter { return &Converter{cfg: cfg} }

// Enabled reports whether conversions are switched on.
func (c *Converter) Enabled() bool { return c.cfg.Enabled }

// Convert renders sourcePath at full width and returns the output path.
func (c *Converter) Convert(ctx context.Context, sourcePath, format string) (string, error) {
	if c.cfg.Backend == "pdftocairo" {
		return c.run(ctx, sourcePath, format, pdftocairoArgs(sourcePath, 1500))
	}
	return c.run(ctx, sourcePath, format, []string{
		"convert", "-flatten", "-density", "300",
		sourcePath + "[0]", "-resize", "1280x",
	})
}

// Thumbnail renders a small preview.
func (c *Converter) Thumbnail(ctx context.Context, sourcePath string) (string, error) {
	if c.cfg.Backend == "pdftocairo" {
		return c.run(ctx, sourcePath, "png", pdftocairoArgs(sourcePath, 700))
	}
	return c.run(ctx, sourcePath, "png", imagemagickArgs(sourcePath, "260x"))
}

// Preview renders a medium-sized preview.
func (c *Converter) Preview(ctx context.Context, sourcePath string) (string, error) {
	if c.cfg.Backend == "pdftocairo" {
		return c.run(ctx, sourcePath, "png", pdftocairoArgs(sourcePath, 1000))
	}
	return c.run(ctx, sourcePath, "png", imagemagickArgs(sourcePath, "1000"))
}

func pdftocairoArgs(sourcePath string, width int) []string {
	return []string{
		"pdftocairo", "-png", "-singlefile",
		"-scale-to-x", fmt.Sprint(width),
		"-scale-to-y", "-1", // keep the aspect ratio
		sourcePath,
	}
}

func imagemagickArgs(sourcePath, width string) []string {
	return []string{
		"convert", "-flatten", "-background", "white", "-density", "300",
		"-define", "pdf:fit-page=" + width,
		sourcePath + "[0]", "-resize", width,
	}
}

// run executes one conversion. The destination differs between the two tools:
// pdftocairo appends its own ".png", so it is given the path without the
// extension, while imagemagick is given the full destination.
func (c *Converter) run(ctx context.Context, sourcePath, format string, args []string) (string, error) {
	if !c.cfg.Enabled {
		return "", ErrConversionsDisabled
	}
	if !approvedFormats[format] {
		return "", fmt.Errorf("%w: format %q is not approved", ErrConversion, format)
	}
	destPath := sourcePath + "." + format
	if c.cfg.Backend == "pdftocairo" {
		args = append(args, sourcePath)
	} else {
		args = append(args, destPath)
	}
	args = append(append([]string{}, c.cfg.CommandPrefix...), args...)

	if err := runWithTimeout(ctx, args, convertTimeout, syscall.SIGTERM); err != nil {
		return "", fmt.Errorf("%w: %v", ErrConversion, err)
	}
	return destPath, nil
}

// CompressPng shrinks a rendered PNG in place. A timeout here is not fatal --
// the uncompressed image is still correct -- which is how the Node
// implementation treats it too.
func (c *Converter) CompressPng(ctx context.Context, path string) error {
	if !c.cfg.Enabled {
		return ErrConversionsDisabled
	}
	err := runWithTimeout(ctx, []string{"optipng", path}, optipngTimeout, syscall.SIGKILL)
	if errors.Is(err, errCommandTimedOut) {
		return nil
	}
	return err
}

var errCommandTimedOut = errors.New("command timed out")

// runWithTimeout runs a command in its own process group and kills the whole
// group on timeout, so a converter that spawns helpers cannot leave them
// behind.
func runWithTimeout(ctx context.Context, args []string, timeout time.Duration, killSignal syscall.Signal) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	cmd.Stdout = os.NewFile(0, os.DevNull)

	if err := cmd.Start(); err != nil {
		return err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("%s failed: %w (stderr: %s)", args[0], err, stderr.String())
		}
		return nil
	case <-timer.C:
		// Negative pid signals the whole process group.
		_ = syscall.Kill(-cmd.Process.Pid, killSignal)
		<-done
		return errCommandTimedOut
	}
}

// limitedBuffer keeps only the first few KB of stderr, so a chatty failure
// cannot fill memory.
type limitedBuffer struct {
	buf []byte
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	const max = 8 * 1024
	if remaining := max - len(b.buf); remaining > 0 {
		if len(p) < remaining {
			remaining = len(p)
		}
		b.buf = append(b.buf, p[:remaining]...)
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return string(b.buf) }
