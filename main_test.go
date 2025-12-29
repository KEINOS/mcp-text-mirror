package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rivo/uniseg"
	"github.com/stretchr/testify/require"
)

var errTest = errors.New("test error")

// mockLogger is a mock implementation of CustomLogger for testing.
type mockLogger struct {
	Fn func(v ...any)
}

// Fatal calls the Fn mock function. It is an implementation of CustomLogger.
func (m mockLogger) Fatal(v ...any) {
	m.Fn(v...)
}

// Print calls the Fn function instead of printing. It is an implementation of
// CustomLogger.
func (m mockLogger) Print(v ...any) {
	m.Fn(v...)
}

// ----------------------------------------------------------------------------
//  main
// ----------------------------------------------------------------------------

//nolint:paralleltest // because of monkey patching
func Test_main_failure(t *testing.T) {
	// Replace logger with one that panics instead of exiting the process.
	originalLogger := logger

	defer func() {
		logger = originalLogger
	}()

	logger = mockLogger{
		Fn: log.Panic,
	}

	// override context to cause failure
	defer func() {
		defaultCtx = context.Background()
	}()

	// setting to nil to simulate failure
	//nolint:fatcontext // to simulate failure
	defaultCtx = nil

	require.Panics(t, func() {
		// Run main with a context that will cause failure
		main()
	}, "Expected main to panic on error")
}

// ----------------------------------------------------------------------------
//  IsDebugMode
// ----------------------------------------------------------------------------

func Test_IsDebugMode(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		// Ensure env variable is not set
		t.Setenv(envNameDebug, "")

		// Clear env variable
		expect := fileLogDefault
		actual := IsDebugMode()

		require.Equal(t, expect, actual,
			"IsDebugMode should return the default fileLogDefault value when env var is not set")
	})

	t.Run("env_var_set", func(t *testing.T) {
		// Set env variable to enable debug mode
		t.Setenv(envNameDebug, "debug.log")

		actual := IsDebugMode()

		require.True(t, actual,
			"IsDebugMode should return true when env var is set")
	})
}

// ----------------------------------------------------------------------------
//  GetLogPath
// ----------------------------------------------------------------------------

func Test_GetLogPath(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		// Ensure env variable is not set
		t.Setenv(envNameDebug, "")

		expect := filepath.Clean(filepath.Join(logDir, logName))
		actual := GetLogPath()

		require.Equal(t, expect, actual,
			"GetLogPath should return the default log path when env var is not set")
	})

	t.Run("env_var_set", func(t *testing.T) {
		// Set env variable to specify log path
		customPath := "/custom/path/debug.log"
		t.Setenv(envNameDebug, customPath)

		actual := GetLogPath()

		require.Equal(t, filepath.Clean(customPath), actual,
			"GetLogPath should return the env var value when it is set")
	})
}

// ----------------------------------------------------------------------------
//  GetServiceVersion
// ----------------------------------------------------------------------------

var dataGetServiceVersion = []struct {
	name        string
	hasInfo     bool
	mainVersion string
	settings    []debug.BuildSetting
	ok          bool
	expected    string
}{
	{
		name:        "has_version_and_revision",
		hasInfo:     true,
		mainVersion: "v1.2.3",
		settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
		},
		ok:       true,
		expected: "v1.2.3 (abcdef1)",
	},
	{
		name:        "no_version_has_revision",
		hasInfo:     true,
		mainVersion: "",
		settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: "abcdef1234567890"},
		},
		ok:       true,
		expected: "abcdef1 (devel)",
	},
	{
		name:        "has_version_no_revision",
		hasInfo:     true,
		mainVersion: "v2.0.0",
		settings: []debug.BuildSetting{
			{Key: "some.other.key", Value: "value"},
		},
		ok:       true,
		expected: "v2.0.0 (unknown)",
	},
	{
		name:        "no_version_no_revision",
		hasInfo:     true,
		mainVersion: "",
		settings: []debug.BuildSetting{
			{Key: "some.other.key", Value: "value"},
		},
		ok:       true,
		expected: "unknown (devel)",
	},
	{
		name:        "no_build_info",
		hasInfo:     false,
		mainVersion: "",
		settings:    nil,
		ok:          false,
		expected:    "unknown (devel)",
	},
}

