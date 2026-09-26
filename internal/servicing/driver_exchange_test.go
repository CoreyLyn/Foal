package servicing

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func wirePackage(published string) wirePackageIdentity {
	return wirePackageIdentity{PublishedName: published, OriginalName: "nv_dispi.inf", Provider: "NVIDIA", DriverDate: "01/01/2025", DriverVersion: "32.0.15.1000"}
}

func TestValidateRequestDriverPackageCapability(t *testing.T) {
	valid := pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteDriverPackageCleanup,
		Packages: []wirePackageIdentity{wirePackage("oem10.inf"), wirePackage("oem11.inf")}}
	if err := validateRequest(valid, "n"); err != nil {
		t.Fatalf("valid driver request rejected: %v", err)
	}
	tooMany := make([]wirePackageIdentity, 257)
	for i := range tooMany {
		tooMany[i] = wirePackage(fmt.Sprintf("oem%d.inf", i+1))
	}
	noIdentity := wirePackage("oem10.inf")
	noIdentity.DriverVersion = ""
	pathOriginal := wirePackage("oem10.inf")
	pathOriginal.OriginalName = `C:\Windows\INF\nv_dispi.inf`
	for name, packages := range map[string][]wirePackageIdentity{
		"empty":         nil,
		"path":          {wirePackage(`C:\Windows\INF\oem10.inf`)},
		"traversal":     {wirePackage(`..\oem10.inf`)},
		"inbox":         {wirePackage("display.inf")},
		"duplicate":     {wirePackage("oem10.inf"), wirePackage("OEM10.INF")},
		"too many":      tooMany,
		"no identity":   {noIdentity},
		"path original": {pathOriginal},
	} {
		req := valid
		req.Packages = packages
		if err := validateRequest(req, "n"); err == nil {
			t.Fatalf("%s driver request accepted", name)
		}
	}
	componentWithPackages := pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteComponentStoreCleanup,
		Packages: []wirePackageIdentity{wirePackage("oem10.inf")}}
	if err := validateRequest(componentWithPackages, "n"); err == nil {
		t.Fatal("component-store request carrying packages accepted")
	}
}

func TestDriverPackageExchangeRoundTrip(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	var received []wirePackageIdentity
	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchangeRequest(helperConn, "n", func(req pipeRequest) pipeResponse {
			received = append([]wirePackageIdentity(nil), req.Packages...)
			observed := int64(2048)
			return responseFromDriverCleanup(clean.DriverPackageCleanupResult{
				Outcome: clean.ServicingOutcomeCompleted,
				Packages: []clean.ServicingDriverPackage{
					{PublishedName: "oem10.inf", Outcome: clean.DriverPackageOutcomeRemoved},
					{PublishedName: "oem11.inf", Outcome: clean.DriverPackageOutcomeNotEligible},
				},
				ObservedFreeBytes: &observed,
			})
		})
	}()

	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	sentPackages := []wirePackageIdentity{wirePackage("oem10.inf"), wirePackage("oem11.inf")}
	resp, sent, err := serverExchangeRequest(serverConn, pipeRequest{
		Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteDriverPackageCleanup,
		Packages: sentPackages,
	})
	if err != nil || !sent {
		t.Fatalf("exchange: sent=%v err=%v", sent, err)
	}
	if err := <-helperErr; err != nil {
		t.Fatalf("helper: %v", err)
	}
	if len(received) != 2 || received[0] != sentPackages[0] || received[1] != sentPackages[1] {
		t.Fatalf("helper received %#v", received)
	}
	ids := identitiesFromWire(received)
	if ids[0].PublishedName != "oem10.inf" || ids[0].DriverVersion != "32.0.15.1000" || ids[0].OriginalName != "nv_dispi.inf" {
		t.Fatalf("identities = %#v", ids)
	}
	res := driverCleanupResultFromResponse(resp)
	if res.Outcome != clean.ServicingOutcomeCompleted || len(res.Packages) != 2 ||
		res.Packages[0].Outcome != clean.DriverPackageOutcomeRemoved || res.Packages[1].Outcome != clean.DriverPackageOutcomeNotEligible {
		t.Fatalf("result = %#v", res)
	}
	if res.ObservedFreeBytes == nil || *res.ObservedFreeBytes != 2048 {
		t.Fatalf("observed = %v", res.ObservedFreeBytes)
	}
}

func TestDriverPackageExchangeInvalidRequestRunsNothing(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	ran := false
	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchangeRequest(helperConn, "n", func(pipeRequest) pipeResponse {
			ran = true
			return pipeResponse{Version: protocolVersion}
		})
	}()
	_ = serverConn.SetDeadline(time.Now().Add(5 * time.Second))
	if err := writeMessage(serverConn, pipeRequest{
		Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteDriverPackageCleanup,
		Packages: []wirePackageIdentity{wirePackage(`..\evil.inf`)},
	}); err != nil {
		t.Fatalf("write: %v", err)
	}
	select {
	case err := <-helperErr:
		if err == nil {
			t.Fatal("helper accepted an invalid package list")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("helper did not return")
	}
	if ran {
		t.Fatal("helper ran a capability for an invalid request")
	}
}
