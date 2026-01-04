package main

import (
	"context"
	"errors"
	"fmt"
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

// =============================================================================
//  Helpers for testing
// =============================================================================

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

// =============================================================================
//  Data providers for tests
// =============================================================================

// dataToReverse provides comprehensive test cases for reverse/mirror operations.
// Each case includes input and expected output verified against uniseg.ReverseString.
// These are used by both unit tests and as seed corpus for fuzz tests.
//
//nolint:gochecknoglobals // intentional: shared test data provider
var dataToReverse = []struct {
	name     string
	input    string
	expected string
}{
	// -------------------------------------------------------------------------
	// Basic cases
	// -------------------------------------------------------------------------
	{"empty", "", ""},
	{"single_ascii", "A", "A"},
	{"single_space", " ", " "},
	{"simple_ascii", "Hello, World!", "!dlroW ,olleH"},
	{"ascii_numbers", "12345", "54321"},
	{"ascii_mixed", "abc123XYZ", "ZYX321cba"},
	{"common accented words", "café résumé naïve", "evïan émusér éfac"},

	// -------------------------------------------------------------------------
	// Unicode: CJK and other scripts
	// -------------------------------------------------------------------------
	{"single_hiragana", "\u3042", "\u3042"},                                                  // あ
	{"hiragana_word", "\u3053\u3093\u306b\u3061\u306f", "\u306f\u3061\u306b\u3093\u3053"},    // こんにちは -> はちにんこ
	{"kanji", "\u4e16\u754c", "\u754c\u4e16"},                                                // 世界 -> 界世
	{"mixed_jp_ascii", "Hello\u4e16\u754c", "\u754c\u4e16olleH"},                             // Hello世界 -> 界世olleH
	{"korean", "\uc548\ub155", "\ub155\uc548"},                                               // 안녕 -> 녕안
	{"arabic", "\u0645\u0631\u062d\u0628\u0627", "\u0627\u0628\u062d\u0631\u0645"},           // مرحبا -> ابحرم
	{"thai", "\u0e2a\u0e27\u0e31\u0e2a\u0e14\u0e35", "\u0e14\u0e35\u0e2a\u0e27\u0e31\u0e2a"}, // สวัสดี -> ดีสวัส
	{ // こんにちは世界 -> 界世はちにんこ
		"mixed_jp_hiragana_kanji",
		"\u3053\u3093\u306b\u3061\u306f\u4e16\u754c",
		"\u754c\u4e16\u306f\u3061\u306b\u3093\u3053",
	},

	// -------------------------------------------------------------------------
	// Emoji: Basic
	// -------------------------------------------------------------------------
	{"single_emoji", "\U0001F642", "\U0001F642"},                       // 🙂
	{"multiple_emoji", "\U0001F642\U0001F643", "\U0001F643\U0001F642"}, // 🙂🙃 -> 🙃🙂
	{"emoji_mixed_ascii", "Hi\U0001F642Bye", "eyB\U0001F642iH"},
	{"emoji_hello_world", "Hello, World\U0001F642", "\U0001F642dlroW ,olleH"},

	// -------------------------------------------------------------------------
	// Emoji: Skin tone modifiers (Fitzpatrick scale)
	// -------------------------------------------------------------------------
	{ // 👋🏽 (medium skin)
		"skin_tone_wave",
		"\U0001F44B\U0001F3FD",
		"\U0001F44B\U0001F3FD",
	},
	{ // 👋🏻👋🏿
		"multiple_skin_tones",
		"\U0001F44B\U0001F3FB\U0001F44B\U0001F3FF",
		"\U0001F44B\U0001F3FF\U0001F44B\U0001F3FB",
	},

	// -------------------------------------------------------------------------
	// Emoji: ZWJ (Zero Width Joiner) sequences
	// -------------------------------------------------------------------------
	{ // 👩‍👩‍👦
		"zwj_family",
		"\U0001F469\u200D\U0001F469\u200D\U0001F466",
		"\U0001F469\u200D\U0001F469\u200D\U0001F466",
	},
	{ // 👩‍💻
		"zwj_technologist",
		"\U0001F469\u200D\U0001F4BB",
		"\U0001F469\u200D\U0001F4BB",
	},
	{ // 👩‍❤️‍👨
		"zwj_heart",
		"\U0001F469\u200D\u2764\uFE0F\u200D\U0001F468",
		"\U0001F469\u200D\u2764\uFE0F\u200D\U0001F468",
	},
	{ // 👩‍💻👨‍💻 -> 👨‍💻👩‍💻
		"zwj_multiple",
		"\U0001F469\u200D\U0001F4BB\U0001F468\u200D\U0001F4BB",
		"\U0001F468\u200D\U0001F4BB\U0001F469\u200D\U0001F4BB",
	},

	// -------------------------------------------------------------------------
	// Emoji: Regional indicator flags
	// -------------------------------------------------------------------------
	{"flag_jp", "\U0001F1EF\U0001F1F5", "\U0001F1EF\U0001F1F5"}, // 🇯🇵
	{"flag_us", "\U0001F1FA\U0001F1F8", "\U0001F1FA\U0001F1F8"}, // 🇺🇸
	{ // 🇯🇵🇺🇸 -> 🇺🇸🇯🇵
		"multiple_flags",
		"\U0001F1EF\U0001F1F5\U0001F1FA\U0001F1F8",
		"\U0001F1FA\U0001F1F8\U0001F1EF\U0001F1F5",
	},

	// -------------------------------------------------------------------------
	// Emoji: Tag sequences (subdivision flags)
	// -------------------------------------------------------------------------
	{ // 🏴󠁧󠁢󠁳󠁣󠁴󠁿 (Scotland flag)
		"flag_scotland",
		"\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
		"\U0001F3F4\U000E0067\U000E0062\U000E0073\U000E0063\U000E0074\U000E007F",
	},

	// -------------------------------------------------------------------------
	// Emoji: Keycap sequences
	// -------------------------------------------------------------------------
	{"keycap_1", "1\uFE0F\u20E3", "1\uFE0F\u20E3"}, // 1️⃣
	{ // 1️⃣2️⃣3️⃣ -> 3️⃣2️⃣1️⃣
		"keycap_sequence",
		"1\uFE0F\u20E32\uFE0F\u20E33\uFE0F\u20E3",
		"3\uFE0F\u20E32\uFE0F\u20E31\uFE0F\u20E3",
	},

	// -------------------------------------------------------------------------
	// Emoji: Variation selectors
	// -------------------------------------------------------------------------
	{"rainbow_flag", "\U0001F3F3\uFE0F\u200D\U0001F308", "\U0001F3F3\uFE0F\u200D\U0001F308"}, // 🏳️‍🌈
	{"heart_text_style", "\u2764\uFE0E", "\u2764\uFE0E"},                                     // ❤︎ (text style)
	{"heart_emoji_style", "\u2764\uFE0F", "\u2764\uFE0F"},                                    // ❤️ (emoji style)

	// -------------------------------------------------------------------------
	// Combining marks (diacritics)
	// -------------------------------------------------------------------------
	{"combining_acute", "e\u0301", "e\u0301"},                                       // é
	{"combining_grave", "a\u0300", "a\u0300"},                                       // à
	{"combining_two_chars", "e\u0301a\u0300", "a\u0300e\u0301"},                     // éà -> àé
	{"combining_multiple_marks", "o\u0302\u0323", "o\u0302\u0323"},                  // ộ (circumflex + dot below)
	{"combining_stacked", "a\u0300\u0301\u0302\u0303", "a\u0300\u0301\u0302\u0303"}, // multiple marks on single base
	{"precomposed_vs_combining", "\u00E9\u00E0", "\u00E0\u00E9"},                    // é à (precomposed) -> à é

	// -------------------------------------------------------------------------
	// Whitespace and control characters
	// -------------------------------------------------------------------------
	{"tabs", "\t\t\t", "\t\t\t"},
	{"spaces", "   ", "   "},
	{"newlines_lf", "\n\n", "\n\n"},
	{"mixed_whitespace", " \t ", " \t "},
	{"text_with_tabs", "a\tb\tc", "c\tb\ta"},

	// -------------------------------------------------------------------------
	// Mixed complex cases
	// -------------------------------------------------------------------------
	{"mixed_all", "Hello\U0001F642\u4e16\u754ce\u0301", "e\u0301\u754c\u4e16\U0001F642olleH"},
	{"sentence_jp", "\u3053\u3093\u306b\u3061\u306fWorld\U0001F30D", "\U0001F30DdlroW\u306f\u3061\u306b\u3093\u3053"},

	// -------------------------------------------------------------------------
	// Edge cases: Long strings
	// -------------------------------------------------------------------------
	{"long_ascii", strings.Repeat("abcdef", 100), strings.Repeat("fedcba", 100)},
	{ // 🙂🙃 x 50 -> 🙃🙂 x 50
		"long_emoji",
		strings.Repeat("\U0001F642\U0001F643", 50),
		strings.Repeat("\U0001F643\U0001F642", 50),
	},
	{ // mixed emoji, Japanese, ASCII repeated
		"long_mixed",
		strings.Repeat("\U0001F642\U0001F643\U0001F469\u200D\U0001F4BB\u3053\u3093\u306b\u3061\u306fABC123", 10),
		strings.Repeat("321CBA\u306f\u3061\u306b\u3093\u3053\U0001F469\u200D\U0001F4BB\U0001F643\U0001F642", 10),
	},
	{ // combining marks long (ẹ́ộ repeated)
		"long_combining",
		strings.Repeat("e\u0301\u0323o\u0302", 20),
		strings.Repeat("o\u0302e\u0301\u0323", 20),
	},

	// -------------------------------------------------------------------------
	// Edge cases: Palindromes (should equal themselves when reversed)
	// -------------------------------------------------------------------------
	{"palindrome_ascii", "racecar", "racecar"},
	{"palindrome_emoji", "\U0001F642\U0001F643\U0001F642", "\U0001F642\U0001F643\U0001F642"},
}

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

// =============================================================================
//  Unit tests
// =============================================================================

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
		Fn: func(v ...any) { panic(v[0]) },
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
		Fn: func(v ...any) { panic(v[0]) },
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

func Test_newLogger_file_open_failure(t *testing.T) {
	t.Parallel()

	// Use a path to a non-existent directory to force file open failure
	invalidPath := filepath.Join("/non-existent-dir-12345", "test.log")
	logger := newLogger(true, invalidPath)

	// Should still return a valid logger (fallback to stderr)
	require.NotNil(t, logger, "newLogger should return a valid logger even if file open fails")

	// Verify the logger works (writes to stderr, not to file)
	require.NotPanics(t, func() {
		logger.Print("test message")
	}, "logger should not panic when writing after file open failure")

	// The file should not exist since the directory doesn't exist
	require.NoFileExists(t, invalidPath,
		"log file should not be created when directory doesn't exist")
}

// ----------------------------------------------------------------------------
//  handleReverse
// ----------------------------------------------------------------------------

func Test_handleReverse(t *testing.T) {
	t.Parallel()

	for index, test := range dataToReverse {
		title := fmt.Sprintf("Test #%d: %s", index+1, test.name)

		t.Run(title, func(t *testing.T) {
			t.Parallel()

			in := MirrorInput{Text: test.input}
			_, out, err := handleReverse(context.Background(), nil, in)

			require.NoError(t, err)
			require.Equal(t, test.expected, out.Text,
				"Reversed text did not match expected output")
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

// =============================================================================
//  Fuzz testing
// =============================================================================

// ----------------------------------------------------------------------------
//  handleReverse
// ----------------------------------------------------------------------------

// FuzzHandleReverse performs fuzz testing on the handleReverse function.
//
// It ensures that it handles arbitrary UTF-8 input without panicking and
// maintains the involution property (reversing twice restores the original).
func FuzzHandleReverse(f *testing.F) {
	for _, data := range dataToReverse {
		f.Add(data.input) // add all predefined test inputs as seeds
	}

	f.Fuzz(func(t *testing.T, input string) {
		ctx := context.Background()

		var (
			out MirrorOutput
			err error
		)

		// Property 1: handleReverse should not panic and should not return error
		// for valid context
		require.NotPanics(t, func() {
			_, out, err = handleReverse(ctx, nil, MirrorInput{Text: input})
			require.NoError(t, err, "handleReverse should not return error for input: %q", input)
		})

		// Property 2: Output must match uniseg.ReverseString exactly
		actual := out.Text
		expect := uniseg.ReverseString(input)
		require.Equal(t, expect, actual, "handleReverse output mismatch for input: %q", input)

		// Property 3: Involution within uniseg's semantics - reversing twice
		// should produce the same result as uniseg.ReverseString applied twice.
		// Note: Due to combining mark handling, reverse(reverse(x)) may not equal x.
		_, out2, err := handleReverse(ctx, nil, MirrorInput(out))
		require.NoError(t, err, "second handleReverse should not return error for input: %q", input)

		expectedAfterDoubleReverse := uniseg.ReverseString(uniseg.ReverseString(input))
		require.Equal(t, expectedAfterDoubleReverse, out2.Text,
			"double reverse output mismatch for input: %q", input)
	})
}
