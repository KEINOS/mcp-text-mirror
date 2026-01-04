// Package main is a tiny MCP (Model Context Protocol) service and tool. It simply
// mirrors (reverses) UTF‑8 text while preserving grapheme clusters.
//
// This repository implements a minimal MCP server and a single `mirror` tool to
// help me (the author) learn MCP basics and to build something that at minimum
// works with VSCode's Copilot (via `stdio` transport).
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rivo/uniseg"
)

// Logger configuration.
const (
	envNameDebug   = "MCP_TEXT_MIRROR_DEBUG_LOG" // env var to enable debug logging. the value is the log path
	fileLogDefault = false                       // set to true to enable debug logging to a file by default
	logName        = "text-mirror.log"
	logDir         = "." // default directory (current directory)
	logFlag        = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	logPerm        = os.FileMode(0o644)
)

// Service metadata.
const (
	serviceName    = "text-mirror"
	serviceVersion = "(devel)" // default version if not set in build info
	serviceTitle   = "Text mirroring/reversing tool"
	revisionLen    = 7 // short revision length for display

	toolName        = "mirror"
	toolDescription = "Reverses the given UTF-8 text"
)

// CustomLogger is the minimal interface needed for fatal logging.
type CustomLogger interface {
	Fatal(v ...any)
	Print(v ...any)
}

// Predefined errors.
var errNilContext = errors.New("given context is nil")

// Dependency injection points to ease testing.
var (
	// logger is used to log fatal errors. Tests can replace it.
	logger CustomLogger = newLogger(IsDebugMode(), GetLogPath())
	// defaultCtx is the context used to run the server which is context.Background()
	// by default, but tests can override it.
	defaultCtx = context.Background()
	// debugReadBuildInfo is a copy of debug.ReadBuildInfo function.
	// Tests can replace it.
	debugReadBuildInfo = debug.ReadBuildInfo
	// runServer is the function that runs the MCP server. Tests can replace it.
	// It will error if given context is nil.
	runServer = func(ctx context.Context, server *mcp.Server) error {
		if ctx == nil {
			return errNilContext
		}

		return server.Run(ctx, &mcp.StdioTransport{})
	}
)

// ============================================================================
//  main
// ============================================================================

func main() {
	// defaultCtx may be overridden in tests.
	exitOnError(run(defaultCtx))
}

// IsDebugMode returns whether debug mode is enabled. If true then logging to a
// file is enabled. By default, it return fileLogDefault constant value.
//
// If 'MCP_TEXT_MIRROR_DEBUG_LOG' environment variable is set to a non-empty
// value, the value is used as the log path and debug mode is enabled.
func IsDebugMode() bool {
	if os.Getenv(envNameDebug) != "" {
		return true
	}

	return fileLogDefault
}

// GetLogPath returns the path to the log file.
//
// If 'MCP_TEXT_MIRROR_DEBUG_LOG' environment variable is set to a non-empty
// value, it returns the value as the log path.
func GetLogPath() string {
	logPath := filepath.Join(logDir, logName)

	envLogPath := os.Getenv(envNameDebug)
	if envLogPath != "" {
		logPath = envLogPath
	}

	return filepath.Clean(logPath)
}

// GetServiceVersion returns the service version string based on build info.
// If the build info is not available, it returns "unknown (devel)".
func GetServiceVersion() string {
	version := serviceVersion // default version (devel)
	revision := "unknown"

	info, ok := debugReadBuildInfo()
	if ok {
		// version
		if info.Main.Version != "" {
			version = info.Main.Version
		}

		// revision
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" && s.Value != "" {
				revision = s.Value

				break
			}
		}

		// Found version. E.g.: v1.0.0 (abcdef0)
		if version != serviceVersion {
			return fmt.Sprintf("%s (%s)", version, revision[:min(len(revision), revisionLen)])
		}
	}

	return fmt.Sprintf("%s %s", revision[:min(len(revision), revisionLen)], version)
}

// ----------------------------------------------------------------------------
//  Helper functions
// ----------------------------------------------------------------------------

// run starts the MCP server and returns any error encountered.
func run(ctx context.Context) error {
	server := newServer()

	// Run server with a transport that uses standard IO. Mock runServer in tests.
	err := runServer(ctx, server)
	if err != nil {
		return wrapError(err, "MCP server failed to run")
	}

	return nil
}

// newServer constructs and configures an MCP server with the mirror tool.
func newServer() *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    serviceName,
			Title:   serviceTitle,
			Version: GetServiceVersion(),
		},
		&mcp.ServerOptions{}, //nolint:exhaustruct // use default options
	)

	// Initialize with zero values then set required fields (avoid exhaustruct
	// linter error)
	toolInfo := new(mcp.Tool)
	toolInfo.Name = toolName
	toolInfo.Description = toolDescription

	// Add tool automatically and force tools to conform to the MCP spec.
	mcp.AddTool(server, toolInfo, handleReverse)

	return server
}

// newLogger creates a default logger.
//
// If toFile is true, it logs to the given path. Otherwise, it logs to standard error.
// If the log file cannot be opened, it silently falls back to logging to standard
// error.
//
// NOTE: The log file is intentionally kept open for the lifetime of the process.
func newLogger(toFile bool, path string) *log.Logger {
	out := os.Stderr

	if toFile {
		path = filepath.Clean(path)

		osFile, err := os.OpenFile(path, logFlag, logPerm)
		if err == nil {
			out = osFile
		}
	}

	logger := log.New(out, "", log.LstdFlags|log.LUTC)

	return logger
}

// debugLog logs the given values if debug mode is enabled.
func debugLog(v ...any) {
	if IsDebugMode() {
		logger.Print(v...)
	}
}

// wrapError returns nil if err is nil.
// Otherwise it wraps the error with given message. If args are provided, it
// formats the message with them.
func wrapError(err error, msg string, args ...any) error {
	if err == nil {
		return nil
	}

	if len(args) > 0 {
		msg = fmt.Sprintf(msg, args...)
	}

	return fmt.Errorf("%s: %w", msg, err)
}

// exitOnError logs the error and terminates the process (or panics in tests).
// If err is nil, it does nothing.
func exitOnError(err error) {
	if err != nil {
		logger.Fatal("Error:", err)
	}
}

// ============================================================================
//  'reverse' tool handler
// ============================================================================

// MirrorInput is the input for the mirror tool.
type MirrorInput struct {
	Text string `json:"text" jsonschema:"UTF-8 text to be mirrored"`
}

// MirrorOutput is the output from the mirror tool.
type MirrorOutput struct {
	Text string `json:"text" jsonschema:"Mirrored text"`
}

// handleReverse returns (meta, output, error) per MCP tool handler contract.
// The returned output contains the reversed/mirrored input text.
//
// If the context is canceled, it returns an error. This tool doesn’t care who
// called it, so the CallToolRequest parameter is unused.
func handleReverse(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	input MirrorInput,
) (*mcp.CallToolResult, MirrorOutput, error) {
	err := ctx.Err()
	if err != nil {
		return nil, MirrorOutput{}, wrapError(err, "request canceled")
	}

	// This is the core function of this tool: reverses the input text
	// If cancellation during the process (reversal) is needed, consider using
	// `select` with `ctx.Done()` channel in a loop over grapheme clusters.
	outputText := uniseg.ReverseString(input.Text)

	// log if debug mode is enabled (fileLogDefault = true or env var is set)
	debugLog("LOG: original text:", input.Text, "=> mirrored text:", outputText)

	return nil, MirrorOutput{Text: outputText}, nil
}
