/*******************************************************************************
*
* Copyright 2018 SAP SE
*
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You should have received a copy of the License along with this
* program. If not, you may obtain a copy of the License at
*
*     http://www.apache.org/licenses/LICENSE-2.0
*
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*
*******************************************************************************/

package limes

import (
	"reflect"
	"testing"

	"github.com/gophercloud/gophercloud"
)

func TestQuotaConstraintParsingSuccess(t *testing.T) {
	constraints, errs := NewQuotaConstraints(clusterForQuotaConstraintTest(), "fixtures/quota-constraint-valid.yaml")

	if len(errs) > 0 {
		t.Errorf("expected no parsing errors, got %d errors:\n", len(errs))
		for idx, err := range errs {
			t.Logf("[%d] %s\n", idx+1, err.Error())
		}
	}

	pointerTo := func(x uint64) *uint64 { return &x }

	expected := QuotaConstraintSet{
		Domains: map[string]QuotaConstraints{
			"germany": {
				"service-one": {
					"things":       {Minimum: pointerTo(20), Maximum: pointerTo(30)},
					"capacity_MiB": {Minimum: pointerTo(10240)},
				},
				"service-two": {
					"capacity_MiB": {Minimum: pointerTo(1)},
				},
			},
			"poland": {
				"service-two": {
					"things": {Minimum: pointerTo(5)},
				},
			},
		},
		Projects: map[string]map[string]QuotaConstraints{
			"germany": {
				"berlin": {
					"service-one": {
						"things":       {Minimum: pointerTo(10)},
						"capacity_MiB": {Minimum: pointerTo(5120), Maximum: pointerTo(6144)},
					},
				},
				"dresden": {
					"service-one": {
						"things": {Minimum: pointerTo(5), Maximum: pointerTo(5)},
					},
					"service-two": {
						"capacity_MiB": {Minimum: pointerTo(1), Maximum: pointerTo(1)},
					},
				},
			},
			"poland": {
				"warsaw": {
					"service-two": {
						"things": {Expected: pointerTo(5), Maximum: pointerTo(10)},
					},
				},
			},
		},
	}
	if !reflect.DeepEqual(constraints, &expected) {
		t.Errorf("actual = %#v\n", constraints)
		t.Errorf("expected = %#v\n", expected)
	}
}

func clusterForQuotaConstraintTest() *Cluster {
	return &Cluster{
		QuotaPlugins: map[string]QuotaPlugin{
			"service-one": quotaConstraintTestPlugin{"service-one"},
			"service-two": quotaConstraintTestPlugin{"service-two"},
		},
	}
}

func TestQuotaConstraintParsingFailure(t *testing.T) {
	expectQuotaConstraintInvalid(t, "fixtures/quota-constraint-invalid.yaml",
		//ordered by appearance in fixture file
		`invalid constraints for domain germany: invalid constraint "not more than 20" for service-one/things: clause "not more than 20" should start with "at least", "at most" or "exactly"`,
		`invalid constraints for domain germany: invalid constraint "at least 10 GiB or something" for service-one/capacity_MiB: value "10 GiB or something" does not match expected format "<number> <unit>"`,
		`invalid constraints for domain germany: invalid constraint "at most 1 ounce" for service-two/capacity_MiB: cannot convert value from ounce to MiB because units are incompatible`,
		`missing domain name for project atlantis`,
		`invalid constraints for project germany/berlin: no such service: unknown`,
		`invalid constraints for project germany/dresden: invalid constraint "at least NaN" for service-one/things: strconv.ParseUint: parsing "NaN": invalid syntax`,
		`invalid constraints for project germany/dresden: invalid constraint "at least 4, at most 2" for service-two/things: constraint clauses cannot simultaneously be satisfied`,
		`invalid constraints for project poland/warsaw: invalid constraint "should be 4 MiB, should be 5 MiB" for service-two/capacity_MiB: cannot have multiple "should be" clauses in one constraint`,
	)

	expectQuotaConstraintInvalid(t, "fixtures/quota-constraint-inconsistent.yaml",
		`inconsistent constraints for domain germany: sum of "at least/exactly" project quotas (20480 MiB) for service-one/capacity_MiB exceeds "at least/exactly" domain quota (10240 MiB)`,
		`inconsistent constraints for domain poland: sum of "at least/exactly" project quotas (5) for service-two/things exceeds "at least/exactly" domain quota (0)`,
	)
}

func expectQuotaConstraintInvalid(t *testing.T, path string, expectedErrors ...string) {
	t.Helper()
	_, errs := NewQuotaConstraints(clusterForQuotaConstraintTest(), path)

	expectedErrs := make(map[string]bool)
	for _, err := range expectedErrors {
		expectedErrs[err] = true
	}

	for _, err := range errs {
		err := err.Error()
		if expectedErrs[err] {
			delete(expectedErrs, err) //check that one off the list
		} else {
			t.Errorf("got unexpected error: %s", err)
		}
	}
	for err := range expectedErrs {
		t.Errorf("did not get expected error: %s", err)
	}
}

type quotaConstraintTestPlugin struct {
	ServiceType string
}

func (p quotaConstraintTestPlugin) Init(client *gophercloud.ProviderClient) error {
	return nil
}
func (p quotaConstraintTestPlugin) ServiceInfo() ServiceInfo {
	return ServiceInfo{Type: p.ServiceType}
}
func (p quotaConstraintTestPlugin) Scrape(client *gophercloud.ProviderClient, clusterID, domainUUID, projectUUID string) (map[string]ResourceData, error) {
	return nil, nil
}
func (p quotaConstraintTestPlugin) SetQuota(client *gophercloud.ProviderClient, clusterID, domainUUID, projectUUID string, quotas map[string]uint64) error {
	return nil
}

func (p quotaConstraintTestPlugin) Resources() []ResourceInfo {
	return []ResourceInfo{
		{Name: "things", Unit: UnitNone},
		{Name: "capacity_MiB", Unit: UnitMebibytes},
	}
}

func TestQuotaConstraintToString(t *testing.T) {
	pointerTo := func(x uint64) *uint64 { return &x }

	type testcase struct {
		Input    QuotaConstraint
		Expected string
	}
	testcases := []testcase{
		{
			Input:    QuotaConstraint{Minimum: pointerTo(10)},
			Expected: "at least 10 MiB",
		}, {
			Input:    QuotaConstraint{Minimum: pointerTo(10), Maximum: pointerTo(20)},
			Expected: "at least 10 MiB, at most 20 MiB",
		}, {
			Input:    QuotaConstraint{Minimum: pointerTo(20), Maximum: pointerTo(20)},
			Expected: "exactly 20 MiB",
		},
	}

	for _, testcase := range testcases {
		actual := testcase.Input.ToString(UnitMebibytes)
		if actual != testcase.Expected {
			t.Errorf("expected %#v to serialize into %q, but got %q",
				testcase.Input, testcase.Expected, actual)
		}
	}
}
