package cmd

import (
	"fmt"
	"testing"
)

// TestResolveBalanceReader confirms BambooHR is preferred, the webhook is the fallback,
// and no configuration yields no reader.
func TestResolveBalanceReader(t *testing.T) {
	t.Parallel()
	tests := []struct {
		Name    string
		Env     map[string]string
		WantNil bool
		WantEmp string
	}{{ // Test 0: BambooHR configured yields a reader and its employee id.
		Name: "bamboohr", Env: map[string]string{
			"VAMOOSE_BAMBOOHR_SUBDOMAIN": "acme", "VAMOOSE_BAMBOOHR_API_KEY": "k",
			"VAMOOSE_BAMBOOHR_EMPLOYEE_ID": "42",
		}, WantEmp: "42",
	}, { // Test 1: A balance webhook is the fallback.
		Name: "webhook", Env: map[string]string{
			"VAMOOSE_BALANCE_WEBHOOK_URL": "https://hr.example.com/balance", "VAMOOSE_HRIS_EMPLOYEE_ID": "e7",
		}, WantEmp: "e7",
	}, { // Test 2: The generic employee id overrides the BambooHR one.
		Name: "generic emp id wins", Env: map[string]string{
			"VAMOOSE_BAMBOOHR_SUBDOMAIN": "acme", "VAMOOSE_BAMBOOHR_API_KEY": "k",
			"VAMOOSE_HRIS_EMPLOYEE_ID": "generic", "VAMOOSE_BAMBOOHR_EMPLOYEE_ID": "bamboo",
		}, WantEmp: "generic",
	}, { // Test 3: Nothing configured yields no reader.
		Name: "none", Env: map[string]string{}, WantNil: true,
	}}
	for testNum, test := range tests {
		t.Run(fmt.Sprintf("test %d", testNum), func(t *testing.T) {
			t.Parallel()
			reader, emp := resolveBalanceReader(func(k string) string { return test.Env[k] })
			if (reader == nil) != test.WantNil {
				t.Fatalf("%s: reader nil = %v, want %v", test.Name, reader == nil, test.WantNil)
			}
			if !test.WantNil && emp != test.WantEmp {
				t.Errorf("%s: employee = %q, want %q", test.Name, emp, test.WantEmp)
			}
		})
	}
}
