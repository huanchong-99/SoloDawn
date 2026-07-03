//! Requirement ledger (评分点账本) sync and prompt assembly.
//!
//! The ledger is the project-scoped list of acceptance points (评分点). Each
//! point is one functional-completeness criterion from the confirmed rubric.
//! This module owns the three ledger operations that sit between the audit
//! plan and the database rows:
//!
//! 1. [`sync_ledger_from_audit_plan`] — at confirm time, extract the
//!    functional-completeness criteria, assign stable server-side point codes
//!    (`RP-001`, ...), rewrite the criteria text to carry the codes, and
//!    upsert ledger rows. LLM-regenerated wording never chooses ids.
//! 2. [`append_regression_section`] — for follow-up rounds, extend the rubric
//!    text with regression assertions for already-delivered points (verify
//!    not broken; no re-scoring).
//! 3. [`build_ledger_background`] — compressed project background for a
//!    follow-up round's planning conversation / initial goal: the point index
//!    plus delivered points' context capsules. Never raw history.

use db::models::requirement_item::{
    ContextCapsule, REQUIREMENT_STATUS_DELIVERED, REQUIREMENT_STATUS_REGRESSED, RequirementItem,
};
use sqlx::SqlitePool;
use uuid::Uuid;

use crate::services::orchestrator::AuditPlan;

/// Dimension whose criteria constitute the ledger points.
const FUNCTIONAL_DIMENSION: &str = "functional_completeness";

/// Strip an existing `[RP-xxx]` prefix from a criterion, returning
/// `(Some(code), rest)` when present.
fn split_point_code(criterion: &str) -> (Option<String>, String) {
    let trimmed = criterion.trim();
    if let Some(rest) = trimmed.strip_prefix('[') {
        if let Some(end) = rest.find(']') {
            let code = &rest[..end];
            if code.starts_with("RP-") && code[3..].chars().all(|c| c.is_ascii_digit()) {
                return (
                    Some(code.to_string()),
                    rest[end + 1..].trim().to_string(),
                );
            }
        }
    }
    (None, trimmed.to_string())
}

/// Sync the project ledger from a freshly generated audit plan and rewrite
/// the plan's functional criteria to carry stable point codes.
///
/// - Criteria matching an existing point's text (case-insensitive) reuse that
///   point's code instead of creating a duplicate row.
/// - New criteria get the next free `RP-xxx` code and a `pending` row.
/// - Existing delivered/regressed rows are never modified here.
///
/// Returns the number of newly inserted points.
pub async fn sync_ledger_from_audit_plan(
    pool: &SqlitePool,
    project_id: Uuid,
    origin_draft_id: &str,
    plan: &mut AuditPlan,
) -> anyhow::Result<usize> {
    let existing = RequirementItem::find_by_project(pool, project_id).await?;

    let mut inserted = 0usize;
    for dim in plan
        .dimensions
        .iter_mut()
        .filter(|d| d.name == FUNCTIONAL_DIMENSION)
    {
        for criterion in &mut dim.criteria {
            let (code_in_text, text) = split_point_code(criterion);
            if text.is_empty() {
                continue;
            }

            // Reuse: explicit code that exists, else exact text match.
            let matched = existing.iter().find(|item| {
                code_in_text.as_deref() == Some(item.point_code.as_str())
                    || item.text.trim().eq_ignore_ascii_case(text.as_str())
            });

            let code = if let Some(item) = matched {
                item.point_code.clone()
            } else {
                let code = RequirementItem::next_point_code(pool, project_id).await?;
                let item = RequirementItem::new(project_id, &code, &text, origin_draft_id);
                RequirementItem::insert(pool, &item).await?;
                inserted += 1;
                code
            };

            *criterion = format!("[{code}] {text}");
        }
    }

    Ok(inserted)
}

/// Append a regression-assertion section to the rubric text for a follow-up
/// round: delivered points must stay green, verified against the code — not
/// re-scored as new work.
pub fn append_regression_section(plan: &mut AuditPlan, delivered: &[RequirementItem]) {
    if delivered.is_empty() {
        return;
    }
    let mut section = String::from(
        "\n\n## Regression Assertions (previously delivered points)\n\
         The points below were delivered and accepted in earlier rounds. They are NOT part of \
         this round's new scope and must NOT be re-scored as new work. For each one, verify the \
         behaviour still exists and is not broken by this round's changes. If a point is broken, \
         report it in `requirement_verdicts` with status \"red\" and explain the regression in \
         `fix_instructions` — a regression is a blocking defect.\n",
    );
    for item in delivered {
        section.push_str(&format!("- [{}] {}\n", item.point_code, item.text));
    }
    plan.raw_principles.push_str(&section);
}

