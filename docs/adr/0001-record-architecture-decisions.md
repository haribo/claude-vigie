# ADR-0001: Record architecture decisions

## Status

Accepted

## Context

The project makes structural decisions (stack, storage, protocols, security
model) that are not obvious from the code alone. Without a written record, the
rationale is lost and past debates are re-litigated.

## Decision

We record significant architecture decisions as ADRs in `docs/adr/`, numbered
sequentially (`NNNN-title-in-kebab-case.md`), following the sections:
**Status / Context / Decision / Rationale / Consequences / References**.

An ADR is immutable once accepted. To change a decision, add a new ADR that
supersedes it and update the old one's status.

## Rationale

- Keeps the "why" close to the "what", versioned with the code
- Cheap to write, cheap to read, easy to review in a PR
- The lightweight Nygard format is widely understood

## Consequences

### Positive

- New contributors can understand past decisions without archaeology
- Decisions are debated once and referenced thereafter

### Negative

- A small discipline cost: significant decisions must be written down

## References

- Michael Nygard, "Documenting Architecture Decisions"
