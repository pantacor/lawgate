# example worker laws

Laws for the example adopting module (`worker` machine and imperative checks).

## no-silent-errors

No failure site in the `worker` machine may swallow an error silently.

- Source: `../README.md`, *Example adopting module* (agreed 2026-11-20).
- Enforced by: machine `worker`
- Status: agreed 2026-11-20

## generated-is-fresh

No generated worker state type differs from its machine declaration.

- Source: `../LAWS.md`, *generated-is-fresh* (agreed 2026-11-19).
- Enforced by: test `TestWorkerStatesAreFresh`
- Status: agreed 2026-11-19

## validate-before-store

Every path through the `worker` machine validates the job before storing the
completed result.

- Source: `../README.md`, *Example adopting module* (agreed 2026-11-20).
- Enforced by: machine `worker`
- Status: agreed 2026-11-20

## rejects-empty-payload

Processing a job with an empty payload fails before writing a completed result.

- Source: `../README.md`, *Example adopting module* (agreed 2026-11-20).
- Enforced by: test `TestRejectsEmptyPayload`
- Status: agreed 2026-11-20
