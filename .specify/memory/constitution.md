<!--
SYNC IMPACT REPORT
==================
Version change: v1.1.0 → v1.2.0
Changes:
  - Added principle #17: "Architecture Decision Records (ADRs)" requiring formal ADRs for significant technical decisions
  - Added ADR template (.specify/templates/adr-template.md)
  - Added master decision log (docs/adr.md) as single entry point for all ADRs
  - Updated Architecture Standards with ADR directory convention and master index
  - Updated Engineering Workflow with ADR creation step
  - MINOR bump: new principle added

Previous version change: v1.0.0 → v1.1.0
  - Added principle #16: "Webhook Segregation" for clear code organization per webhook
  - Updated Architecture Standards with webhook-specific folder structure pattern

Templates requiring updates:
  - .specify/templates/adr-template.md: ✅ Created (new template)
  - .specify/templates/plan-template.md: ✅ Updated (added ADR to documentation structure)
  - .specify/templates/spec-template.md: ✅ No changes needed
  - .specify/templates/tasks-template.md: ✅ No changes needed

Follow-up TODOs: None
-->

# Alkemio Kratos Webhooks Service Engineering Constitution

**Version**: 1.2.0
**Ratification Date**: 2025-11-01
**Last Amended**: 2026-02-26

## Core Principles

1. **Deterministic Configuration**
   - All runtime settings flow through typed structs in `internal/config`; no package reads from `os.Getenv` outside bootstrap.

2. **Operational Observability**
   - Zap logging includes correlation IDs, challenge IDs, and outcome status. Prometheus metrics were intentionally removed in Nov 2025; structured logs (shipped via the platform stack) are the single required observability signal.

3. **Fail-Fast Maintenance Controls**
   - Maintenance mode MUST immediately return HTTP 503 with Retry-After headers without touching external services.

4. **Test-Driven Delivery**
   - Tests exist only when they defend a real invariant or observable behaviour. Contract tests MUST exercise OpenAPI behaviour before handler implementation.

5. **Secure Build Pipeline**
   - Docker builds MUST pin base image digests, run `go test` and `go build` in multi-stage pipeline, and produce distroless runtime images.

6. **CI Integrity**
   - GitHub Actions pipelines MUST run linting, unit tests, integration smoke, SBOM generation, and signed Docker pushes before releases.

7. **Evidence-Based Performance Goals**
   - Do not invent ad-hoc SLAs (e.g., "p95 < 500 ms") for thin orchestration layers whose latency is dominated by external systems. Only codify performance targets when the service owns the bottleneck and can meaningfully improve it; otherwise focus on timeout budgets, dependency health checks, and log-based diagnostics.

8. **Meaningful Success Criteria**
   - Specifications MUST express success criteria that are directly testable within webhooks (e.g., contract or integration tests) and under this feature's control; avoid vanity metrics or external business outcomes that cannot be validated during development.
   - Never invent arbitrary measurable outcomes pulled from thin air (e.g., "50ms response time", "40% improvement") unless backed by baseline measurements or explicit stakeholder requirements.

9. **No Busywork**
   - Every task, test, and artifact MUST deliver demonstrable value. Reject make-work activities that exist only to satisfy process checkboxes.
   - Do not create documentation, comments, or abstractions "just in case" — only when they solve a real problem or prevent a real mistake.
   - Specifications SHOULD be lean: include only what is necessary to communicate intent and validate correctness.

10. **Meaningful Tests Only**
    - Tests MUST defend real invariants or catch real regressions. Never write tests for the sake of coverage metrics.
    - Avoid testing implementation details, trivial getters/setters, or scenarios that cannot fail in practice.
    - If a test does not help catch bugs or document critical behaviour, delete it.

11. **Latest Dependencies Always**
    - When adding or updating any dependency, always verify the latest stable version by checking online sources (pkg.go.dev, GitHub releases, npm registry, etc.). Never rely on AI training data for version numbers.
    - Pin dependencies to specific versions in go.mod/go.sum, but ensure those versions are current at time of addition.
    - Document rationale when intentionally using older versions (compatibility, stability concerns).

12. **Always Root Cause Analysis**
    - Never apply opportunistic or speculative fixes hoping they might resolve an issue.
    - Before any bug fix, identify and document the actual root cause with evidence (logs, traces, reproduction steps).
    - If the root cause is unclear, invest time in debugging and analysis first — guessing wastes more time than investigating.
    - Fixes MUST directly address the identified root cause, not symptoms.

13. **No Legacy Code**
    - Never silently assume backward compatibility is required. We control the full stack and all consumers.
    - Do not leave code "just in case" — dead, deprecated, or unused code has no right to stay in the codebase unless explicitly requested.
    - When a feature requires changes across multiple services, coordinate those changes rather than maintaining compatibility shims.
    - Aim for a minimalistic, well-maintainable codebase. Every line of code MUST justify its existence.
    - Remove backward-compatibility hacks, unused exports, commented-out code, and defensive code for scenarios that no longer apply.

