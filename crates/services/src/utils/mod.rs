/// Convert a string to a URL-safe slug
///
/// Rules:
/// - Convert to lowercase
/// - Keep only ASCII alphanumeric characters
/// - Replace all other characters (spaces, special chars, CJK) with hyphens
/// - Remove consecutive hyphens
/// - Trim hyphens from start and end
pub fn slugify(input: &str) -> String {
    input
        .to_lowercase()
        .chars()
        .fold(String::new(), |mut acc, c| {
            if c.is_ascii_alphanumeric() {
                acc.push(c);
            } else if !acc.ends_with('-') && !acc.is_empty() {
                // Non-ASCII-alphanumeric characters become hyphens (deduplicated)
                acc.push('-');
            }
            acc
        })
        .trim_matches('-')
        .to_string()
}

/// Generate unique branch name for a task
///
/// Format: workflow/{workflow_id}/{slug}-{index}
/// If branch exists, appends -2, -3, etc.
pub fn generate_task_branch_name(
    workflow_id: &str,
    task_name: &str,
    existing_branches: &[String],
) -> String {
    use std::collections::HashSet;

    let base = format!("workflow/{}/{}", workflow_id, slugify(task_name));

    // E28-07: O(1) lookup per iteration. Previously a linear scan that was
    // fine for small N but could degrade once a workflow accumulated many
    // branches. Building the set is O(n) once; each `contains` is then O(1).
    let existing: HashSet<&str> = existing_branches.iter().map(String::as_str).collect();

    let mut candidate = base.clone();
    let mut counter = 2;
    while existing.contains(candidate.as_str()) {
        candidate = format!("{base}-{counter}");
        counter += 1;
    }

    candidate
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_slugify_basic() {
        assert_eq!(slugify("Login Feature"), "login-feature");
        assert_eq!(slugify("User Authentication"), "user-authentication");
    }

    #[test]
    fn test_slugify_special_chars() {
        assert_eq!(slugify("Hello@World!"), "hello-world");
        assert_eq!(slugify("Test#1$Feature"), "test-1-feature");
    }

    #[test]
    fn test_slugify_multiple_spaces() {
        assert_eq!(slugify("Multiple   Spaces"), "multiple-spaces");
    }

    #[test]
    fn test_slugify_trim_hyphens() {
        assert_eq!(slugify("-Leading and Trailing-"), "leading-and-trailing");
    }

    #[test]
    fn test_slugify_chinese_chars() {
        // Chinese characters should be removed (not alphanumeric)
        assert_eq!(slugify("用户登录 User Login"), "user-login");
    }

    #[test]
    fn test_generate_task_branch_name_no_conflicts() {
        let existing = vec![];
        let result = generate_task_branch_name("wf-123", "Login Feature", &existing);
        assert_eq!(result, "workflow/wf-123/login-feature");
    }

    #[test]
    fn test_generate_task_branch_name_with_conflicts() {
        let existing = vec!["workflow/wf-123/login-feature".to_string()];
        let result = generate_task_branch_name("wf-123", "Login Feature", &existing);
        assert_eq!(result, "workflow/wf-123/login-feature-2");
    }

    #[test]
    fn test_generate_task_branch_name_multiple_conflicts() {
        let existing = vec![
            "workflow/wf-123/login-feature".to_string(),
            "workflow/wf-123/login-feature-2".to_string(),
        ];
        let result = generate_task_branch_name("wf-123", "Login Feature", &existing);
        assert_eq!(result, "workflow/wf-123/login-feature-3");
    }
}