//nolint:paralleltest // because of monkey patching
func Test_GetServiceVersion(t *testing.T) {
	originalDebugReadBuildInfo := debugReadBuildInfo

	defer func() {
		debugReadBuildInfo = originalDebugReadBuildInfo
	}()

	for index, test := range dataGetServiceVersion {
		title := fmt.Sprintf("Test #%d: %s", index+1, test.name)

		t.Run(title, func(t *testing.T) {
			debugReadBuildInfo = func() (*debug.BuildInfo, bool) {
				if !test.hasInfo {
					return nil, false
				}

				bldInfo := new(debug.BuildInfo) // avoid exhaustruct lint error
				bldInfo.Main.Version = test.mainVersion
				bldInfo.Settings = test.settings

				return bldInfo, true
			}

			expect := test.expected
			actual := GetServiceVersion()

			require.Equal(t, expect, actual,
				"GetServiceVersion did not return expected version string")
		})
	}
}

// ----------------------------------------------------------------------------
//  exitOnError
// ----------------------------------------------------------------------------

//nolint:paralleltest // monkey patches global state
func Test_exitOnError(t *testing.T) {
	// Replace logger with one that panics instead of exiting the process.
	originalLogger := logger

	defer func() {
		logger = originalLogger
	}()

	logger = mockLogger{
		Fn: log.Panic,
	}

	err := errTest

	require.Panics(t, func() {
		exitOnError(err)
	}, "Expected exitOnError to panic on error")
}

func Test_exitOnError_nil(t *testing.T) {
	t.Parallel()

	// Should not panic when err is nil
	require.NotPanics(t, func() {
		exitOnError(nil)
	})
}

// ----------------------------------------------------------------------------
//  run
// ----------------------------------------------------------------------------

//nolint:paralleltest // because of monkey patching
func Test_run_success(t *testing.T) {
	orig := runServer

	defer func() { runServer = orig }()

	runServer = func(_ context.Context, _ *mcp.Server) error {
		return nil // success
	}

	err := run(context.Background())
	require.NoError(t, err)
}

func Test_run_error(t *testing.T) {
	t.Parallel()

	// Canceled context causes mcpServer.Run to return an error
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	require.NotPanics(t, func() {
		err := run(ctx)

		require.Error(t, err)
		require.ErrorIs(t, err, context.Canceled)
	})
}

// ----------------------------------------------------------------------------
//  newServer
// ----------------------------------------------------------------------------

func Test_newServer(t *testing.T) {
	t.Parallel()

	require.NotNil(t, newServer())
}

// ----------------------------------------------------------------------------
//  newLogger
// ----------------------------------------------------------------------------

func Test_newLogger_no_log(t *testing.T) {
	t.Parallel()

	logDirPath := t.TempDir()
	logFilePath := filepath.Join(logDirPath, t.Name()+".log")
	logger := newLogger(false, logFilePath)

	require.NotNil(t, logger)
	require.NoFileExists(t, logFilePath,
		"log file should not be created if 'toFile' if false")
}

func Test_newLogger_out_file(t *testing.T) {
	t.Parallel()

	logDirPath := t.TempDir()
	require.DirExists(t, logDirPath, "temp dir should exist")

	logFilePath := filepath.Clean(filepath.Join(logDirPath, t.Name()+".log"))
	logger := newLogger(true, logFilePath)

	require.NotNil(t, logger)

	const logMsg = "test log entry"

	// Log something to ensure the file is created
	logger.Print(logMsg)

	require.FileExists(t, logFilePath,
		"log file should be created if 'toFile' is true")

	content, err := os.ReadFile(logFilePath)
	require.NoError(t, err, "should be able to read the log file")

	require.Contains(t, string(content), logMsg,
		"log file should contain the logged entry")
}

// ----------------------------------------------------------------------------
//  handleReverse
// ----------------------------------------------------------------------------

func Test_handleReverse(t *testing.T) {
	t.Parallel()

	in := MirrorInput{Text: "Hello, World🙂"}
	_, out, err := handleReverse(context.Background(), nil, in)
	require.NoError(t, err)

	// Expect the same result as uniseg.ReverseString
	expect := uniseg.ReverseString(in.Text)
	require.Equal(t, expect, out.Text)
}