/// Compressed project background for a follow-up round: the ledger index (one
/// line per point) plus the context capsules of delivered points. This is the
/// ONLY project memory handed to the next round — capsules are the map, the
/// repository is the territory.
///
/// Returns `None` when the project has no ledger yet.
pub async fn build_ledger_background(
    pool: &SqlitePool,
    project_id: Uuid,
) -> anyhow::Result<Option<String>> {
    let items = RequirementItem::find_by_project(pool, project_id).await?;
    if items.is_empty() {
        return Ok(None);
    }

    let mut out = String::from(
        "## Project Background — Requirement Ledger (from previous rounds)\n\
         Each point below is an acceptance requirement of this project. Delivered points carry a \
         compressed context capsule: pointers to where the work lives and knowledge the code \
         cannot show. Use capsules to start from the existing implementation instead of \
         re-exploring the project from zero; the repository itself is always the source of truth \
         (verify against the code before building on a capsule).\n\n### Point Index\n",
    );
    for item in &items {
        out.push_str(&format!(
            "- [{}] ({}) {}\n",
            item.point_code, item.status, item.text
        ));
    }

    let delivered: Vec<&RequirementItem> = items
        .iter()
        .filter(|i| {
            i.status == REQUIREMENT_STATUS_DELIVERED || i.status == REQUIREMENT_STATUS_REGRESSED
        })
        .filter(|i| i.context_capsule.is_some())
        .collect();

    if !delivered.is_empty() {
        out.push_str("\n### Context Capsules (delivered points)\n");
        for item in delivered {
            let capsule = item
                .context_capsule
                .as_deref()
                .and_then(|j| serde_json::from_str::<ContextCapsule>(j).ok())
                .unwrap_or_default();
            out.push_str(&format!("\n[{}] {}\n", item.point_code, item.text));
            if !capsule.built.is_empty() {
                out.push_str(&format!("- Built: {}\n", capsule.built));
            }
            if !capsule.lives_where.is_empty() {
                out.push_str(&format!("- Lives in: {}\n", capsule.lives_where));
            }
            if !capsule.decisions.is_empty() {
                out.push_str(&format!("- Decisions & gotchas: {}\n", capsule.decisions));
            }
            if !capsule.extension_notes.is_empty() {
                out.push_str(&format!("- Extend from: {}\n", capsule.extension_notes));
            }
            if let Some(commits) = item.provenance_commits.as_deref() {
                if !commits.is_empty() {
                    out.push_str(&format!("- Delivered as: {commits}\n"));
                }
            }
        }
    }

    Ok(Some(out))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::services::orchestrator::{AuditDimensionSpec, AuditMode};

    async fn test_pool() -> SqlitePool {
        let pool = SqlitePool::connect(":memory:").await.unwrap();
        sqlx::query(
            r"
            CREATE TABLE requirement_item (
                id TEXT PRIMARY KEY,
                project_id BLOB NOT NULL,
                point_code TEXT NOT NULL,
                text TEXT NOT NULL,
                status TEXT NOT NULL DEFAULT 'pending',
                origin_draft_id TEXT,
                context_capsule TEXT,
                provenance_workflow_id TEXT,
                provenance_commits TEXT,
                created_at DATETIME NOT NULL DEFAULT (datetime('now')),
                updated_at DATETIME NOT NULL DEFAULT (datetime('now')),
                delivered_at DATETIME,
                UNIQUE(project_id, point_code)
            )
            ",
        )
        .execute(&pool)
        .await
        .unwrap();
        pool
    }

    fn plan_with_criteria(criteria: Vec<&str>) -> AuditPlan {
        AuditPlan {
            mode: AuditMode::Builtin,
            dimensions: vec![
                AuditDimensionSpec {
                    name: "buildability".into(),
                    name_zh: "可构建性".into(),
                    max_score: 20.0,
                    criteria: vec!["builds cleanly".into()],
                    sub_dimensions: None,
                },
                AuditDimensionSpec {
                    name: "functional_completeness".into(),
                    name_zh: "功能完整性".into(),
                    max_score: 25.0,
                    criteria: criteria.into_iter().map(String::from).collect(),
                    sub_dimensions: None,
                },
            ],
            pass_threshold: 90.0,
            generated_at: "2026-07-03T00:00:00Z".into(),
            raw_principles: "principles".into(),
        }
    }

    #[test]
    fn split_point_code_variants() {
        assert_eq!(split_point_code("plain text"), (None, "plain text".into()));
        assert_eq!(
            split_point_code("[RP-004] tagged"),
            (Some("RP-004".into()), "tagged".into())
        );
        // Non-RP bracket prefixes are content, not codes.
        assert_eq!(
            split_point_code("[P1] priority tag"),
            (None, "[P1] priority tag".into())
        );
    }

    #[tokio::test]
    async fn sync_assigns_codes_and_inserts_rows() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let mut plan = plan_with_criteria(vec!["create memos", "delete memos"]);

        let inserted = sync_ledger_from_audit_plan(&pool, project, "draft-1", &mut plan)
            .await
            .unwrap();
        assert_eq!(inserted, 2);

        let functional = &plan.dimensions[1].criteria;
        assert_eq!(functional[0], "[RP-001] create memos");
        assert_eq!(functional[1], "[RP-002] delete memos");
        // Non-functional dimensions untouched.
        assert_eq!(plan.dimensions[0].criteria[0], "builds cleanly");

        let items = RequirementItem::find_by_project(&pool, project).await.unwrap();
        assert_eq!(items.len(), 2);
        assert!(items.iter().all(|i| i.status == "pending"));
    }

    #[tokio::test]
    async fn sync_reuses_existing_points_by_text_and_code() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();

        let mut round1 = plan_with_criteria(vec!["create memos"]);
        sync_ledger_from_audit_plan(&pool, project, "draft-1", &mut round1)
            .await
            .unwrap();

        // Round 2 restates the old point (different case), carries one tagged
        // criterion, and adds a genuinely new one.
        let mut round2 =
            plan_with_criteria(vec!["Create Memos", "[RP-001] create memos", "timed reminders"]);
        let inserted = sync_ledger_from_audit_plan(&pool, project, "draft-2", &mut round2)
            .await
            .unwrap();
        assert_eq!(inserted, 1, "only the reminder point is new");

        let functional = &round2.dimensions[1].criteria;
        assert_eq!(functional[0], "[RP-001] Create Memos");
        assert_eq!(functional[1], "[RP-001] create memos");
        assert_eq!(functional[2], "[RP-002] timed reminders");

        let items = RequirementItem::find_by_project(&pool, project).await.unwrap();
        assert_eq!(items.len(), 2);
    }

    #[tokio::test]
    async fn regression_section_lists_delivered_points() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        let item = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &item).await.unwrap();
        RequirementItem::mark_delivered(&pool, &item.id, None, "wf-1", None)
            .await
            .unwrap();

        let delivered =
            RequirementItem::find_by_project_and_status(&pool, project, "delivered")
                .await
                .unwrap();
        let mut plan = plan_with_criteria(vec!["timed reminders"]);
        append_regression_section(&mut plan, &delivered);

        assert!(plan.raw_principles.contains("Regression Assertions"));
        assert!(plan.raw_principles.contains("[RP-001] memo CRUD"));

        // Empty delivered list leaves the rubric untouched.
        let mut plan2 = plan_with_criteria(vec!["x"]);
        append_regression_section(&mut plan2, &[]);
        assert_eq!(plan2.raw_principles, "principles");
    }

    #[tokio::test]
    async fn background_is_none_without_ledger_and_indexes_with() {
        let pool = test_pool().await;
        let project = Uuid::new_v4();
        assert!(build_ledger_background(&pool, project).await.unwrap().is_none());

        let a = RequirementItem::new(project, "RP-001", "memo CRUD", "draft-1");
        RequirementItem::insert(&pool, &a).await.unwrap();
        let capsule = serde_json::to_string(&ContextCapsule {
            built: "CRUD endpoints".into(),
            lives_where: "src/memo/".into(),
            decisions: String::new(),
            extension_notes: "extend routes".into(),
        })
        .unwrap();
        RequirementItem::mark_delivered(&pool, &a.id, Some(&capsule), "wf-1", Some("2 files"))
            .await
            .unwrap();
        let b = RequirementItem::new(project, "RP-002", "timed reminders", "draft-2");
        RequirementItem::insert(&pool, &b).await.unwrap();

        let bg = build_ledger_background(&pool, project).await.unwrap().unwrap();
        assert!(bg.contains("[RP-001] (delivered) memo CRUD"));
        assert!(bg.contains("[RP-002] (pending) timed reminders"));
        assert!(bg.contains("Lives in: src/memo/"));
        assert!(bg.contains("Delivered as: 2 files"));
        // Pending points have no capsule section.
        assert!(!bg.contains("[RP-002] timed reminders\n- Built"));
    }
}
