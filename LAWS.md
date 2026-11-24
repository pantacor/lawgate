# lawgate laws

The rules lawgate keeps as the framework every adopting repo trusts. Humans
own this file: only the owner changes a law, and the change goes through the
owner's review (CODEOWNERS). `go run ./cmd/lawgate check` checks every law
here against the code: each enforcer must exist and name its law.

## The gate

### laws-have-a-source

No law exists without a source: every law in a `LAWS.md` cites a doc section
or a dated owner decision, names what enforces it, and has a status; an
enforcer that does not exist, or a test that does not name its law, is a
problem.

- Source: Owner decision 2026-11-19 (the proof-gated development plan, *Laws of the framework
  itself*).
- Enforced by: test `TestCheckReportsEachProblem`
- Subject: github.com/pantacor/lawgate/lawsdoc.checkSource
- Status: agreed 2026-11-19

### rule-changes-are-reviewed

No law, guard, kernel bound, proof property or gate file changes without the
owner's review: every rule file is covered by the required owner in
CODEOWNERS, which the platform enforces; a commit trailer is only an audit
note.

- Source: Owner decision 2026-11-19 (the proof-gated development plan); owner decision 2026-11-20:
  approval is platform review (CODEOWNERS naming the owner, enforced by
  branch protection), not a typed trailer.
- Enforced by: test `TestCheckFailsUncoveredRuleFile`, test `TestAuditRejectsRetroactiveNoteWithoutItsOwnApprovalTrailer`
- Subject: github.com/pantacor/lawgate/rulechanges.checkCoverage, github.com/pantacor/lawgate/rulechanges.history.auditCommit
- Status: agreed 2026-11-20

### laws-bite

No law is kept on trust: every law has a test that removes its behaviour and
shows the law breaking.

- Source: Owner decision 2026-11-19 (the proof-gated development plan, *Laws of the framework
  itself*).
- Enforced by: test `TestCheckLawsRequiresABreakingWitness`, test `TestBiteRunsAgreedEnforcers`, test `TestMutationReportsARunWithoutTheTestsResult`
  (every machine law carries a witness: a mutation of an isolated copy of
  the machine on which the law must report a violation, or `CheckLaws`
  refuses it; every agreed law enforced by a Go test names its subject, the
  function it protects, and the gate mutates each guard condition of that
  subject (negated, comparison flipped, guarded return disabled) through
  `go test -overlay`: the law's tests must fail on every mutant; each law's
  proof and subject are pinned in `LAWS.lock`, and CI replays the base
  commit's proofs against the change; a Node test or a script is rerun with
  `LAWGATE_BITE=<law>` and must fail after a control rerun naming an
  unrelated law passes, and a script must print `lawgate: rejects <law>`; a
  `Subject: not yet.` or `Bites: not yet.` gap stays pending with its
  reason).
- Subject: github.com/pantacor/lawgate/machine.checkLaw, github.com/pantacor/lawgate/cmd/lawgate.bitePhase.judge, github.com/pantacor/lawgate/cmd/lawgate.bitePhase.checkGoLaw, github.com/pantacor/lawgate/cmd/lawgate.bitePhase.checkLaw, github.com/pantacor/lawgate/mutate.Target.Mutants
- Status: agreed 2026-11-19

## Machines at run time

### no-silent-drift

No undeclared transition passes unnoticed: a move the declared machine does
not allow, from a state the pass is not in, or a trace that is not a path of
the machine, is refused with an error the caller must surface.

- Source: Owner decision 2026-11-19 (the proof-gated development plan, *Laws of the framework
  itself*).
- Enforced by: test `TestGoChecksContinuity`, test `TestIsPathRefusesBrokenTraces`, test `TestGoRefusesAnEmptyEdgeName`
- Subject: github.com/pantacor/lawgate/run.cursor.checkContinuity, github.com/pantacor/lawgate/run.checkMoveShape
- Status: agreed 2026-11-19

### generated-is-fresh

No generated state type differs from the declaration it came from.

- Source: Owner decision 2026-11-19 (the proof-gated development plan, *Laws of the framework
  itself*).
- Enforced by: test `TestCheckReportsStaleGeneratedFiles`, test `TestGenerationRejectsMissingTargetEvidence`, test `TestGenerationRejectsUncoveredGeneratedSource`
  (`gen.ReportCheck` regenerates in memory and reports each target fresh,
  stale or missing; `lawgate check` runs each module's `generate_check`,
  records those verdicts, and fails a generated file no target covers).
- Subject: github.com/pantacor/lawgate/gen.targetStatus, github.com/pantacor/lawgate/cmd/lawgate.generationCheck.run
- Status: agreed 2026-11-19

## Code

### readable-laws-code

No laws code is hard to read: inside a function body no statement more than
2 levels deep (a loop's single leading guard does not count); no struct or
map literal built inline inside another; no function longer than 40 lines;
no condition of 3 or more `&&`/`||` terms without a name; no `bool`
parameter. lawgate itself has zero findings and an empty baseline.

- Source: Owner decision 2026-11-20 ("I want to be able to read the laws
  code easily: no deep indentation, no objects inside objects inline").
- Enforced by: test `TestCoreFollowsTheReadabilityRule`
- Subject: github.com/pantacor/lawgate/readability.CheckFile
- Status: agreed 2026-11-20
