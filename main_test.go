package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cloudfoundry-community/go-cfclient"
)

func TestBrokerURL(t *testing.T) {
	url := "https://spring-cloud-broker.apps.pivotal.io/dashboard/p-config-server/GUID"
	expected := "https://spring-cloud-broker.apps.pivotal.io/cli/instances/GUID/parameters"
	if actual := commandLineURL(url); actual != expected {
		t.Errorf("wrong URL, want %q, got %q", expected, actual)
	}
}

func TestServicesByLabel(t *testing.T) {
	summaries := make([]cfclient.ServiceSummary, 3)
	summaries[0].ServicePlan.Service.Label = "label0"
	summaries[1].ServicePlan.Service.Label = "label1"
	summaries[2].ServicePlan.Service.Label = "label0"

	filtered := servicesByLabel("label0", summaries)
	l := len(filtered)
	if l != 2 {
		t.Errorf("want 2 plans with label0, got %d", l)
	}

	for i := 0; i < l; i++ {
		if label := filtered[i].ServicePlan.Service.Label; label != "label0" {
			t.Errorf("want 'label0', got %q", label)
		}
	}
}

func TestValidateConfigServerParams(t *testing.T) {
	for i, test := range []struct {
		gitRepos   bool
		encryptKey bool
		composite  bool
		json       string
	}{
		{false, false, false, `{}`},
		{true, false, false, `{ "git": { "repos": [] } }`},           // git.repos - invalid for SCS 3.x
		{false, true, false, `{ "encrypt": { "key": "foo" } }`},      // encrypt.key - valid only in 3.1.6+
		{false, false, true, `{ "composite": [ { "git": {} } ] }`},   // SCS 2.x composite git - invalid
		{false, false, true, `{ "composite": [ { "vault": {} } ] }`}, // SCS 2.x composite vault - invalid

		{false, false, false, `{ "composite": [ { "type": "vault" } ] }`}, // SCS 3.x style - valid

	} {
		t.Run(fmt.Sprintf("test %d", i), func(t *testing.T) {
			var params map[string]interface{}
			if err := json.Unmarshal([]byte(test.json), &params); err != nil {
				t.Errorf("invalid JSON: '%v': %v", test.json, err)
				return
			}
			usesGitRepos, usesEncryptKey, usesComposite := validateConfigServerParams(params)
			if usesGitRepos != test.gitRepos {
				t.Errorf("uses git repos want %v got %v", test.gitRepos, usesGitRepos)
			}
			if usesEncryptKey != test.encryptKey {
				t.Errorf("encrypt key want %v got %v", test.encryptKey, usesEncryptKey)
			}
			if usesComposite != test.composite {
				t.Errorf("composite want %v got %v", test.composite, usesComposite)
			}
		})
	}
}

func TestBuildUsages(t *testing.T) {
	space := &cfclient.Space{
		Name:             "test-space",
		OrganizationGuid: "org-guid",
	}
	c := &checker{
		orgNamesByGUID: map[string]string{
			"org-guid": "test-org",
		},
	}
	summary := cfclient.ServiceSummary{
		Name: "my-service-registry",
		Guid: "service-instance-guid",
	}
	summaries := []cfclient.ServiceSummary{summary}

	var usages []usage
	errors := c.buildUsages(space, summaries, &usages)

	if l := len(errors); l > 0 {
		t.Errorf("got %d errors: %v", l, errors)
	}

	if l := len(usages); l != 1 {
		t.Fatalf("want 1 usage, got %d", l)
	}

	assert := func(exp, actual string) {
		t.Helper()
		if exp != actual {
			t.Errorf("want %q, got %q", exp, actual)
		}
	}

	assert("test-org", usages[0].org)
	assert("test-space", usages[0].space)
	assert("my-service-registry", usages[0].serviceInstanceName)
}
