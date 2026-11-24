package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const anchoredLaw = "never-overspend"

const anchoredDoc = "## never-overspend\n\n- Source: owner decision 2026-11-20\n- Enforced by: test `TestPayment`\n- Subject: not yet. This fixture isolates lock and replay from mutation.\n- Bites: not yet. This fixture isolates lock and replay from mutation.\n- Status: agreed 2026-11-20\n"

const anchoredWalletSource = `package wallet

import "errors"

func Pay(balance, price int) error {
	if price > balance {
		return errors.New("insufficient funds")
	}
	return nil
}
`

const anchoredWalletProof = `package wallet

import "testing"

// TestPayment enforces never-overspend.
func TestPayment(t *testing.T) {
	checkPayment(t)
}
`

const anchoredWalletHelper = `package wallet

import "testing"

func checkPayment(t *testing.T) {
	if Pay(36, 900) == nil {
		t.Fatal("paid 900 with 36 in the wallet")
	}
}
`

func TestLockRejectsEditedTransitiveProof(t *testing.T) {
	root := anchoredFixture(t)
	rewriteWalletHelper(t, root)
	directory := expectAnchoredCheckFailure(t, root)
	assertReport(t, directory, "laws-summary.md", "proof of "+anchoredLaw+" changed", "owner")
	writeAnchoredLock(t, root)
}

func TestAnchoredReplayUsesBaseProof(t *testing.T) {
	root := anchoredFixture(t)
	base := commitAnchoredFixture(t, root)
	t.Run("removed guard", func(t *testing.T) {
		broken := "package wallet\nfunc Pay(balance, price int) error { return nil }\n"
		assertBrokenWalletRejected(t, root, base, broken)
	})
	t.Run("wrong condition", func(t *testing.T) {
		broken := strings.Replace(anchoredWalletSource, "price > balance", "price < 0", 1)
		assertBrokenWalletRejected(t, root, base, broken)
	})
	t.Run("honest change", func(t *testing.T) {
		honest := strings.Replace(anchoredWalletSource, "price > balance", "balance < price", 1)
		writeFixture(t, root, "wallet.go", honest)
		writeAnchoredLock(t, root)
		if err := run(anchoredCheckArgs(root, base), io.Discard); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("removed law", func(t *testing.T) {
		writeFixture(t, root, "LAWS.md", "# Wallet laws\n")
		writeAnchoredLock(t, root)
		directory := expectAnchoredCheckFailure(t, root, base)
		assertReport(t, directory, "laws-summary.md", anchoredLaw, "removed or renamed")
	})
}

func TestAnchoredReplayAcceptsRemovalApprovedAtBase(t *testing.T) {
	root := anchoredFixture(t)
	base := commitAnchoredFixture(t, root)
	original := readAnchoredLock(t, root)
	approval := strings.Replace(original, `"law": "never-overspend",`, `"law": "never-overspend", "allow_removal": true,`, 1)
	writeFixture(t, root, "LAWS.lock", approval)
	if regenerated := readAnchoredLock(t, root); !strings.Contains(regenerated, `"allow_removal": true`) {
		t.Fatal("lock regeneration discarded the owner's approval")
	}
	if err := run(anchoredCheckArgs(root, base), io.Discard); err != nil {
		t.Fatalf("owner's approval should pass with the law still in place: %v", err)
	}
	anchoredGit(t, root, "add", "LAWS.lock")
	anchoredGit(t, root, "commit", "-qm", "approve retirement of wallet law")
	approved := strings.TrimSpace(anchoredGit(t, root, "rev-parse", "HEAD"))
	writeFixture(t, root, "LAWS.md", "# Wallet laws\n")
	writeAnchoredLock(t, root)
	expectAnchoredCheckFailure(t, root, base)
	if err := run(anchoredCheckArgs(root, approved), io.Discard); err != nil {
		t.Fatalf("base-approved law removal was refused: %v", err)
	}
}

func assertBrokenWalletRejected(t *testing.T, root, base, broken string) {
	t.Helper()
	writeFixture(t, root, "wallet.go", broken)
	rewriteWalletHelper(t, root)
	writeAnchoredLock(t, root)
	if err := run(anchoredCheckArgs(root), io.Discard); err != nil {
		t.Fatalf("candidate proof should pass on its own: %v", err)
	}
	directory := expectAnchoredCheckFailure(t, root, base)
	assertReport(t, directory, "laws-summary.md", "the approved proof of "+anchoredLaw+" fails against this change")
}

func anchoredFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "lawgate.json", `{"modules":[{"path":".","language":"go","laws":"LAWS.md","machines":false}]}`)
	writeFixture(t, root, "go.mod", "module example.test/wallet\n\ngo 1.25\n")
	writeFixture(t, root, "LAWS.md", anchoredDoc)
	writeFixture(t, root, "wallet.go", anchoredWalletSource)
	writeFixture(t, root, "wallet_test.go", anchoredWalletProof)
	writeFixture(t, root, "helper_test.go", anchoredWalletHelper)
	writeAnchoredLock(t, root)
	return root
}

