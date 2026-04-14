use std::collections::{HashMap, HashSet, VecDeque};
use std::path::{Path, PathBuf};

use anyhow::Context;
use glob::Pattern;
use serde::Deserialize;
use tracing::debug;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum PackageManager {
    Npm,
    Pnpm,
    Yarn,
    Bun,
}

impl PackageManager {
    pub fn command(self) -> &'static str {
        match self {
            Self::Npm => "npm",
            Self::Pnpm => "pnpm",
            Self::Yarn => "yarn",
            Self::Bun => "bun",
        }
    }

    pub fn script_args(self, script: &str) -> Vec<String> {
        vec!["run".to_string(), script.to_string()]
    }

    pub fn exec_command(self, binary: &str, args: &[String]) -> (String, Vec<String>) {
        match self {
            Self::Npm => {
                let mut full_args = vec![binary.to_string()];
                full_args.extend(args.iter().cloned());
                (resolve_node_exe("npx"), full_args)
            }
            Self::Pnpm => {
                let mut full_args = vec!["exec".to_string(), binary.to_string()];
                full_args.extend(args.iter().cloned());
                (resolve_node_exe("pnpm"), full_args)
            }
            Self::Yarn => {
                let mut full_args = vec!["exec".to_string(), binary.to_string()];
                full_args.extend(args.iter().cloned());
                (resolve_node_exe("yarn"), full_args)
            }
            Self::Bun => {
                let mut full_args = vec![binary.to_string()];
                full_args.extend(args.iter().cloned());
                (resolve_node_exe("bunx"), full_args)
            }
        }
    }

    fn detect(project_root: &Path, start_dir: &Path, package_manager_field: Option<&str>) -> Option<Self> {
        Self::from_package_manager_field(package_manager_field)
            .or_else(|| Self::from_lockfiles(project_root, start_dir))
    }

    fn from_package_manager_field(value: Option<&str>) -> Option<Self> {
        let value = value?.to_ascii_lowercase();
        if value.starts_with("pnpm@") || value == "pnpm" {
            Some(Self::Pnpm)
        } else if value.starts_with("npm@") || value == "npm" {
            Some(Self::Npm)
        } else if value.starts_with("yarn@") || value == "yarn" {
            Some(Self::Yarn)
        } else if value.starts_with("bun@") || value == "bun" {
            Some(Self::Bun)
        } else {
            None
        }
    }

    fn from_lockfiles(project_root: &Path, start_dir: &Path) -> Option<Self> {
        for dir in start_dir.ancestors() {
            if !dir.starts_with(project_root) {
                break;
            }

            for (file_name, manager) in [
                ("pnpm-lock.yaml", Self::Pnpm),
                ("package-lock.json", Self::Npm),
                ("yarn.lock", Self::Yarn),
                ("bun.lockb", Self::Bun),
                ("bun.lock", Self::Bun),
            ] {
                if dir.join(file_name).exists() {
                    return Some(manager);
                }
            }

            if dir == project_root {
                break;
            }
        }

        None
    }
}

pub fn resolve_node_exe(name: &str) -> String {
    #[cfg(windows)]
    {
        if is_explicit_command_path(name) {
            return name.to_string();
        }

        if let Some(path_var) = std::env::var_os("PATH") {
            for dir in std::env::split_paths(&path_var) {
                for candidate in [format!("{name}.cmd"), format!("{name}.exe"), name.to_string()] {
                    let path = dir.join(&candidate);
                    if path.is_file() {
                        return path.to_string_lossy().into_owned();
                    }
                }
            }
        }

        format!("{name}.cmd")
    }

    #[cfg(not(windows))]
    {
        name.to_string()
    }
}

