// Package driverstore inventories third-party display driver packages in the
// Windows driver store and selects superseded, unused packages (ADR 0036).
//
// It never deletes driver-store files. Removal goes through
// SetupUninstallOEMInfW without SUOI_FORCEDELETE, which Windows refuses for any
// package a device (present or not) was installed with.
package driverstore

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// DisplayClassGUID is the Display adapter device setup class.
const DisplayClassGUID = "{4d36e968-e325-11ce-bfc1-08002be10318}"

// MaxRequestPackages bounds how many packages one removal request may name.
const MaxRequestPackages = 256

// ErrUnsupported is returned on platforms without a Windows driver store.
var ErrUnsupported = errors.New("driverstore: unsupported platform")

// Package is one third-party driver package. StoreDir never leaves this
// package's callers' process: results and records carry identifiers only.
type Package struct {
	PublishedName string
	OriginalName  string
	Provider      string
	ClassGUID     string
	// DriverDate and DriverVersion are the [Version] DriverVer fields as declared
	// (mm/dd/yyyy and w.x.y.z).
	DriverDate    string
	DriverVersion string
	Bytes         int64
	// InUse reports that some device node, present or not, lists this package as
	// its driver INF.
	InUse bool
}

// Inventory is a read-only snapshot of display-class third-party packages.
type Inventory struct {
	Packages []Package
}

// RemoveOutcome is the per-package result of a removal attempt.
type RemoveOutcome string

const (
	RemoveOutcomeRemoved RemoveOutcome = "removed"
	RemoveOutcomeInUse   RemoveOutcome = "in_use"
	RemoveOutcomeFailed  RemoveOutcome = "failed"
)

var publishedNamePattern = regexp.MustCompile(`(?i)^oem([0-9]{1,6})\.inf$`)

// ValidPublishedName reports whether name is a third-party published INF name
// (oem<digits>.inf). Paths, inbox INF names, and anything else are rejected.
func ValidPublishedName(name string) bool {
	return publishedNamePattern.MatchString(name)
}

// ValidateRequest checks a removal request: between 1 and MaxRequestPackages
// valid published names, unique case-insensitively.
func ValidateRequest(names []string) error {
	if len(names) == 0 {
		return errors.New("driverstore: empty removal request")
	}
	if len(names) > MaxRequestPackages {
		return fmt.Errorf("driverstore: removal request names %d packages, limit %d", len(names), MaxRequestPackages)
	}
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if !ValidPublishedName(name) {
			return fmt.Errorf("driverstore: invalid published name %q", name)
		}
		key := strings.ToLower(name)
		if _, dup := seen[key]; dup {
			return fmt.Errorf("driverstore: duplicate published name %q", name)
		}
		seen[key] = struct{}{}
	}
	return nil
}

// Identity is the confirmed identity of one package a removal request names:
// the published name plus the original INF name, provider, and DriverVer that
// were disclosed. A package is removed only while a fresh inventory still
// reports exactly this identity as a superseded candidate, so a reused
// published name never redirects removal to a different package.
type Identity struct {
	PublishedName string
	OriginalName  string
	Provider      string
	DriverDate    string
	DriverVersion string
}

// maxIdentityFieldLength bounds each identity string in a removal request.
const maxIdentityFieldLength = 128

// IdentityOf returns the identity of an inventory package.
func IdentityOf(pkg Package) Identity {
	return Identity{
		PublishedName: pkg.PublishedName,
		OriginalName:  pkg.OriginalName,
		Provider:      pkg.Provider,
		DriverDate:    pkg.DriverDate,
		DriverVersion: pkg.DriverVersion,
	}
}

// Matches reports whether pkg has exactly this identity, comparing trimmed
// fields case-insensitively.
func (id Identity) Matches(pkg Package) bool {
	return sameField(id.PublishedName, pkg.PublishedName) &&
		sameField(id.OriginalName, pkg.OriginalName) &&
		sameField(id.Provider, pkg.Provider) &&
		sameField(id.DriverDate, pkg.DriverDate) &&
		sameField(id.DriverVersion, pkg.DriverVersion)
}

func sameField(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

// ValidateIdentities checks an identity-bound removal request: the published
// names pass ValidateRequest, every identity names a bare original INF and a
// DriverVer, and every field is bounded.
func ValidateIdentities(ids []Identity) error {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, id.PublishedName)
	}
	if err := ValidateRequest(names); err != nil {
		return err
	}
	for _, id := range ids {
		if strings.TrimSpace(id.OriginalName) == "" || strings.ContainsAny(id.OriginalName, `\/:`) ||
			strings.TrimSpace(id.DriverDate) == "" || strings.TrimSpace(id.DriverVersion) == "" {
			return fmt.Errorf("driverstore: %q has an incomplete identity", id.PublishedName)
		}
		for _, field := range []string{id.OriginalName, id.Provider, id.DriverDate, id.DriverVersion} {
			if len(field) > maxIdentityFieldLength {
				return fmt.Errorf("driverstore: %q identity field exceeds %d bytes", id.PublishedName, maxIdentityFieldLength)
			}
		}
	}
	return nil
}

