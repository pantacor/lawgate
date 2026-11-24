package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pantacor/lawgate/machine"
)

// wantOutline is the Plain outline of the test worker: labels and catches
// with their limit, state marks (entry, pass entry, end, park), the doc and
// waker of a wait, sites with their line and labels, edges with their guard
// and limit, and the findings.
const wantOutline = `demo  worker.go
labels retrying
catch fault   ⇥ retry                  the job retries on the ladder (limit 3)
catch fail    ⇥ failed                 the job fails

start · entry  ■ running
  store-running                store
    ✎ write  1:10          store the running status
  on outcome
  └ stored → work

work
  when the job is claimed
  run                          compute
    ⚡ fault  2:20          the job retries
    ✗ fail   untagged:21   the job fails with the cause
  on outcome
  ├ ran → finish
  └ busy ↺ work when the lock is held (limit 2)

finish
  store-done                   store
    ✎ write  3:30          store the job as done [stores-done]
  on outcome
  └ stored ⇥ done

retry · pass entry · park  ■ waiting
  the job says when it retries
  woken by the backoff timer
  on outcome
  ├ next claim ↺ work
  └ ladder exhausted ⇥ failed

done · end

failed · end

findings
  high worker.go:20 the retry hides the cause
`

func TestTextWritesThePlainOutline(t *testing.T) {
	var out bytes.Buffer
	Text(&out, worker(), Plain)
	if out.String() != wantOutline {
		t.Fatalf("outline:\n%s\nwant:\n%s", out.String(), wantOutline)
	}
}

func TestTextColoursOnlyForANSI(t *testing.T) {
	var plain, coloured bytes.Buffer
	Text(&plain, worker(), Plain)
	Text(&coloured, worker(), ANSI)
	if strings.Contains(plain.String(), "\x1b[") {
		t.Fatal("the plain outline holds an escape code")
	}
	if !strings.Contains(coloured.String(), ansiGreen+"done"+ansiReset) {
		t.Fatal("the ANSI outline paints the done state green")
	}
}

// wantWorkBrief is the brief of state work: its edges in (a loop back to
// itself included), its sites by tag or by match, its edges and catches out,
// the rules naming the machine's test, and the finding on its line.
// The brief quotes code in backticks, which a raw string cannot hold: the
// golden text writes them as single quotes.
var wantWorkBrief = strings.ReplaceAll(`# Change state "work" of demo

Implemented by 'Work' in 'worker.go'.
Entered when: the job is claimed.

## Entered from
- start --stored--> work
- work --busy--> work
- retry --next claim--> work

## Steps and the lines they own
- **run** (compute) runs the job
  - line 20 '2' fault: the job retries
  - line 21 'failJob(' fail: the job fails with the cause

## Contract: these ways out must still exist after your change
- on ran → finish
- on busy ↺ work when the lock is held (limit 2)
- a fault site is caught ⇥ retry (limit 3)
- a fail site is caught ⇥ failed

## Rules
- Touch only the lines listed above unless an edge has to move.
- Every failure must reach the user: a failure with a message, never a silent log.
- If you add, remove or rename a log tag, update the machine declaration; 'go run ./cmd/machine -laws' must pass.

## Known findings in this state
- the retry hides the cause
`, "'", "`")

func TestBriefOfAWorkingState(t *testing.T) {
	var out bytes.Buffer
	if err := Brief(&out, worker(), "work"); err != nil {
		t.Fatal(err)
	}
	if out.String() != wantWorkBrief {
		t.Fatalf("brief:\n%s\nwant:\n%s", out.String(), wantWorkBrief)
	}
}

// A wait is entered through a catch and, as a pass entry, by a later pass.
func TestBriefNamesCatchesAndPassEntriesIn(t *testing.T) {
	var out bytes.Buffer
	if err := Brief(&out, worker(), "retry"); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"- the start of a later pass (a pass entry)\n", "- work --catch fault--> retry\n", "The user sees the status `waiting`"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the retry brief lacks %q:\n%s", want, out.String())
		}
	}
}

func TestBriefRefusesAnUndeclaredState(t *testing.T) {
	var out bytes.Buffer
	err := Brief(&out, worker(), "ghost")
	if err == nil || !strings.Contains(err.Error(), `machine demo has no state "ghost"`) {
		t.Fatalf("err = %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("nothing is written for an undeclared state, got:\n%s", out.String())
	}
}

func TestSummaryCountsStatesAndSites(t *testing.T) {
	want := "6 states, 1 fail, 1 fault, 0 silent, 2 write, 0 log, 0 guard"
	if got := Summary(worker()); got != want {
		t.Fatalf("summary = %q, want %q", got, want)
	}
	m := worker()
	m.States[1].Steps[0].Sites[0].Kind = "warn"
	if got := Summary(m); !strings.Contains(got, "1 warn") {
		t.Fatalf("a machine with a warn site counts it: %q", got)
	}
}

// A catch that carries on is drawn as a loop with its note, and the brief
// of a state holding its kind says the pass carries on there.
func TestCatchThatCarriesOnIsShownAsCarryingOn(t *testing.T) {
	m := worker()
	warned := site("4", machine.SiteWarn, "partly done", 22)
	m.States[1].Steps[0].Sites = append(m.States[1].Steps[0].Sites, warned)
	m.Catches = append(m.Catches, machine.Catch{On: machine.SiteWarn, Note: "the job log"})
	var outline, brief bytes.Buffer
	Text(&outline, m, Plain)
	if !strings.Contains(outline.String(), "catch warn    ↺ carries on             the job log\n") {
		t.Fatalf("outline:\n%s", outline.String())
	}
	if err := Brief(&brief, m, "work"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(brief.String(), "- a warn site carries on: the job log\n") {
		t.Fatalf("brief:\n%s", brief.String())
	}
}