#[cfg(windows)]
fn is_explicit_command_path(name: &str) -> bool {
    // R5 Fix 6 v3: "explicit" means the caller gave us a path we should
    // hand to `CreateProcessW` verbatim. That is:
    //   1. An absolute path (C:\..., \\?\...), OR
    //   2. Contains a path separator (relative with dirs, e.g. `./local/tool`).
    //
    // v2 also short-circuited on "has_extension", but that was too eager:
    // bare names like `tool.exe` (no separator, just an extension) would
    // skip PATH lookup and return verbatim even when `tool.exe` only exists
    // in a PATH dir, not in cwd. CreateProcessW doesn't do PATH lookup on
    // bare names, so that spawn would fail. By removing the extension check,
    // bare `tool.exe` now flows through the PATH-scan path: the resolver
    // tries `tool.exe.cmd`, `tool.exe.exe`, then `tool.exe` in each PATH
    // directory and returns the first hit — correctly resolving the shim.
    //
    // Primary-brain v1 also used `Path::new(name).parent().is_some()` which
    // returns `Some("")` for every non-root path including "npm" — wrongly
    // classifying bare names as explicit and short-circuiting PATH lookup.
    // This v3 form keeps only the two genuine "explicit path" signals.
    let has_separator = name.contains('/') || name.contains('\\');
    Path::new(name).is_absolute() || has_separator
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum NodeQualityCommand {
    Script { script: String },
    PackageExec { binary: String, args: Vec<String> },
}

impl NodeQualityCommand {
    pub fn describe(&self) -> String {
        match self {
            Self::Script { script } => format!("script:{script}"),
            Self::PackageExec { binary, args } => {
                if args.is_empty() {
                    format!("exec:{binary}")
                } else {
                    format!("exec:{binary} {}", args.join(" "))
                }
            }
        }
    }
}

pub fn resolve_node_command(
    package_manager: Option<PackageManager>,
    command: &NodeQualityCommand,
) -> (String, Vec<String>) {
    let package_manager = package_manager.unwrap_or(PackageManager::Npm);
    match command {
        NodeQualityCommand::Script { script } => {
            (resolve_node_exe(package_manager.command()), package_manager.script_args(script))
        }
        NodeQualityCommand::PackageExec { binary, args } => {
            package_manager.exec_command(binary, args)
        }
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct JsTargetCapabilities {
    pub lint: Option<NodeQualityCommand>,
    pub typecheck: Option<NodeQualityCommand>,
    pub test: Option<NodeQualityCommand>,
}

impl JsTargetCapabilities {
    pub fn has_any(&self) -> bool {
        self.lint.is_some() || self.typecheck.is_some() || self.test.is_some()
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct RepoChecks {
    pub generate_types: Option<NodeQualityCommand>,
    pub prepare_db: Option<NodeQualityCommand>,
}

#[derive(Debug, Clone)]
pub struct JsTarget {
    name: Option<String>,
    root: PathBuf,
    manifest_path: PathBuf,
    package_manager: Option<PackageManager>,
    capabilities: JsTargetCapabilities,
    has_tsconfig: bool,
    dependency_names: HashSet<String>,
}

impl JsTarget {
    pub fn name(&self) -> Option<&str> {
        self.name.as_deref()
    }

    pub fn root(&self) -> &Path {
        &self.root
    }

    pub fn manifest_path(&self) -> &Path {
        &self.manifest_path
    }

    pub fn package_manager(&self) -> Option<PackageManager> {
        self.package_manager
    }

    pub fn capabilities(&self) -> &JsTargetCapabilities {
        &self.capabilities
    }

    pub fn has_tsconfig(&self) -> bool {
        self.has_tsconfig
    }

    /// Declared dependency names from the target's `package.json` (deps, devDeps,
    /// peerDeps, optionalDeps merged). Used by Fix #4 to detect imports of
    /// undeclared packages such as `@testing-library/react`.
    pub fn dependency_names(&self) -> &HashSet<String> {
        &self.dependency_names
    }

    pub fn display_name(&self, repo_root: &Path) -> String {
        self.name.clone().unwrap_or_else(|| self.relative_root(repo_root))
    }

    pub fn relative_root(&self, repo_root: &Path) -> String {
        match self.root.strip_prefix(repo_root) {
            Ok(relative) if relative.as_os_str().is_empty() => ".".to_string(),
            Ok(relative) => normalize_relative_path(&relative.to_string_lossy()),
            Err(_) => normalize_relative_path(&self.root.to_string_lossy()),
        }
    }

    pub(crate) fn contains_relative_path(&self, repo_root: &Path, relative_path: &str) -> bool {
        let root = self.relative_root(repo_root);
        if root == "." {
            return true;
        }

        // E37-02: `/` boundary check — equality (empty rest) or `{root}/` prefix
        // ensures we don't match e.g. root="foo" against "foobar/x".
        relative_path == root || relative_path.starts_with(&format!("{root}/"))
    }
}

#[derive(Debug, Clone)]
pub struct RepositoryDiscovery {
    repo_root: PathBuf,
    repo_package_manager: Option<PackageManager>,
    repo_checks: RepoChecks,
    js_targets: Vec<JsTarget>,
    rust_manifests: Vec<PathBuf>,
    js_dependents: Vec<Vec<usize>>,
}

impl RepositoryDiscovery {
    pub fn discover(project_root: &Path) -> anyhow::Result<Self> {
        let repo_root = project_root.to_path_buf();
        let root_manifest_path = project_root.join("package.json");
        let root_manifest = read_manifest(&root_manifest_path)?;
        let repo_package_manager = root_manifest.as_ref().and_then(|manifest| {
            PackageManager::detect(project_root, project_root, manifest.package_manager.as_deref())
        });

        let workspace_patterns = read_workspace_pattern_strings(project_root, root_manifest.as_ref())?;
        let workspace_matchers = compile_workspace_patterns(&workspace_patterns)?;

        let mut js_targets = Vec::new();
        if let Some(root_manifest) = root_manifest.as_ref() {
            let include_root_target = workspace_matchers.is_empty() || project_root.join("tsconfig.json").exists();
            if include_root_target {
                js_targets.push(build_js_target(
                    project_root,
                    project_root,
                    &root_manifest_path,
                    root_manifest,
                    repo_package_manager,
                ));
            }
        }

        let mut scanned_manifests = scan_known_manifests(project_root);
        scanned_manifests.package_json.sort();
        scanned_manifests.cargo_toml.sort();

        if !workspace_matchers.is_empty() {
            scanned_manifests
                .package_json
                .retain(|path| path != &root_manifest_path);

            for manifest_path in &scanned_manifests.package_json {
                let Some(manifest_dir) = manifest_path.parent() else {
                    continue;
                };
                let relative_dir = normalize_relative_path(
                    &manifest_dir
                        .strip_prefix(project_root)
                        .unwrap_or(manifest_dir)
                        .to_string_lossy(),
                );
                // E37-04: normalize Windows backslashes before glob match so
                // patterns like `packages/*` match on Windows paths too.
                let relative_dir = relative_dir.replace('\\', "/");

                if !workspace_matchers.iter().any(|pattern| pattern.matches(&relative_dir)) {
                    continue;
                }

                if let Some(manifest) = read_manifest(manifest_path)? {
                    let package_manager = PackageManager::detect(
                        project_root,
                        manifest_dir,
                        manifest.package_manager.as_deref(),
                    )
                    .or(repo_package_manager);
                    js_targets.push(build_js_target(
                        project_root,
                        manifest_dir,
                        manifest_path,
                        &manifest,
                        package_manager,
                    ));
                }
            }
        }

        let repo_checks = root_manifest
            .as_ref()
            .map(resolve_repo_checks)
            .unwrap_or_default();
        let rust_manifests = scanned_manifests.cargo_toml;
        let js_dependents = build_js_dependents(&js_targets);

        debug!(
            js_targets = js_targets.len(),
            rust_manifests = rust_manifests.len(),
            repo_checks = ?repo_checks,
            "quality discovery completed"
        );

        Ok(Self {
            repo_root,
            repo_package_manager,
            repo_checks,
            js_targets,
            rust_manifests,
            js_dependents,
        })
    }

    pub fn repo_root(&self) -> &Path {
        &self.repo_root
    }

    pub fn repo_package_manager(&self) -> Option<PackageManager> {
        self.repo_package_manager
    }

    pub fn repo_checks(&self) -> &RepoChecks {
        &self.repo_checks
    }

    pub fn js_targets(&self) -> &[JsTarget] {
        &self.js_targets
    }

    pub fn has_js_targets(&self) -> bool {
        !self.js_targets.is_empty()
    }

    pub fn has_rust_targets(&self) -> bool {
        !self.rust_manifests.is_empty()
    }

    pub fn applicable_js_targets(&self, changed_files: Option<&[String]>) -> Vec<&JsTarget> {
        if self.js_targets.is_empty() {
            return Vec::new();
        }

        let Some(files) = changed_files else {
            return self.js_targets.iter().collect();
        };

        if files.is_empty() {
            return self.js_targets.iter().collect();
        }

        let mut directly_selected = Vec::new();
        let mut selected = HashSet::new();
        let mut repo_global_change = false;

        for file in files {
            let normalized = normalize_relative_path(file);
            let mut matched = false;

            for (index, target) in self.js_targets.iter().enumerate() {
                if target.contains_relative_path(&self.repo_root, &normalized) {
                    if selected.insert(index) {
                        directly_selected.push(index);
                    }
                    matched = true;
                }
            }

            if !matched {
                repo_global_change = true;
                break;
            }
        }

        // E37-08: early return once a repo-global change is detected — avoids
        // running dependent-target expansion we'd immediately discard.
        if repo_global_change {
            return self.js_targets.iter().collect();
        }

        let mut queue: VecDeque<usize> = directly_selected.iter().copied().collect();
        let mut expanded = Vec::new();
        while let Some(index) = queue.pop_front() {
            for dependent in &self.js_dependents[index] {
                if selected.insert(*dependent) {
                    expanded.push(*dependent);
                    queue.push_back(*dependent);
                }
            }
        }

        expanded.sort_unstable();
        directly_selected.extend(expanded);
        directly_selected
            .into_iter()
            .map(|index| &self.js_targets[index])
            .collect()
    }
}

#[derive(Debug, Deserialize)]
struct NodeManifest {
    name: Option<String>,
    #[serde(default)]
    scripts: HashMap<String, String>,
    #[serde(default)]
    dependencies: HashMap<String, serde_json::Value>,
    #[serde(default, rename = "devDependencies")]
    dev_dependencies: HashMap<String, serde_json::Value>,
    #[serde(default, rename = "peerDependencies")]
    peer_dependencies: HashMap<String, serde_json::Value>,
    #[serde(default, rename = "packageManager")]
    package_manager: Option<String>,
    #[serde(default)]
    workspaces: Option<WorkspacesField>,
}

impl NodeManifest {
    fn has_script(&self, script: &str) -> bool {
        self.scripts.contains_key(script)
    }

    fn script_body(&self, script: &str) -> Option<&str> {
        self.scripts.get(script).map(|s| s.as_str())
    }

    fn has_dependency(&self, dependency: &str) -> bool {
        self.dependencies.contains_key(dependency)
            || self.dev_dependencies.contains_key(dependency)
            || self.peer_dependencies.contains_key(dependency)
    }

    fn dependency_names(&self) -> HashSet<String> {
        self.dependencies
            .keys()
            .chain(self.dev_dependencies.keys())
            .chain(self.peer_dependencies.keys())
            .cloned()
            .collect()
    }

    fn workspace_patterns(&self) -> Vec<String> {
        match self.workspaces.as_ref() {
            Some(WorkspacesField::Array(patterns)) => patterns.clone(),
            Some(WorkspacesField::Object { packages }) => packages.clone(),
            None => Vec::new(),
        }
    }
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum WorkspacesField {
    Array(Vec<String>),
    Object {
        #[serde(default)]
        packages: Vec<String>,
    },
}

fn read_manifest(path: &Path) -> anyhow::Result<Option<NodeManifest>> {
    if !path.exists() {
        return Ok(None);
    }

    let content = std::fs::read_to_string(path)
        .with_context(|| format!("reading manifest {}", path.display()))?;
    let manifest = serde_json::from_str::<NodeManifest>(&content)
        .with_context(|| format!("parsing manifest {}", path.display()))?;
    Ok(Some(manifest))
}

fn resolve_repo_checks(manifest: &NodeManifest) -> RepoChecks {
    RepoChecks {
        generate_types: manifest
            .has_script("generate-types:check")
            .then(|| NodeQualityCommand::Script {
                script: "generate-types:check".to_string(),
            }),
        prepare_db: manifest
            .has_script("prepare-db:check")
            .then(|| NodeQualityCommand::Script {
                script: "prepare-db:check".to_string(),
            }),
    }
}

/// Return true if the script body looks like a gate-bypass stub, i.e. it
/// contains no token that belongs to a real lint/typecheck/test tool.
///
/// Triggered in R4 after a coder terminal rewrote `type-check` to
/// `echo 'TypeScript check completed successfully' && exit 0` to escape
/// the enforce gate; primary-brain R4-Fix-D required both tool-token
/// gating and stronger alias priority before trusting a script entry.
fn is_stub_script_body(body: &str) -> bool {
    let lower = body.to_lowercase();
    // A body that doesn't mention ANY known quality tool is suspect.
    const TOOL_TOKENS: &[&str] = &[
        "tsc", "eslint", "vitest", "jest", "prettier", "biome", "rome", "swc",
        "vue-tsc", "tsup", "rollup", "vite", "webpack", "xo",
    ];
    let mentions_tool = TOOL_TOKENS.iter().any(|tok| lower.contains(tok));
    if mentions_tool {
        return false;
    }

    // No tool mention — look for the obvious "echo ... && exit 0" pattern or
    // pure `exit 0` / `true` bodies. Do NOT mark as stub if body looks like
    // a legitimate chain into another npm/pnpm/yarn script (in that case
    // the discovered chain still yields a real tool at the leaf; callers
    // already prefer the stronger alias first).
    let trimmed = lower.trim();
    let bypass_markers = ["exit 0", "echo", "true"];
    bypass_markers.iter().any(|m| trimmed.contains(m))
}

fn pick_real_script<'a>(
    manifest: &'a NodeManifest,
    candidates: &[&'a str],
) -> Option<&'a str> {
    // Priority 1: first candidate that exists AND whose body is not a stub.
    for name in candidates {
        if let Some(body) = manifest.script_body(name) {
            if !is_stub_script_body(body) {
                return Some(name);
            }
        }
    }
    None
}

fn resolve_js_capabilities(root: &Path, manifest: &NodeManifest) -> JsTargetCapabilities {
    // R4 Fix D: prefer explicit `quality:*`, `*:all`, `*:ci` aliases BEFORE the
    // plain single-token scripts, because a terminal can rewrite `lint` /
    // `type-check` / `test` to a stub to bypass the enforce gate, while the
    // project still exposes the real check under a more specific alias.
    let lint_candidates = &["quality:lint", "lint:all", "lint:ci", "lint:check", "lint"];
    let typecheck_candidates = &[
        "quality:typecheck",
        "quality:types",
        "type-check:all",
        "type-check:ci",
        "typecheck:all",
        "typecheck:ci",
        "type-check",
        "typecheck",
        "check",
    ];
    let test_candidates = &["quality:test", "test:ci", "test:run", "test"];

    let lint = pick_real_script(manifest, lint_candidates).map(|script| {
        NodeQualityCommand::Script { script: script.to_string() }
    });

    let typecheck = pick_real_script(manifest, typecheck_candidates)
        .map(|script| NodeQualityCommand::Script { script: script.to_string() })
        .or_else(|| {
            (root.join("tsconfig.json").exists() && manifest.has_dependency("typescript")).then(|| {
                NodeQualityCommand::PackageExec {
                    binary: "tsc".to_string(),
                    args: vec!["--noEmit".to_string()],
                }
            })
        });

    let test = pick_real_script(manifest, test_candidates).map(|script| {
        NodeQualityCommand::Script { script: script.to_string() }
    });

    JsTargetCapabilities {
        lint,
        typecheck,
        test,
    }
}

fn build_js_target(
    project_root: &Path,
    root: &Path,
    manifest_path: &Path,
    manifest: &NodeManifest,
    package_manager: Option<PackageManager>,
) -> JsTarget {
    let has_tsconfig = root.join("tsconfig.json").exists();
    let capabilities = resolve_js_capabilities(root, manifest);
    debug!(
        target = %normalize_relative_path(&root.strip_prefix(project_root).unwrap_or(root).to_string_lossy()),
        lint = ?capabilities.lint,
        typecheck = ?capabilities.typecheck,
        test = ?capabilities.test,
        package_manager = ?package_manager,
        "discovered JS target"
    );

    JsTarget {
        name: manifest.name.clone(),
        root: root.to_path_buf(),
        manifest_path: manifest_path.to_path_buf(),
        package_manager,
        capabilities,
        has_tsconfig,
        dependency_names: manifest.dependency_names(),
    }
}

fn build_js_dependents(targets: &[JsTarget]) -> Vec<Vec<usize>> {
    let mut by_name = HashMap::new();
    for (index, target) in targets.iter().enumerate() {
        if let Some(name) = target.name() {
            by_name.insert(name.to_string(), index);
        }
    }

    let mut dependents = vec![Vec::new(); targets.len()];
    for (index, target) in targets.iter().enumerate() {
        for dependency in &target.dependency_names {
            if let Some(target_index) = by_name.get(dependency) {
                dependents[*target_index].push(index);
            }
        }
    }

    for list in &mut dependents {
        list.sort_unstable();
        list.dedup();
    }

    dependents
}

fn read_workspace_pattern_strings(
    project_root: &Path,
    root_manifest: Option<&NodeManifest>,
) -> anyhow::Result<Vec<String>> {
    let mut patterns = root_manifest
        .map(NodeManifest::workspace_patterns)
        .unwrap_or_default();

    patterns.extend(read_pnpm_workspace_patterns(project_root)?);
    patterns.sort();
    patterns.dedup();
    Ok(patterns)
}

fn read_pnpm_workspace_patterns(project_root: &Path) -> anyhow::Result<Vec<String>> {
    let path = project_root.join("pnpm-workspace.yaml");
    if !path.exists() {
        return Ok(Vec::new());
    }

    let content = std::fs::read_to_string(&path)
        .with_context(|| format!("reading pnpm workspace file {}", path.display()))?;
    let workspace = serde_yaml::from_str::<PnpmWorkspace>(&content)
        .with_context(|| format!("parsing pnpm workspace file {}", path.display()))?;
    Ok(workspace.packages)
}

fn compile_workspace_patterns(patterns: &[String]) -> anyhow::Result<Vec<Pattern>> {
    patterns
        .iter()
        .map(|pattern| {
            // E37-10: log a warning so silently-skipped invalid patterns are
            // surfaced; error is still returned to the caller.
            Pattern::new(pattern)
                .inspect_err(|err| {
                    tracing::warn!(
                        pattern = %pattern,
                        error = %err,
                        "invalid workspace pattern; will fail compilation"
                    );
                })
                .with_context(|| format!("invalid workspace pattern: {pattern}"))
        })
        .collect()
}

#[derive(Debug, Default, Deserialize)]
struct PnpmWorkspace {
    #[serde(default)]
    packages: Vec<String>,
}


#[derive(Default)]
struct ScannedManifests {
    package_json: Vec<PathBuf>,
    cargo_toml: Vec<PathBuf>,
}

fn scan_known_manifests(project_root: &Path) -> ScannedManifests {
    let mut found = ScannedManifests::default();
    scan_known_manifests_recursive(project_root, &mut found);
    found
}

fn scan_known_manifests_recursive(current: &Path, found: &mut ScannedManifests) {
    let entries = match std::fs::read_dir(current) {
        Ok(entries) => entries,
        Err(_) => return,
    };

    for entry in entries.flatten() {
        let path = entry.path();
        let Ok(file_type) = entry.file_type() else {
            continue;
        };

        if file_type.is_dir() {
            if should_skip_directory(&path) {
                continue;
            }
            scan_known_manifests_recursive(&path, found);
            continue;
        }

        if !file_type.is_file() {
            continue;
        }

        match path.file_name().and_then(|name| name.to_str()) {
            Some("package.json") => found.package_json.push(path),
            Some("Cargo.toml") => found.cargo_toml.push(path),
            _ => {}
        }
    }
}

fn should_skip_directory(path: &Path) -> bool {
    matches!(
        path.file_name().and_then(|name| name.to_str()),
        Some(".git" | "node_modules" | "target" | "dist" | "build" | "vendor" | ".next" | "coverage")
    )
}

fn normalize_relative_path(path: &str) -> String {
    let normalized = path.replace('\\', "/");
    let normalized = normalized.trim_start_matches("./");
    normalized.trim_matches('/').to_string()
}

#[cfg(test)]
mod tests {
    use super::*;

    fn temp_project_root() -> PathBuf {
        let path = std::env::temp_dir().join(format!("quality-discovery-{}", uuid::Uuid::new_v4()));
        std::fs::create_dir_all(&path).unwrap();
        path
    }

    fn write_file(path: &Path, content: &str) {
        if let Some(parent) = path.parent() {
            std::fs::create_dir_all(parent).unwrap();
        }
        std::fs::write(path, content).unwrap();
    }

    fn cleanup(path: &Path) {
        let _ = std::fs::remove_dir_all(path);
    }

    #[test]
    fn discovers_workspace_targets_without_frontend_name_heuristic() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["backend", "web", "shared"],
  "scripts": {
    "generate-types:check": "npm run gen"
  }
}"#,
        );
        write_file(&root.join("package-lock.json"), "{}");
        write_file(
            &root.join("backend/package.json"),
            r#"{
  "name": "backend",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_file(&root.join("backend/tsconfig.json"), "{}");
        write_file(
            &root.join("web/package.json"),
            r#"{
  "name": "web",
  "scripts": { "lint": "eslint .", "test:run": "vitest run" }
}"#,
        );
        write_file(
            &root.join("shared/package.json"),
            r#"{
  "name": "shared",
  "devDependencies": { "typescript": "^5.0.0" }
}"#,
        );
        write_file(&root.join("shared/tsconfig.json"), "{}");

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let names: Vec<_> = discovery
            .js_targets()
            .iter()
            .map(|target| target.display_name(&root))
            .collect();

        assert_eq!(names, vec!["backend", "shared", "web"]);
        assert!(discovery.repo_checks().generate_types.is_some());
        assert_eq!(discovery.repo_package_manager(), Some(PackageManager::Npm));
        cleanup(&root);
    }

    #[test]
    fn detects_package_exec_typecheck_when_tsconfig_and_typescript_exist() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["pkg"],
  "packageManager": "pnpm@10.0.0"
}"#,
        );
        write_file(
            &root.join("pkg/package.json"),
            r#"{
  "name": "pkg",
  "devDependencies": { "typescript": "^5.0.0" }
}"#,
        );
        write_file(&root.join("pkg/tsconfig.json"), "{}");

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let target = &discovery.js_targets()[0];
        assert_eq!(target.package_manager(), Some(PackageManager::Pnpm));
        assert_eq!(
            target.capabilities().typecheck,
            Some(NodeQualityCommand::PackageExec {
                binary: "tsc".to_string(),
                args: vec!["--noEmit".to_string()],
            })
        );
        cleanup(&root);
    }

    #[test]
    fn discovers_workspace_targets_from_pnpm_workspace_yaml() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "packageManager": "pnpm@10.0.0"
}"#,
        );
        write_file(
            &root.join("pnpm-workspace.yaml"),
            "packages:
  - apps/*
  - packages/*
",
        );
        write_file(
            &root.join("apps/web/package.json"),
            r#"{
  "name": "web",
  "scripts": { "lint": "eslint ." }
}"#,
        );
        write_file(
            &root.join("packages/shared/package.json"),
            r#"{
  "name": "shared",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let roots: Vec<_> = discovery
            .js_targets()
            .iter()
            .map(|target| target.relative_root(&root))
            .collect();

        assert_eq!(roots, vec!["apps/web", "packages/shared"]);
        cleanup(&root);
    }

    #[test]
    fn merges_manifest_workspaces_with_pnpm_workspace_yaml() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["apps/*"]
}"#,
        );
        write_file(
            &root.join("pnpm-workspace.yaml"),
            "packages:
  - apps/*
  - packages/*
",
        );
        write_file(&root.join("apps/web/package.json"), r#"{ "name": "web" }"#);
        write_file(&root.join("packages/shared/package.json"), r#"{ "name": "shared" }"#);

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let roots: Vec<_> = discovery
            .js_targets()
            .iter()
            .map(|target| target.relative_root(&root))
            .collect();

        assert_eq!(roots, vec!["apps/web", "packages/shared"]);
        cleanup(&root);
    }

    #[test]
    fn pnpm_workspace_yaml_patterns_are_deduplicated() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{ "name": "repo", "private": true }"#,
        );
        write_file(
            &root.join("pnpm-workspace.yaml"),
            "packages:
  - apps/*
  - apps/*
  - packages/*
",
        );

        let patterns = read_workspace_pattern_strings(
            &root,
            read_manifest(&root.join("package.json")).unwrap().as_ref(),
        )
        .unwrap();
        assert_eq!(patterns, vec!["apps/*", "packages/*"]);
        cleanup(&root);
    }

    #[test]
    fn invalid_pnpm_workspace_yaml_returns_error() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{ "name": "repo", "private": true }"#,
        );
        write_file(&root.join("pnpm-workspace.yaml"), "packages: [
");

        let error = RepositoryDiscovery::discover(&root).unwrap_err().to_string();
        assert!(error.contains("pnpm workspace file"));
        cleanup(&root);
    }

    #[test]
    fn root_target_is_not_included_for_pnpm_workspace_without_root_tsconfig() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "packageManager": "pnpm@10.0.0"
}"#,
        );
        write_file(&root.join("pnpm-workspace.yaml"), "packages:
  - apps/*
");
        write_file(&root.join("apps/web/package.json"), r#"{ "name": "web" }"#);

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let roots: Vec<_> = discovery
            .js_targets()
            .iter()
            .map(|target| target.relative_root(&root))
            .collect();

        assert_eq!(roots, vec!["apps/web"]);
        cleanup(&root);
    }

    #[test]
    fn root_target_is_included_for_pnpm_workspace_when_root_has_tsconfig() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "packageManager": "pnpm@10.0.0"
}"#,
        );
        write_file(&root.join("tsconfig.json"), "{}");
        write_file(&root.join("pnpm-workspace.yaml"), "packages:
  - apps/*
");
        write_file(&root.join("apps/web/package.json"), r#"{ "name": "web" }"#);

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let roots: Vec<_> = discovery
            .js_targets()
            .iter()
            .map(|target| target.relative_root(&root))
            .collect();

        assert_eq!(roots, vec![".", "apps/web"]);
        cleanup(&root);
    }

    #[test]
    fn expands_affected_targets_to_local_dependents() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["shared", "backend", "web"],
  "packageManager": "npm@10.0.0"
}"#,
        );
        write_file(
            &root.join("shared/package.json"),
            r#"{
  "name": "@repo/shared",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_file(
            &root.join("backend/package.json"),
            r#"{
  "name": "backend",
  "scripts": { "type-check": "tsc --noEmit" },
  "dependencies": { "@repo/shared": "workspace:*" }
}"#,
        );
        write_file(
            &root.join("web/package.json"),
            r#"{
  "name": "web",
  "scripts": { "type-check": "tsc --noEmit" },
  "dependencies": { "@repo/shared": "workspace:*" }
}"#,
        );

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let affected: Vec<_> = discovery
            .applicable_js_targets(Some(&["shared/src/index.ts".to_string()]))
            .into_iter()
            .map(|target| target.display_name(&root))
            .collect();

        assert_eq!(affected, vec!["@repo/shared", "backend", "web"]);
        cleanup(&root);
    }

    #[test]
    fn repo_global_change_selects_all_targets() {
        let root = temp_project_root();
        write_file(
            &root.join("package.json"),
            r#"{
  "name": "repo",
  "private": true,
  "workspaces": ["backend", "frontend"],
  "packageManager": "pnpm@10.0.0"
}"#,
        );
        write_file(
            &root.join("backend/package.json"),
            r#"{
  "name": "backend",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );
        write_file(
            &root.join("frontend/package.json"),
            r#"{
  "name": "frontend",
  "scripts": { "type-check": "tsc --noEmit" }
}"#,
        );

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        let affected: Vec<_> = discovery
            .applicable_js_targets(Some(&["README.md".to_string()]))
            .into_iter()
            .map(|target| target.display_name(&root))
            .collect();

        assert_eq!(affected, vec!["backend", "frontend"]);
        cleanup(&root);
    }

    #[test]
    fn finds_rust_targets_recursively() {
        let root = temp_project_root();
        write_file(&root.join("Cargo.toml"), "[workspace]\nmembers = []\n");
        write_file(
            &root.join("crates/foo/Cargo.toml"),
            "[package]\nname='foo'\nversion='0.1.0'\n",
        );

        let discovery = RepositoryDiscovery::discover(&root).unwrap();
        assert!(discovery.has_rust_targets());
        cleanup(&root);
    }

    #[test]
    fn scan_known_manifests_collects_package_and_cargo_in_one_pass() {
        let root = temp_project_root();
        write_file(&root.join("package.json"), r#"{ "name": "repo" }"#);
        write_file(&root.join("apps/web/package.json"), r#"{ "name": "web" }"#);
        write_file(&root.join("Cargo.toml"), "[workspace]\nmembers = []\n");
        write_file(
            &root.join("crates/foo/Cargo.toml"),
            "[package]\nname='foo'\nversion='0.1.0'\n",
        );

        let manifests = scan_known_manifests(&root);
        assert_eq!(manifests.package_json.len(), 2);
        assert_eq!(manifests.cargo_toml.len(), 2);
        cleanup(&root);
    }

    // ───────────────────────── R4 Fix D tests ─────────────────────────

    #[test]
    fn is_stub_script_body_identifies_echo_exit0_bypass() {
        // R4 Fix D: coder terminal wrote `"type-check": "echo ... && exit 0"`
        // to bypass the enforce gate. We must classify that as a stub.
        assert!(is_stub_script_body("echo 'type-check ok' && exit 0"));
        assert!(is_stub_script_body("echo done"));
        assert!(is_stub_script_body("exit 0"));
        assert!(is_stub_script_body("true"));
    }

    #[test]
    fn is_stub_script_body_preserves_real_tool_calls() {
        // Real tool bodies MUST NOT be flagged as stubs.
        assert!(!is_stub_script_body("tsc --noEmit"));
        assert!(!is_stub_script_body("eslint . --ext ts,tsx"));
        assert!(!is_stub_script_body("vitest run"));
        assert!(!is_stub_script_body("jest --coverage"));
        // Chains that ultimately invoke a real tool are fine.
        assert!(!is_stub_script_body("pnpm -w run lint && prettier --check ."));
        assert!(!is_stub_script_body("npm run type-check:client && npm run type-check:server"));
    }

    #[test]
    fn capabilities_prefer_quality_alias_over_stubbed_plain_alias() {
        // Exact R4 R4 incident shape: coder terminal wrote stub `type-check`
        // but project also exposed real `type-check:all` / `quality:types`.
        // Discovery MUST pick the real alias.
        let manifest = serde_json::from_str::<NodeManifest>(
            r#"{
  "name": "app",
  "scripts": {
    "type-check": "echo 'TypeScript check completed successfully' && exit 0",
    "type-check:all": "tsc --noEmit -p tsconfig.server.json",
    "lint": "echo 'ESLint check completed successfully' && exit 0",
    "lint:all": "eslint . --ext ts,tsx",
    "test": "echo 'Tests completed successfully' && exit 0",
    "test:ci": "vitest run"
  }
}"#,
        )
        .unwrap();
        let root = std::env::temp_dir();
        let caps = resolve_js_capabilities(&root, &manifest);
        // Expect the :all / :ci real aliases to win over the plain stubs.
        match &caps.typecheck {
            Some(NodeQualityCommand::Script { script }) => {
                assert_eq!(script, "type-check:all", "must pick the real alias, not the stub")
            }
            other => panic!("expected typecheck Script, got {other:?}"),
        }
        match &caps.lint {
            Some(NodeQualityCommand::Script { script }) => {
                assert_eq!(script, "lint:all", "must pick the real alias, not the stub")
            }
            other => panic!("expected lint Script, got {other:?}"),
        }
        match &caps.test {
            Some(NodeQualityCommand::Script { script }) => {
                assert_eq!(script, "test:ci", "must pick the real alias, not the stub")
            }
            other => panic!("expected test Script, got {other:?}"),
        }
    }

    #[test]
    fn resolve_node_exe_returns_plain_name_on_non_windows() {
        #[cfg(not(windows))]
        {
            // On Unix, resolver is a pass-through.
            assert_eq!(resolve_node_exe("npm"), "npm".to_string());
            assert_eq!(resolve_node_exe("pnpm"), "pnpm".to_string());
        }
    }

    #[test]
    #[cfg(windows)]
    fn resolve_node_exe_produces_runnable_shape_on_windows() {
        // R5 Fix 6: On Windows, `tokio::process::Command::new("npm")`
        // silently fails because CreateProcessW won't pick up `npm.cmd` on
        // its own. Resolver must return either:
        //  (1) a concrete path ending in .cmd/.exe (when PATH lookup hit),
        //  (2) or the fallback literal "{name}.cmd" (when nothing in PATH
        //      matched — still better than the bare name the gate used to
        //      pass before Fix 6).
        let resolved = resolve_node_exe("npm");
        let lower = resolved.to_lowercase();
        assert!(
            lower.ends_with(".cmd") || lower.ends_with(".exe"),
            "resolver must always return a .cmd/.exe suffix on Windows; got `{resolved}`"
        );
    }

    #[test]
    #[cfg(windows)]
    fn resolve_node_exe_passes_through_explicit_paths() {
        // R5 Fix 6 v3: "explicit" = absolute path OR contains a separator.
        // Primary-brain v2 review caught the v2 bug where a bare extension-
        // bearing name like `tool.exe` short-circuited the PATH scan even
        // when `tool.exe` was only discoverable via PATH. Now only absolute
        // paths and separator-bearing relative paths are passed verbatim;
        // a bare `tool.exe` flows through the PATH scan like any other
        // name the helper hasn't seen before.
        assert_eq!(
            resolve_node_exe("C:\\Program Files\\nodejs\\npm.cmd"),
            "C:\\Program Files\\nodejs\\npm.cmd".to_string()
        );
        assert_eq!(
            resolve_node_exe(".\\local\\tool"),
            ".\\local\\tool".to_string()
        );
        assert_eq!(
            resolve_node_exe("/usr/local/bin/npm"),
            "/usr/local/bin/npm".to_string()
        );
    }

    #[test]
    #[cfg(windows)]
    fn resolve_node_exe_bare_extension_goes_through_path_scan() {
        // v3 regression guard: a bare extension-bearing name must NOT
        // short-circuit. The resolver should run its PATH scan; for an
        // arbitrary unlikely-in-PATH name like this, the scan won't hit
        // any candidate, so the function returns the fallback literal
        // `{name}.cmd` — still distinct from the input verbatim.
        let resolved = resolve_node_exe("extremely-unlikely-shim-name-for-test");
        assert_eq!(
            resolved,
            "extremely-unlikely-shim-name-for-test.cmd".to_string(),
            "bare name with no PATH hit must fall back to `.cmd` suffix"
        );

        // Direct regression guard for the exact v2 bug: a bare name that
        // ALREADY carries a `.exe` suffix must still flow through the PATH
        // scan rather than short-circuiting. With no PATH hit, the resolver
        // falls back to `{name}.cmd` — note this produces `*.exe.cmd`, which
        // is distinct from the input verbatim and proves the branch no
        // longer treats extension-bearing bare names as explicit paths.
        let resolved_exe = resolve_node_exe("extremely-unlikely-shim-name-for-test.exe");
        assert_eq!(
            resolved_exe,
            "extremely-unlikely-shim-name-for-test.exe.cmd".to_string(),
            "bare `.exe` name must not short-circuit — must scan PATH then fall back"
        );
    }

    #[test]
    fn capabilities_falls_back_to_package_exec_when_only_stubs_exist() {
        // No real alias; plain `type-check` is a stub; tsconfig + typescript
        // dep present → fall back to tsc --noEmit via PackageExec.
        let root = temp_project_root();
        write_file(&root.join("tsconfig.json"), r#"{ "compilerOptions": {} }"#);
        let manifest = serde_json::from_str::<NodeManifest>(
            r#"{
  "name": "app",
  "scripts": { "type-check": "echo ok && exit 0" },
  "devDependencies": { "typescript": "^5.0.0" }
}"#,
        )
        .unwrap();
        let caps = resolve_js_capabilities(&root, &manifest);
        match &caps.typecheck {
            Some(NodeQualityCommand::PackageExec { binary, args }) => {
                assert_eq!(binary, "tsc");
                assert_eq!(args, &vec!["--noEmit".to_string()]);
            }
            other => panic!("expected PackageExec tsc, got {other:?}"),
        }
        cleanup(&root);
    }
}