// Table-driven tests for edge cases: combining marks, ZWJ sequences, flags, and long strings.
func Test_handleReverse_edgeCases(t *testing.T) {
	t.Parallel()

	for index, test := range []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"single_rune", "A"},
		{"combining_marks", "e\u0301a\u0300"}, // é à
		{"zwj_sequence", "👩‍👩‍👦"},
		{"flags", "🇯🇵🇺🇸"},
		{"long", strings.Repeat("🙂🙃👩‍💻こんにちはABC123", 100)},
		{"combining_long", strings.Repeat("e\u0301\u0323o\u0302", 200)},
	} {
		title := fmt.Sprintf("%d_%s", index+1, test.name)

		t.Run(title, func(t *testing.T) {
			t.Parallel()

			// Call the handler and compare with uniseg's result
			_, out, err := handleReverse(
				context.Background(),
				nil,
				MirrorInput{Text: test.input},
			)
			require.NoError(t, err)

			expect := uniseg.ReverseString(test.input)
			require.Equal(t, expect, out.Text)

			// Reversing twice should restore the original string
			_, out2, err := handleReverse(
				context.Background(),
				nil,
				MirrorInput(out),
			)

			require.NoError(t, err)
			require.Equal(t, test.input, out2.Text)
		})
	}
}

func Test_handleReverse_cancelled(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	// cancel before calling to simulate early cancellation
	cancel()

	_, _, err := handleReverse(ctx, nil, MirrorInput{Text: "ignored"})
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

// ----------------------------------------------------------------------------
//  debugLog
// ----------------------------------------------------------------------------

func Test_debugLog(t *testing.T) {
	originalLogger := logger

	defer func() {
		logger = originalLogger
	}()

	var loggedMessages []string // log to trace messages for testing

	logger = mockLogger{
		Fn: func(v ...any) {
			loggedMessages = append(loggedMessages, fmt.Sprint(v...))
		},
	}

	t.Run("debug_mode_enabled", func(t *testing.T) {
		// Enable debug mode
		t.Setenv(envNameDebug, "debug.log")

		loggedMessages = nil // reset

		debugLog("Debug message 1:", 123)
		debugLog("Debug message 2:", true)

		require.Len(t, loggedMessages, 2,
			"Expected 2 log messages when debug mode is enabled")
		require.Contains(t, loggedMessages[0], "Debug message 1:123")
		require.Contains(t, loggedMessages[1], "Debug message 2:true")
	})

	t.Run("debug_mode_disabled", func(t *testing.T) {
		// Disable debug mode
		t.Setenv(envNameDebug, "")

		loggedMessages = nil // reset

		debugLog("Debug message 1:", 123)
		debugLog("Debug message 2:", true)

		require.Empty(t, loggedMessages,
			"Expected no log messages when debug mode is disabled")
	})
}

// ----------------------------------------------------------------------------
//  wrapError
// ----------------------------------------------------------------------------

func Test_wrapError(t *testing.T) {
	t.Parallel()

	for index, test := range []struct {
		name      string
		err       error
		msg       string
		args      []any
		wantNil   bool
		wantMsg   string
		wantInner error
	}{
		{
			name:      "nil_error_returns_nil",
			err:       nil,
			msg:       "should not appear",
			args:      nil,
			wantNil:   true,
			wantMsg:   "",
			wantInner: nil,
		},
		{
			name:      "error_without_format_args",
			err:       errTest,
			msg:       "operation failed",
			args:      nil,
			wantNil:   false,
			wantMsg:   "operation failed: test error",
			wantInner: errTest,
		},
		{
			name:      "error_with_format_args",
			err:       errTest,
			msg:       "failed at step %d with code %s",
			args:      []any{42, "ERR001"},
			wantNil:   false,
			wantMsg:   "failed at step 42 with code ERR001: test error",
			wantInner: errTest,
		},
	} {
		title := fmt.Sprintf("Test #%d: %s", index+1, test.name)

		t.Run(title, func(t *testing.T) {
			t.Parallel()

			got := wrapError(test.err, test.msg, test.args...)
			if test.wantNil {
				require.NoError(t, got)

				return
			}

			require.Error(t, got)
			require.ErrorContains(t, got, test.wantMsg)
			require.ErrorIs(t, got, test.wantInner)
		})
	}
}
