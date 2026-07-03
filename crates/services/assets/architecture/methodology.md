<!--
Adapted from the "architecture-copilot" skill in study8677/architecture-copilot
Source: https://github.com/study8677/architecture-copilot
License: MIT. See the SoloDawn LICENSE file for full attribution.
Changes: condensed and adapted for automated prompt injection (2026-07-04).
-->

# Architecture Thinking Checklist

Apply this checklist to the requirement text before decomposing work. Answer every item yourself; where requirements are silent, state an explicit assumption and proceed. Three beliefs: architecture is forced out of constraints, not drawn; no silver bullets, only tradeoffs — a plan with no stated downside is not thought through; no best architecture, only the most fitting under this constraint set.

0. Positioning. One sentence: what this is, for whom, and which existing product it most resembles.

1. Scope reduction. Write an explicit MVP do/don't list. Every item cut makes the architecture an order of magnitude simpler. Separate functional requirements (what) from quality attributes (how well); the latter plus constraints usually decide the architecture.

2. Six soul questions — answer each from requirements or record an assumption:
   - Scale: users/data now and at peak? (when to prepare for scale)
   - Read/write ratio? (which side to optimize for)
   - Consistency: must a write be immediately readable, or is brief staleness tolerable?
   - Growth: size in a year — gradual or explosive? (how much headroom)
   - Cost of failure: outage, lost data, wrong charge — how bad? (reliability and audit spend)
   - Constraints: team, time, budget, compliance, existing systems? (constraints kill options)

3. Back-of-envelope estimates (orders of magnitude): write QPS ~ daily writes / 100k; read QPS = ratio x write QPS; peak ~ 3x average; storage/year ~ item size x daily volume x 365. For AI/LLM features also estimate tokens per request, calls per task, first-token latency, per-call and daily cost, agent loop and fan-out caps. Then name the component that dies first (reads, writes, storage, bandwidth, GPU, queue, human review, cost) and concentrate design effort there.

4. Quality attribute ranking. Rank performance, availability, durability, scalability, consistency, security, cost, maintainability, observability, evolvability; for each top attribute, name what is knowingly sacrificed. "All equally important" means the analysis is unfinished.

5. Key decision forks. For each major choice record: "chose X over Y because Z; cost is W." Typical forks: storage by access shape (relational/KV/vector/object/inverted index/log); sync vs async on the critical path; cache or not, and how stale; state on client or server; monolith vs split — default to a modular monolith; microservices solve people-scaling first.

6. Convergence outputs, in order: data model first (data | access shape | store | consistency | why); ASCII container diagram (context black box plus external dependencies, then 5-6 containers with directional, meaningful arrows); ADR one-liners (decision/reason/cost/trigger signal); the first bottleneck at 100x load and its fix; an MVP-first evolution route (MVP, growth, maturity — never force the mature design onto the MVP).

7. Self-challenge — attack the plan: consistency and idempotency (double writes, retries, duplicate charges, reconciliation); resilience (dependency down, timeouts, backoff, circuit breaking, degradation); hot spots (fan-out, P99 tails, queue buildup, cache stampede, single shard/GPU); security and multi-tenancy (authz, tenant isolation, secrets, audit boundaries); AI-specific risks (hallucination, prompt injection, cost drift, missing evals); which single assumption, if wrong, hurts most.

Default to simple; upgrade only on signals: P95/P99 sustained over target, error budget burning, write QPS or single-store limits approached, releases blocking each other, per-task AI cost drifting, eval regressions. No signal: stay with the modular monolith. On signal: write an ADR with reason, alternative, and rollback path.

Fold the resulting decisions into the task instructions for implementing agents: one unified tech stack, shared data models defined once and referenced everywhere, and explicit module boundaries with named interfaces — so parallel work composes instead of colliding.
