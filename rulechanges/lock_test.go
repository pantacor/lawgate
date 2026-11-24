package rulechanges

import (
	"testing"
)

func TestProofLocksAreRuleFiles(t *testing.T) {
	const custom = "rules/proofs.json"
	config := `{"lock":"rules/proofs.json","modules":[{"path":".","language":"node","laws":"LAWS.md","machines":false}]}`
	tree := sourceTree{paths: []string{"lawgate.json", "LAWS.lock"}, read: func(string) ([]byte, error) {
		return []byte(config), nil
	}}
	rules := classify(tree).rules
	for _, file := range []string{"LAWS.lock", custom} {
		if !rules[file] {
			t.Errorf("proof lock %q was omitted from rule files", file)
		}
	}
}
