# ADR-002: Consolidate All Kratos Webhook Handling into a Single Service

**Feature**: 003-webhook-consolidation
**Date**: 2026-02-27
**Status**: Proposed

## Context

Ory Kratos webhooks are currently split across two services:

| Service | Webhook Responsibility | Infrastructure |
|---------|----------------------|----------------|
| **webhook-gateway** | Verification event (→ RabbitMQ), login backoff (Redis counters + reverse proxy) | Redis, RabbitMQ |
| **oidc-service** | Post-login/registration identity resolution (→ patches Kratos identity metadata via Admin API) | Postgres (reads), Kratos Admin API, Alkemio API |

The oidc-service's primary responsibility is Hydra OAuth2 challenge handling (login/consent providers). Its Kratos webhook is a secondary concern that was added there because it already had the identity resolution wiring (Postgres access, Kratos Admin API client). This is a misplaced concern — Kratos webhook handling is not related to OAuth2 challenge flows.

This fragmentation causes concrete operational problems:

1. **Two Kratos webhook targets to configure** — the Kratos Helm values must declare webhook URLs for both services, each with separate service discovery, health checks, and failure modes.
2. **Two services to monitor for webhook health** — webhook failures in oidc-service are mixed with OAuth2 flow logs, making triage harder.
3. **Two services to secure** — webhook authentication (shared secrets, mTLS) must be configured independently for each target.
4. **Scattered domain knowledge** — understanding "what happens when a user logs in via Kratos" requires reading code in two repositories with different conventions.
5. **Coupled release cycles** — changes to Kratos webhook behavior (e.g., new hook points, schema changes) may require coordinated deploys across both services.

## Decision

Consolidate all Kratos webhook handling into this repository (webhook-gateway), renaming it to **kratos-hooks** to reflect its expanded scope. The consolidation will be phased:

**Phase 1 — Absorb identity resolution webhook:**
- Implement the post-login/registration identity resolution handler currently in oidc-service
- This handler receives the Kratos after-login/after-registration webhook, resolves the user's Alkemio identity (via Alkemio API or direct Postgres query), and patches the Kratos identity metadata with the resolved identity fields via the Kratos Admin API
- Add the necessary clients: Kratos Admin API client, Alkemio API client (or Postgres read access)

**Phase 2 — Remove webhook code from oidc-service:**
- Delete the Kratos webhook handler and related wiring from oidc-service
- Update Kratos Helm values to point all webhooks at the consolidated service
- oidc-service retains only its primary responsibility: Hydra OAuth2 challenge handling

**Phase 3 — Rename and re-brand:**
- Rename the repository and Docker image from `webhook-gateway` to `kratos-hooks`
- Update all deployment references (Helm charts, CI pipelines, monitoring dashboards)

## Alternatives Considered

| Alternative | Pros | Cons | Why Rejected |
|-------------|------|------|--------------|
| **A. Keep as-is (two services)** | No migration effort. Each service stays small. | Fragmented ops, dual webhook config in Kratos values, scattered domain knowledge, two services to monitor/secure for the same concern. | Ongoing operational cost outweighs migration effort |
| **B. Move everything into oidc-service** | Single service for all Kratos hooks. oidc-service already has identity resolution wiring. | Conflates two distinct domains (OAuth2 challenges vs. Kratos webhooks). oidc-service becomes a grab-bag. Webhook-gateway's reverse proxy and Redis logic don't belong in an OAuth2 service. | Wrong direction — deepens the misplaced-concern problem |
| **C. Move into Alkemio server** | Eliminates a standalone service entirely. | Alkemio server is a large monolith with its own release cadence. Webhook handling is latency-sensitive (Kratos blocks on webhook responses). Coupling to the monolith's deploy cycle adds risk. | Inappropriate coupling to a much larger system |
| **D. Kratos sidecar (per-pod webhook handler)** | Co-located with Kratos, minimal network hop. | Requires running webhook logic in every Kratos pod. Harder to manage shared state (Redis, RabbitMQ). Complicates horizontal scaling — sidecar resource usage scales with Kratos pods, not webhook load. | Over-engineered; network hop cost is negligible in-cluster |

## Consequences

**Positive:**
- Single Kratos webhook target — one service URL in Kratos Helm values for all hooks
- Unified monitoring — all webhook logs, metrics, and traces in one service
- Single point for webhook authentication configuration
- Cohesive domain — "what happens on Kratos events" is answered by reading one codebase
- oidc-service becomes a clean, focused OAuth2 challenge handler
- Shared infrastructure (Redis, RabbitMQ) is already wired in webhook-gateway

**Negative:**
- Larger dependency footprint — the consolidated service gains Postgres (read), Kratos Admin API, and Alkemio API dependencies that it doesn't currently have
- Increased blast radius — a bug in the consolidated service could affect all Kratos webhooks, not just a subset
- Migration effort — phased rollout requires coordinated changes across webhook-gateway, oidc-service, and Kratos Helm values
- Repository rename (`webhook-gateway` → `kratos-hooks`) requires updating CI pipelines, Docker registries, Helm charts, and documentation references

**Risk mitigation:**
- Phased approach allows rollback at each phase — Phase 1 can run in parallel with the oidc-service handler (both active) before Phase 2 removes the old code
- Health checks and circuit breakers on new external clients (Kratos Admin API, Alkemio API) prevent cascading failures
- The rename (Phase 3) is deferred until after functional consolidation is proven stable

## Empirical Validation

Pending — this is a proposed architectural change. Validation criteria for each phase:

1. **Phase 1**: Identity resolution webhook handler passes the same integration tests as the oidc-service handler. Kratos identity metadata is correctly patched after login/registration.
2. **Phase 2**: Kratos Helm values point exclusively at the consolidated service. oidc-service webhook code is deleted. No regressions in login/registration flows.
3. **Phase 3**: All deployment references updated. CI/CD pipelines build and deploy `kratos-hooks`. Monitoring dashboards reflect the new service name.

## References

- [ADR-001: Reverse Proxy for Login Interception](../../002-login-backoff/decisions/ADR-001-reverse-proxy-for-login-interception.md) — existing webhook-gateway architectural decision
- [webhook-gateway handlers](../../../internal/webhooks/) — current webhook implementations
- [webhook-gateway Redis client](../../../internal/clients/redis.go) — existing shared infrastructure
- [Ory Kratos Webhooks Documentation](https://www.ory.sh/docs/kratos/hooks/configure-hooks) — hook configuration reference
