package pathsafe

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

type Reason struct {
	Code    string
	Message string
}

func ValidateDeletePath(path string) (Reason, bool) {
	if strings.TrimSpace(path) == "" {
		return reject("empty_path", "delete path cannot be empty")
	}

	normalized := stripLongPathPrefix(path)
	cleaned := strings.ToLower(filepath.Clean(normalized))

	if strings.HasPrefix(cleaned, `\\`) {
		return reject("unc_path", "UNC paths require explicit handling and cannot be cleaned by default")
	}
	if !filepath.IsAbs(cleaned) {
		return reject("relative_path", "delete path must be absolute")
	}
	if containsShortNameSegment(cleaned) {
		return reject("short_name_path", "8.3 short-name paths cannot be used for cleanup safety decisions")
	}

	for _, protected := range []string{
		`c:\windows`,
		`c:\program files`,
		`c:\program files (x86)`,
	} {
		if cleaned == protected || strings.HasPrefix(cleaned, protected+`\`) {
			return reject("protected_path", "protected system paths cannot be cleaned by Foal")
		}
	}

	info, err := os.Lstat(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while validating delete path")
		}
		return reject("stat_failed", "delete path could not be inspected before cleanup")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return reject("reparse_point", "reparse points cannot be cleaned by default")
	}

	reparse, err := hasReparseAttribute(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while reading Windows file attributes")
		}
		return reject("stat_failed", "Windows file attributes could not be inspected before cleanup")
	}
	if reparse {
		return reject("reparse_point", "reparse points cannot be cleaned by default")
	}

	hardlink, err := hasMultipleHardlinks(normalized)
	if err != nil {
		if errors.Is(err, fs.ErrPermission) {
			return reject("permission_denied", "permission denied while checking hardlink count")
		}
		return reject("stat_failed", "hardlink count could not be inspected before cleanup")
	}
	if hardlink {
		return reject("hardlink_path", "files with multiple hardlinks cannot be cleaned by default")
	}

	return Reason{}, true
}

func reject(code, message string) (Reason, bool) {
	return Reason{Code: code, Message: message}, false
}

func stripLongPathPrefix(path string) string {
	if strings.HasPrefix(path, `\\?\UNC\`) {
		return `\\` + strings.TrimPrefix(path, `\\?\UNC\`)
	}
	return strings.TrimPrefix(path, `\\?\`)
}

func containsShortNameSegment(path string) bool {
	for _, segment := range strings.FieldsFunc(path, func(r rune) bool {
		return r == '\\' || r == '/'
	}) {
		if strings.Contains(segment, "~") {
			return true
		}
	}
	return false
}

func hasReparseAttribute(path string) (bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	attrs, err := syscall.GetFileAttributes(ptr)
	if err != nil {
		return false, err
	}
	return attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}

func hasMultipleHardlinks(path string) (bool, error) {
	ptr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false, err
	}
	handle, err := syscall.CreateFile(
		ptr,
		0,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil,
		syscall.OPEN_EXISTING,
		syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(handle)

	var data syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(handle, &data); err != nil {
		return false, err
	}
	if data.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return false, nil
	}
	return data.NumberOfLinks > 1, nil
}
