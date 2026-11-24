package lawsdoc

import (
	"slices"
	"testing"

	"github.com/pantacor/lawgate/mutate"
)

func TestSubjectNamesTheFunctionsALawProtects(t *testing.T) {
	doc := section("first-law", "test `TestFirst`", agreed)
	doc += "- Subject: `example.org/shop.Pay`, `example.org/shop.Wallet.Refund`\n"
	plan := Plan(doc)[0]
	pay := mutate.Subject{ImportPath: "example.org/shop", Func: "Pay"}
	refund := mutate.Subject{ImportPath: "example.org/shop", Receiver: "Wallet", Func: "Refund"}
	if !slices.Equal(plan.Subjects, []mutate.Subject{pay, refund}) || plan.SubjectGap != "" {
		t.Fatalf("subjects = %+v, gap %q", plan.Subjects, plan.SubjectGap)
	}
	if problems := plan.problems; len(problems) != 0 {
		t.Fatalf("problems: %q", problems)
	}
}

func TestSubjectGapKeepsItsReason(t *testing.T) {
	doc := section("first-law", "test `TestFirst`", agreed) + "- Subject: not yet. The rule lives in a shell script.\n"
	plan := Plan(doc)[0]
	if plan.SubjectGap != "The rule lives in a shell script." || len(plan.Subjects) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
}

func TestSubjectMustBeAFunctionOrAGapWithAReason(t *testing.T) {
	doc := section("first-law", "test `TestFirst`", agreed)
	requireProblem(t, doc+"- Subject: not yet.\n", Inputs{}, "must say why")
	requireProblem(t, doc+"- Subject: the wallet\n", Inputs{}, "is not <import path>.<Func>")
	requireProblem(t, doc+"- Subject:\n", Inputs{}, "names no function")
}

func TestSubjectRequiresATestEnforcer(t *testing.T) {
	doc := section("first-law", "machine `worker`", agreed) + "- Subject: example.org/shop.Pay\n"
	requireProblem(t, doc, machineInputs(), "no test enforcer")
}

func TestSubjectLinesAddUp(t *testing.T) {
	doc := section("first-law", "test `TestFirst`", agreed)
	doc += "- Subject: `example.org/shop.Pay`\n- Subject: `example.org/shop.Refund`\n"
	plan := Plan(doc)[0]
	both := len(plan.Subjects) == 2 && plan.Subjects[1].Func == "Refund"
	if !both || len(plan.problems) != 0 {
		t.Fatalf("subjects = %+v, problems %q", plan.Subjects, plan.problems)
	}
	gap := section("first-law", "test `TestFirst`", agreed) + "- Subject: example.org/shop.Pay\n- Subject: not yet. Later.\n"
	requireProblem(t, gap, Inputs{}, "\"Subject:\" must be")
}
