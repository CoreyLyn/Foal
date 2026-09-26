package driverstore

import (
	"fmt"
	"strings"
	"testing"
)

func nvidiaPackage(number int, date, version string, inUse bool) Package {
	return Package{
		PublishedName: fmt.Sprintf("oem%d.inf", number),
		OriginalName:  "nv_dispi.inf",
		Provider:      "NVIDIA",
		ClassGUID:     DisplayClassGUID,
		DriverDate:    date,
		DriverVersion: version,
		Bytes:         int64(number) * 1000,
		InUse:         inUse,
	}
}

func publishedNames(packages []Package) []string {
	names := make([]string, 0, len(packages))
	for _, pkg := range packages {
		names = append(names, pkg.PublishedName)
	}
	return names
}

// TestSelectSupersededResearchHostLayout mirrors the research host: the newest
// NVIDIA package is in use, so every older package is a candidate, oldest first.
func TestSelectSupersededResearchHostLayout(t *testing.T) {
	inv := Inventory{Packages: []Package{
		nvidiaPackage(213, "09/17/2026", "32.0.16.1714", true),
		nvidiaPackage(84, "11/06/2024", "32.0.15.6614", false),
		nvidiaPackage(209, "09/04/2026", "32.0.16.1692", false),
		nvidiaPackage(101, "10/15/2024", "32.0.15.6603", false),
		nvidiaPackage(196, "05/19/2026", "32.0.16.1047", false),
	}}
	got := publishedNames(SelectSuperseded(inv))
	want := []string{"oem101.inf", "oem84.inf", "oem196.inf", "oem209.inf"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestSelectSupersededKeepsInUseOlderPackageAndNewest(t *testing.T) {
	// A device rolled back to an older package: keep it and the newest.
	inv := Inventory{Packages: []Package{
		nvidiaPackage(10, "01/01/2025", "32.0.15.1000", false),
		nvidiaPackage(11, "02/01/2025", "32.0.15.2000", true),
		nvidiaPackage(12, "03/01/2025", "32.0.15.3000", false),
		nvidiaPackage(13, "04/01/2025", "32.0.15.4000", false),
	}}
	got := publishedNames(SelectSuperseded(inv))
	want := []string{"oem10.inf", "oem12.inf"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("candidates = %v, want %v", got, want)
	}
}

func TestSelectSupersededRanksByDateBeforeVersion(t *testing.T) {
	// Windows prefers the more recent DriverVer date, even with a lower version.
	older := Package{PublishedName: "oem157.inf", OriginalName: "gamevieweridddriver.inf", Provider: "GameViewer",
		ClassGUID: DisplayClassGUID, DriverDate: "05/16/2024", DriverVersion: "17.13.25.697"}
	newer := Package{PublishedName: "oem188.inf", OriginalName: "gamevieweridddriver.inf", Provider: "GameViewer",
		ClassGUID: DisplayClassGUID, DriverDate: "02/28/2026", DriverVersion: "15.6.5.199"}
	got := publishedNames(SelectSuperseded(Inventory{Packages: []Package{older, newer}}))
	if strings.Join(got, ",") != "oem157.inf" {
		t.Fatalf("candidates = %v, want [oem157.inf]", got)
	}
}

func TestSelectSupersededTieBreaksByVersionThenPublishedNumber(t *testing.T) {
	a := nvidiaPackage(20, "06/01/2025", "32.0.15.7000", false)
	b := nvidiaPackage(21, "06/01/2025", "32.0.15.7100", false)
	got := publishedNames(SelectSuperseded(Inventory{Packages: []Package{b, a}}))
	if strings.Join(got, ",") != "oem20.inf" {
		t.Fatalf("version tie-break candidates = %v, want [oem20.inf]", got)
	}

	c := nvidiaPackage(30, "06/01/2025", "32.0.15.7000", false)
	d := nvidiaPackage(31, "06/01/2025", "32.0.15.7000", false)
	got = publishedNames(SelectSuperseded(Inventory{Packages: []Package{d, c}}))
	if strings.Join(got, ",") != "oem30.inf" {
		t.Fatalf("number tie-break candidates = %v, want [oem30.inf]", got)
	}
}

func TestSelectSupersededIgnoresNonDisplayInboxAndSingletons(t *testing.T) {
	audio := nvidiaPackage(40, "01/01/2025", "1.0.0.0", false)
	audio.ClassGUID = "{4d36e96c-e325-11ce-bfc1-08002be10318}"
	audioNew := nvidiaPackage(41, "02/01/2025", "1.0.0.1", false)
	audioNew.ClassGUID = audio.ClassGUID
	inbox := nvidiaPackage(50, "01/01/2025", "1.0.0.0", false)
	inbox.PublishedName = "display.inf"
	inboxNew := nvidiaPackage(51, "02/01/2025", "1.0.0.1", false)
	inboxNew.PublishedName = "display2.inf"
	single := Package{PublishedName: "oem60.inf", OriginalName: "other.inf", Provider: "Other",
		ClassGUID: DisplayClassGUID, DriverDate: "01/01/2020", DriverVersion: "1.0"}

	got := SelectSuperseded(Inventory{Packages: []Package{audio, audioNew, inbox, inboxNew, single}})
	if len(got) != 0 {
		t.Fatalf("candidates = %v, want none", publishedNames(got))
	}
}

func TestSelectSupersededSeparatesFamiliesByProviderAndOriginalName(t *testing.T) {
	a := nvidiaPackage(70, "01/01/2025", "1.0", false)
	b := nvidiaPackage(71, "02/01/2025", "1.1", false)
	b.Provider = "Other vendor"
	c := nvidiaPackage(72, "03/01/2025", "1.2", false)
	c.OriginalName = "nv_other.inf"
	if got := SelectSuperseded(Inventory{Packages: []Package{a, b, c}}); len(got) != 0 {
		t.Fatalf("candidates = %v, want none across distinct families", publishedNames(got))
	}
}

func TestSelectSupersededFailsFamilyClosedOnUnparseableDriverVer(t *testing.T) {
	for _, bad := range []Package{
		nvidiaPackage(82, "", "32.0.15.7000", false),
		nvidiaPackage(82, "13/01/2025", "32.0.15.7000", false),
		nvidiaPackage(82, "02/30/2025", "32.0.15.7000", false),
		nvidiaPackage(82, "06/01/2025", "", false),
		nvidiaPackage(82, "06/01/2025", "32.0.15.70000", false),
		nvidiaPackage(82, "06/01/2025", "1.2.3.4.5", false),
	} {
		inv := Inventory{Packages: []Package{
			nvidiaPackage(80, "01/01/2025", "32.0.15.1000", false),
			nvidiaPackage(81, "02/01/2025", "32.0.15.2000", true),
			bad,
		}}
		if got := SelectSuperseded(inv); len(got) != 0 {
			t.Fatalf("DriverVer %q,%q: candidates = %v, want family skipped", bad.DriverDate, bad.DriverVersion, publishedNames(got))
		}
	}
}

func TestSelectSupersededAcceptsHyphenDates(t *testing.T) {
	inv := Inventory{Packages: []Package{
		nvidiaPackage(90, "01-01-2025", "1.0", false),
		nvidiaPackage(91, "02-01-2025", "1.1", true),
	}}
	if got := publishedNames(SelectSuperseded(inv)); strings.Join(got, ",") != "oem90.inf" {
		t.Fatalf("candidates = %v, want [oem90.inf]", got)
	}
}

func TestValidPublishedName(t *testing.T) {
	for _, name := range []string{"oem1.inf", "OEM213.INF", "oem999999.inf"} {
		if !ValidPublishedName(name) {
			t.Fatalf("%q should be valid", name)
		}
	}
	for _, name := range []string{"", "oem.inf", "oem1234567.inf", "oem1.inf.bak", `..\oem1.inf`, `C:\Windows\INF\oem1.inf`, "nv_dispi.inf", "oem1.pnf", " oem1.inf"} {
		if ValidPublishedName(name) {
			t.Fatalf("%q should be invalid", name)
		}
	}
}

func TestValidateRequest(t *testing.T) {
	if err := ValidateRequest([]string{"oem1.inf", "oem2.inf"}); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	if err := ValidateRequest(nil); err == nil {
		t.Fatal("empty request accepted")
	}
	if err := ValidateRequest([]string{"oem1.inf", "OEM1.INF"}); err == nil {
		t.Fatal("duplicate request accepted")
	}
	if err := ValidateRequest([]string{"oem1.inf", `..\evil.inf`}); err == nil {
		t.Fatal("invalid name accepted")
	}
	tooMany := make([]string, MaxRequestPackages+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("oem%d.inf", i+1)
	}
	if err := ValidateRequest(tooMany); err == nil {
		t.Fatal("oversized request accepted")
	}
	if err := ValidateRequest(tooMany[:MaxRequestPackages]); err != nil {
		t.Fatalf("request at the limit rejected: %v", err)
	}
}

func TestIdentityMatchesExactPackage(t *testing.T) {
	pkg := Package{PublishedName: "oem10.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/02/2025", DriverVersion: "32.0.15.1000"}
	id := IdentityOf(pkg)
	if !id.Matches(Package{PublishedName: "OEM10.INF", OriginalName: " NV_DISPI.INF", Provider: "nvidia", DriverDate: "01/02/2025", DriverVersion: "32.0.15.1000"}) {
		t.Fatal("identity should match case-insensitively after trimming")
	}
	for name, other := range map[string]Package{
		"reused name":   {PublishedName: "oem10.inf", OriginalName: "u0123456.inf", Provider: "Advanced Micro Devices, Inc.", DriverDate: "01/02/2025", DriverVersion: "32.0.15.1000"},
		"other version": {PublishedName: "oem10.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/02/2025", DriverVersion: "32.0.15.2000"},
		"other date":    {PublishedName: "oem10.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "02/02/2025", DriverVersion: "32.0.15.1000"},
		"other name":    {PublishedName: "oem11.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/02/2025", DriverVersion: "32.0.15.1000"},
	} {
		if id.Matches(other) {
			t.Fatalf("%s matched: %#v", name, other)
		}
	}
}

func TestValidateIdentities(t *testing.T) {
	valid := Identity{PublishedName: "oem10.inf", OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/02/2025", DriverVersion: "32.0.15.1000"}
	if err := ValidateIdentities([]Identity{valid}); err != nil {
		t.Fatalf("valid identity rejected: %v", err)
	}
	for name, mutate := range map[string]func(*Identity){
		"bad name":         func(id *Identity) { id.PublishedName = `..\oem10.inf` },
		"missing original": func(id *Identity) { id.OriginalName = " " },
		"path original":    func(id *Identity) { id.OriginalName = `C:\Windows\INF\nv_dispi.inf` },
		"missing date":     func(id *Identity) { id.DriverDate = "" },
		"missing version":  func(id *Identity) { id.DriverVersion = "" },
		"long provider":    func(id *Identity) { id.Provider = strings.Repeat("x", maxIdentityFieldLength+1) },
	} {
		id := valid
		mutate(&id)
		if err := ValidateIdentities([]Identity{id}); err == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	if err := ValidateIdentities([]Identity{valid, valid}); err == nil {
		t.Fatal("duplicate identity accepted")
	}
}
