package servicing

import (
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/CoreyLyn/Foal/internal/clean"
)

func TestValidateRequestDriverPackageCapability(t *testing.T) {
	valid := pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteDriverPackageCleanup, Packages: []string{"oem10.inf", "oem11.inf"}}
	if err := validateRequest(valid, "n"); err != nil {
		t.Fatalf("valid driver request rejected: %v", err)
	}
	tooMany := make([]string, 257)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("oem%d.inf", i+1)
	}
	for name, packages := range map[string][]string{
		"empty":     nil,
		"path":      {`C:\Windows\INF\oem10.inf`},
		"traversal": {`..\oem10.inf`},
		"inbox":     {"display.inf"},
		"duplicate": {"oem10.inf", "OEM10.INF"},
		"too many":  tooMany,
	} {
		req := valid
		req.Packages = packages
		if err := validateRequest(req, "n"); err == nil {
			t.Fatalf("%s driver request accepted", name)
		}
	}
	componentWithPackages := pipeRequest{Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteComponentStoreCleanup, Packages: []string{"oem10.inf"}}
	if err := validateRequest(componentWithPackages, "n"); err == nil {
		t.Fatal("component-store request carrying packages accepted")
	}
}

func TestDriverPackageExchangeRoundTrip(t *testing.T) {
	serverConn, helperConn := net.Pipe()
	defer serverConn.Close()
	defer helperConn.Close()

	var received []string
	helperErr := make(chan error, 1)
	go func() {
		helperErr <- helperExchangeRequest(helperConn, "n", func(req pipeRequest) pipeResponse {
			received = append([]string(nil), req.Packages...)
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
	resp, err := serverExchangeRequest(serverConn, pipeRequest{
		Version: protocolVersion, Nonce: "n", Capability: wireCapabilityExecuteDriverPackageCleanup,
		Packages: []string{"oem10.inf", "oem11.inf"},
	})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if err := <-helperErr; err != nil {
		t.Fatalf("helper: %v", err)
	}
	if strings.Join(received, ",") != "oem10.inf,oem11.inf" {
		t.Fatalf("helper received %v", received)
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
		Packages: []string{`..\evil.inf`},
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