func writeAnchoredLock(t *testing.T, root string) {
	t.Helper()
	if err := run([]string{"lock", "--root", root}, io.Discard); err != nil {
		t.Fatal(err)
	}
}

func rewriteWalletHelper(t *testing.T, root string) {
	t.Helper()
	safe := strings.Replace(anchoredWalletHelper, "Pay(36, 900) == nil", "Pay(36, 1) != nil", 1)
	writeFixture(t, root, "helper_test.go", safe)
}

func anchoredCheckArgs(root string, base ...string) []string {
	args := []string{"check", "--root", root}
	if len(base) != 0 {
		args = append(args, "--base", base[0])
	}
	return args
}

func expectAnchoredCheckFailure(t *testing.T, root string, base ...string) string {
	t.Helper()
	var output bytes.Buffer
	if err := run(anchoredCheckArgs(root, base...), &output); err == nil {
		t.Fatal("check accepted an unapproved proof")
	}
	directory := strings.TrimSpace(output.String())
	logSummary(t, directory)
	return directory
}

func commitAnchoredFixture(t *testing.T, root string) string {
	t.Helper()
	anchoredGit(t, root, "init", "-q")
	anchoredGit(t, root, "add", ".")
	anchoredGit(t, root, "commit", "-qm", "approve wallet law")
	return strings.TrimSpace(anchoredGit(t, root, "rev-parse", "HEAD"))
}

func anchoredGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	flags := []string{"-c", "user.name=Wallet Owner", "-c", "user.email=owner@example.test", "-c", "commit.gpgsign=false"}
	command := exec.Command("git", append(flags, args...)...)
	command.Dir = root
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestLockPinsSubjectsAndIgnoresProofLayout(t *testing.T) {
	root := anchoredFixture(t)
	doc := strings.Replace(anchoredDoc, "not yet. This fixture isolates lock and replay from mutation.", "example.test/wallet.Pay, example.test/wallet.Account.Check", 1)
	writeFixture(t, root, "LAWS.md", doc)
	const method = "type Account struct{}\nfunc (Account) Check() int { return 1 }\n"
	writeFixture(t, root, "wallet.go", anchoredWalletSource+method)
	before := readAnchoredLock(t, root)
	writeFixture(t, root, "helper_test.go", "\n\n"+anchoredWalletHelper)
	writeFixture(t, root, "wallet.go", strings.Replace(anchoredWalletSource, "balance, price", "balance,   price", 1)+method)
	if updated := readAnchoredLock(t, root); updated != before {
		t.Fatalf("formatting changed proof lock:\n%s\n%s", before, updated)
	}
	writeFixture(t, root, "wallet.go", anchoredWalletSource+strings.Replace(method, "return 1", "return 2", 1))
	directory := expectAnchoredCheckFailure(t, root)
	assertReport(t, directory, "laws-summary.md", "subject of "+anchoredLaw+" changed")
	after := readAnchoredLock(t, root)
	if after == before || !strings.Contains(after, "example.test/wallet.Account.Check") {
		t.Fatalf("lock did not pin the method's source:\n%s", after)
	}
}

func readAnchoredLock(t *testing.T, root string) string {
	t.Helper()
	writeAnchoredLock(t, root)
	data, err := os.ReadFile(root + "/LAWS.lock")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