// driverRank orders packages in one family: DriverVer date first (Windows
// prefers the more recent date), then version, then published number.
type driverRank struct {
	date      time.Time
	version   [4]uint32
	published int
}

func (a driverRank) newerThan(b driverRank) bool {
	if !a.date.Equal(b.date) {
		return a.date.After(b.date)
	}
	for i := range a.version {
		if a.version[i] != b.version[i] {
			return a.version[i] > b.version[i]
		}
	}
	return a.published > b.published
}

func parseRank(pkg Package) (driverRank, bool) {
	date, ok := parseDriverDate(pkg.DriverDate)
	if !ok {
		return driverRank{}, false
	}
	version, ok := parseDriverVersion(pkg.DriverVersion)
	if !ok {
		return driverRank{}, false
	}
	match := publishedNamePattern.FindStringSubmatch(pkg.PublishedName)
	if match == nil {
		return driverRank{}, false
	}
	published, err := strconv.Atoi(match[1])
	if err != nil {
		return driverRank{}, false
	}
	return driverRank{date: date, version: version, published: published}, true
}

// parseDriverDate parses mm/dd/yyyy (a hyphen separator is also allowed).
func parseDriverDate(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '/' || r == '-' })
	if len(parts) != 3 {
		return time.Time{}, false
	}
	month, err1 := strconv.Atoi(parts[0])
	day, err2 := strconv.Atoi(parts[1])
	year, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil || month < 1 || month > 12 || day < 1 || day > 31 || year < 1980 || year > 9999 {
		return time.Time{}, false
	}
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	if date.Day() != day {
		return time.Time{}, false
	}
	return date, true
}

// parseDriverVersion parses w.x.y.z with one to four parts, each below 65536.
func parseDriverVersion(value string) ([4]uint32, bool) {
	var version [4]uint32
	parts := strings.Split(strings.TrimSpace(value), ".")
	if len(parts) == 0 || len(parts) > 4 {
		return version, false
	}
	for i, part := range parts {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || n > 65535 {
			return version, false
		}
		version[i] = uint32(n)
	}
	return version, true
}

func familyKey(pkg Package) string {
	return strings.ToLower(strings.TrimSpace(pkg.OriginalName)) + "\x00" +
		strings.ToLower(strings.TrimSpace(pkg.Provider)) + "\x00" +
		strings.ToLower(strings.TrimSpace(pkg.ClassGUID))
}

// SelectSuperseded returns the packages that are safe candidates for removal:
// third-party (oem<digits>.inf) Display-class packages that no device uses and
// that are not the newest package of their family (same original INF name,
// provider, and class GUID). Every in-use package and the newest package of each
// family are kept. A family with any unparseable DriverVer yields no candidates.
// Output is ordered by family, then oldest first.
func SelectSuperseded(inv Inventory) []Package {
	families := map[string][]Package{}
	var order []string
	seen := map[string]struct{}{}
	for _, pkg := range inv.Packages {
		if !ValidPublishedName(pkg.PublishedName) ||
			!strings.EqualFold(strings.TrimSpace(pkg.ClassGUID), DisplayClassGUID) ||
			strings.TrimSpace(pkg.OriginalName) == "" {
			continue
		}
		name := strings.ToLower(pkg.PublishedName)
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		key := familyKey(pkg)
		if _, ok := families[key]; !ok {
			order = append(order, key)
		}
		families[key] = append(families[key], pkg)
	}
	sort.Strings(order)

	var candidates []Package
	for _, key := range order {
		members := families[key]
		if len(members) < 2 {
			continue
		}
		ranks := make([]driverRank, len(members))
		valid := true
		for i, pkg := range members {
			rank, ok := parseRank(pkg)
			if !ok {
				valid = false
				break
			}
			ranks[i] = rank
		}
		if !valid {
			continue
		}
		newest := 0
		for i := 1; i < len(members); i++ {
			if ranks[i].newerThan(ranks[newest]) {
				newest = i
			}
		}
		indexes := make([]int, 0, len(members))
		for i := range members {
			if i != newest && !members[i].InUse {
				indexes = append(indexes, i)
			}
		}
		sort.Slice(indexes, func(a, b int) bool { return ranks[indexes[b]].newerThan(ranks[indexes[a]]) })
		for _, i := range indexes {
			candidates = append(candidates, members[i])
		}
	}
	return candidates
}
