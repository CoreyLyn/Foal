package clean

import (
	"bufio"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/CoreyLyn/Foal/internal/core/pathsafe"
)

const protectionFileOverrideEnv = "FOAL_PROTECTION_FILE"

type ProtectionConfiguration struct {
	Validator   pathsafe.Validator
	Diagnostics []ProtectionDiagnostic
	LoadError   *StructuredIssue
}

type ProtectionDiagnostic struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Source      string `json:"source"`
	Line        int    `json:"line,omitempty"`
	Path        string `json:"path,omitempty"`
}

func LoadProtectionConfiguration() ProtectionConfiguration {
	path, explicit := protectionFilePath()
	content, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, fs.ErrNotExist) {
			return ProtectionConfiguration{Validator: pathsafe.NewValidator(nil), Diagnostics: []ProtectionDiagnostic{}}
		}
		loadError := issue("protection_file_load_failed", err.Error(), false, path, "")
		return ProtectionConfiguration{
			Validator:   pathsafe.NewValidator(nil),
			Diagnostics: []ProtectionDiagnostic{},
			LoadError:   &loadError,
		}
	}
	if !utf8.Valid(content) {
		loadError := issue("protection_file_invalid_utf8", "selected protection file is not valid UTF-8", false, path, "")
		return ProtectionConfiguration{
			Validator:   pathsafe.NewValidator(nil),
			Diagnostics: []ProtectionDiagnostic{},
			LoadError:   &loadError,
		}
	}

	paths := make([]string, 0)
	diagnostics := make([]ProtectionDiagnostic, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		entry := strings.TrimSpace(scanner.Text())
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		displayPath, _, reason, ok := pathsafe.NormalizeProtectionPath(entry)
		if !ok {
			diagnostics = append(diagnostics, ProtectionDiagnostic{
				Code:        reason.Code,
				Message:     reason.Message,
				Recoverable: true,
				Source:      path,
				Line:        lineNumber,
				Path:        entry,
			})
			continue
		}
		paths = append(paths, displayPath)
	}
	if err := scanner.Err(); err != nil {
		loadError := issue("protection_file_load_failed", err.Error(), false, path, "")
		return ProtectionConfiguration{
			Validator:   pathsafe.NewValidator(nil),
			Diagnostics: diagnostics,
			LoadError:   &loadError,
		}
	}
	return ProtectionConfiguration{
		Validator:   pathsafe.NewValidator(paths),
		Diagnostics: diagnostics,
	}
}

func protectionFilePath() (string, bool) {
	if override := strings.TrimSpace(os.Getenv(protectionFileOverrideEnv)); override != "" {
		return override, true
	}
	return filepath.Join(os.Getenv("APPDATA"), "Foal", "protection.txt"), false
}