14. **Single Source of Truth**
    - No two methods SHOULD implement the same logic in different modules. If duplication exists, extract to a single shared utility.
    - When methods share partial logic, extract the common part to a shared helper.
    - There is always exactly one piece of code for any given logic — find it, use it, or create it once.
    - Before implementing new logic, search for existing implementations. Extend rather than duplicate.
    - Configuration, constants, and type definitions MUST live in one canonical location.

15. **No Assumptions**
    - Never assume requirements, behavior, or implementation details that are not explicitly defined.
    - If something is unclear or unknown, ask the user for clarification before proceeding.
    - If factual information is needed (versions, API specs, library behavior), search online to verify.
    - Do not guess — guessing leads to rework. Asking or searching takes less time than fixing wrong assumptions.

16. **Webhook Segregation**
    - Each webhook integration MUST have its own dedicated folder under `internal/webhooks/<webhook-name>/`.
    - Webhook folders MUST be self-contained: handler, models, validation, and webhook-specific logic live together.
    - Shared utilities (HTTP clients, logging, common types) MUST remain in separate shared packages; webhook folders MUST NOT duplicate shared logic.
    - A developer navigating to a webhook folder MUST be able to understand that webhook's complete behaviour without reading unrelated webhook code.
    - Adding a new webhook MUST NOT require modifying existing webhook folders beyond shared registration/routing.

17. **Architecture Decision Records (ADRs)**
    - Significant architectural decisions MUST be captured as numbered ADR documents in `specs/<feature>/decisions/`.
    - Every ADR MUST be indexed in the master decision log at `docs/adr.md`. This is the single entry point for discovering all architectural decisions across the service.
    - An ADR is required when a decision: (a) introduces or changes infrastructure patterns, (b) selects one approach over viable alternatives with meaningful trade-offs, (c) constrains future design choices, or (d) is non-obvious and would otherwise rely on tribal knowledge.
    - Each ADR MUST include: context (why the decision was needed), decision (what was chosen), alternatives considered (what was rejected and why), consequences (positive and negative), and empirical validation (evidence that the decision is correct, not just theoretical reasoning).
    - ADRs are immutable once accepted. If a decision is reversed, a new ADR supersedes the original (linking back to it) rather than editing it.
    - Research findings (research.md) document investigation outcomes. ADRs document architectural commitments. A research finding MAY lead to an ADR when it results in a binding design choice.

## Architecture Standards

- Directory layout:
  - `cmd/server`: entrypoint wiring configuration and HTTP server start.
  - `internal/*`: non-exported packages encapsulating config, clients, middleware, and domain orchestration.
  - `internal/webhooks/<webhook-name>/`: dedicated folder per webhook integration containing:
    - `handler.go`: HTTP handler for the webhook endpoint.
    - `models.go`: request/response types specific to this webhook.
    - `validation.go`: input validation rules (if needed).
    - Additional files as the webhook's complexity requires.
  - `pkg/telemetry`: shared logging utilities safe for reuse.
  - `configs/`: sample environment files and documentation.
  - `contracts/`: OpenAPI definitions and generated fixtures.
  - `specs/<feature>/decisions/`: ADR documents for significant architectural choices within a feature.
  - `docs/adr.md`: master index of all ADRs across features — single entry point for architectural decisions.
  - `docs/`: operational runbooks and quickstarts.
- Health endpoints (`/health/live`, `/health/ready`) MUST satisfy constitution observability principle with structured JSON responses.
- Maintenance middleware MUST short-circuit request handling while still recording structured logs.
- Performance requirements appear only when the service directly controls the critical path; otherwise specifications MUST document dependency expectations instead of arbitrary p95 quotas.

## Engineering Workflow

- Execute Spec Kit phases sequentially: research → design → tasks → implementation → analysis → validation. ADRs are created during or after implementation when significant architectural decisions are made or empirically validated.
- Tests (`go test ./...`) MUST pass before merging. Contract tests run under `test/contract`; integration tests use `httptest` with mocked clients.
- Any change to OpenAPI contracts requires regenerating associated fixtures and reviewing downstream consumers.
- Release bumps require update to `docs/operations.md` and changelog entry summarizing risk.
- Feature numbering is global. Every new feature branch, Spec Kit directory, and checklist MUST use the next available integer across the repository (e.g., `004-agent-claim`), never resetting numbering per initiative or contributor.

## Governance

- **Constitution Version**: 1.2.0
- **Versioning Policy**: Semantic versioning (MAJOR.MINOR.PATCH)
  - MAJOR: Backward incompatible governance/principle removals or redefinitions.
  - MINOR: New principle/section added or materially expanded guidance.
  - PATCH: Clarifications, wording, typo fixes, non-semantic refinements.
- **Amendment Procedure**: Constitution changes require explicit review and documentation of rationale. All amendments update the "Last Amended" date.
- **Compliance Review**: Deviations from this constitution are documented in Spec Kit plans with mitigation timelines.
- Major changes (new external dependency, architecture shift) require constitution update and review.
- Operability incidents result in follow-up tasks captured in Spec Kit memory.
