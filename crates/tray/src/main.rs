//! SoloDawn system tray application for Windows.
//!
//! Manages the lifecycle of `solodawn-server.exe`, provides a system tray
//! icon with context menu for common actions, and reads `.env` configuration
//! from the installation directory.

// Hide console window on Windows
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::{
    env, fs,
    path::{Path, PathBuf},
    process::{Child, Command, Stdio},
    sync::{
        Mutex,
        atomic::{AtomicBool, Ordering},
    },
};

use tray_icon::{
    Icon, TrayIconBuilder,
    menu::{Menu, MenuEvent, MenuItem, PredefinedMenuItem},
};
use winit::{
    application::ApplicationHandler,
    event::WindowEvent,
    event_loop::{ActiveEventLoop, EventLoop},
    window::WindowId,
};

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const DEFAULT_PORT: u16 = 23456;
const SERVER_BINARY: &str = "solodawn-server.exe";

// ---------------------------------------------------------------------------
// Application state
// ---------------------------------------------------------------------------

static SERVER_RUNNING: AtomicBool = AtomicBool::new(false);

struct TrayApp {
    install_dir: PathBuf,
    server_process: Mutex<Option<Child>>,
    // Menu item IDs stored for event matching
    menu_open_id: tray_icon::menu::MenuId,
    menu_start_id: tray_icon::menu::MenuId,
    menu_stop_id: tray_icon::menu::MenuId,
    menu_quit_id: tray_icon::menu::MenuId,
    _tray_icon: Option<tray_icon::TrayIcon>,
}

impl TrayApp {
    fn new(install_dir: PathBuf) -> Self {
        Self {
            install_dir,
            server_process: Mutex::new(None),
            menu_open_id: tray_icon::menu::MenuId::new("open"),
            menu_start_id: tray_icon::menu::MenuId::new("start"),
            menu_stop_id: tray_icon::menu::MenuId::new("stop"),
            menu_quit_id: tray_icon::menu::MenuId::new("quit"),
            _tray_icon: None,
        }
    }

    /// Resolve the server binary path.
    fn server_path(&self) -> PathBuf {
        self.install_dir.join(SERVER_BINARY)
    }

    /// Read the configured port from `.env`, defaulting to 23456.
    fn port(&self) -> u16 {
        let env_path = self.install_dir.join(".env");
        if env_path.exists() {
            if let Ok(contents) = std::fs::read_to_string(&env_path) {
                for line in contents.lines() {
                    let line = line.trim();
                    if let Some(val) = line.strip_prefix("BACKEND_PORT=") {
                        if let Ok(p) = val.trim().parse::<u16>() {
                            return p;
                        }
                    }
                }
            }
        }
        DEFAULT_PORT
    }

    /// Load environment variables from `.env` and return them as a Vec of (key, value).
    fn load_env_vars(&self) -> Vec<(String, String)> {
        let env_path = self.install_dir.join(".env");
        let mut vars = Vec::new();
        if env_path.exists() {
            if let Ok(contents) = std::fs::read_to_string(&env_path) {
                // Strip UTF-8 BOM if present (PowerShell writes BOM with -Encoding UTF8)
                let contents = contents.strip_prefix('\u{FEFF}').unwrap_or(&contents);
                for line in contents.lines() {
                    let line = line.trim();
                    if line.is_empty() || line.starts_with('#') {
                        continue;
                    }
                    if let Some((key, value)) = line.split_once('=') {
                        let key = key.trim();
                        let value = value.trim().trim_matches('"');
                        vars.push((key.to_string(), value.to_string()));
                    }
                }
                tracing::info!("Loaded {} env vars from .env", vars.len());
            }
        }
        vars
    }

    /// Start the server process.
    fn start_server(&self) {
        let server_path = self.server_path();
        if !server_path.exists() {
            tracing::error!("Server binary not found: {}", server_path.display());
            return;
        }

        let mut guard = self
            .server_process
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if guard.is_some() {
            tracing::warn!("Server is already running");
            return;
        }

        let env_vars = self.load_env_vars();

        // Build PATH with legacy bundled tools (backward compat) (order matters: bundled first, system last)
        let mut path_dirs: Vec<String> = Vec::new();
        let node_dir = self.install_dir.join("node_portable");
        if node_dir.exists() {
            path_dirs.push(node_dir.to_string_lossy().to_string());
        }
        // npm global bin directory (where `npm install -g` puts CLI executables)
        let npm_global = self.install_dir.join("node_portable").join("npm-global");
        if npm_global.exists() {
            path_dirs.push(npm_global.to_string_lossy().to_string());
        }
        // Git: PortableGit (has bash.exe)
        let git_cmd = self.install_dir.join("git").join("cmd");
        if git_cmd.exists() {
            path_dirs.push(git_cmd.to_string_lossy().to_string());
        }
        // git usr/bin contains bash.exe (needed by Claude Code CLI)
        let git_usr_bin = self.install_dir.join("git").join("usr").join("bin");
        if git_usr_bin.exists() {
            path_dirs.push(git_usr_bin.to_string_lossy().to_string());
        }
        let gh_dir = self.install_dir.join("gh");
        if gh_dir.exists() {
            path_dirs.push(gh_dir.to_string_lossy().to_string());
        }
        // Append system PATH
        if let Ok(sys_path) = env::var("PATH") {
            path_dirs.push(sys_path);
        }
        let combined_path = path_dirs.join(";");

        // Create logs directory and redirect server output to log file
        let logs_dir = self.install_dir.join("logs");
        let _ = fs::create_dir_all(&logs_dir);

        // Rotate logs: keep daily logs, auto-delete files older than 7 days
        if let Ok(entries) = fs::read_dir(&logs_dir) {
            let week_ago =
                std::time::SystemTime::now() - std::time::Duration::from_secs(7 * 24 * 3600);
            for entry in entries.flatten() {
                if let Ok(meta) = entry.metadata() {
                    if let Ok(modified) = meta.modified() {
                        if modified < week_ago {
                            let _ = fs::remove_file(entry.path());
                            tracing::info!("Deleted old log: {}", entry.path().display());
                        }
                    }
                }
            }
        }

        let log_file_path = logs_dir.join("server.log");

        let stdout_file = fs::File::create(&log_file_path)
            .map(Stdio::from)
            .unwrap_or(Stdio::null());
        let stderr_file = fs::OpenOptions::new()
            .create(true)
            .append(true)
            .open(&log_file_path)
            .map(Stdio::from)
            .unwrap_or(Stdio::null());

        // CREATE_NO_WINDOW (0x08000000) hides the console window on Windows
        #[cfg(target_os = "windows")]
        use std::os::windows::process::CommandExt;

        let mut cmd = Command::new(&server_path);
        cmd.current_dir(&self.install_dir)
            .envs(env_vars)
            .env("PATH", &combined_path)
            .env("SOLODAWN_INSTALL_DIR", &self.install_dir)
            .stdout(stdout_file)
            .stderr(stderr_file);

        #[cfg(target_os = "windows")]
        cmd.creation_flags(0x08000000); // CREATE_NO_WINDOW

        match cmd.spawn() {
            Ok(child) => {
                tracing::info!(
                    "Server started (PID: {}), log: {}",
                    child.id(),
                    log_file_path.display()
                );
                *guard = Some(child);
                SERVER_RUNNING.store(true, Ordering::SeqCst);
            }
            Err(e) => {
                tracing::error!("Failed to start server: {}", e);
                // Also write error to log file for diagnostics
                let _ = fs::write(&log_file_path, format!("Failed to start server: {}\n", e));
            }
        }
    }

    /// Stop the server process.
    fn stop_server(&self) {
        let mut guard = self
            .server_process
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if let Some(mut child) = guard.take() {
            tracing::info!("Stopping server (PID: {})", child.id());
            let _ = child.kill();
            let _ = child.wait();
            SERVER_RUNNING.store(false, Ordering::SeqCst);
            tracing::info!("Server stopped");
        }
    }

    /// Open the SoloDawn web UI in the default browser.
    fn open_browser(&self) {
        let port = self.port();
        let url = format!("http://127.0.0.1:{}", port);
        if let Err(e) = open::that(&url) {
            tracing::error!("Failed to open browser: {}", e);
        }
    }

    /// Create a default icon (orange square).
    fn create_default_icon() -> Icon {
        // 16x16 RGBA orange icon
        let size = 16u32;
        let mut rgba = Vec::with_capacity((size * size * 4) as usize);
        for _ in 0..size * size {
            // Orange: RGB(230, 126, 34)
            rgba.extend_from_slice(&[230, 126, 34, 255]);
        }
        Icon::from_rgba(rgba, size, size).expect("Failed to create default icon")
    }

    /// Try to load the application icon from the assets directory.
    /// Supports both .ico and .png formats.
    fn load_icon(install_dir: &Path) -> Icon {
        // Try .ico first (Windows native), then .png
        let candidates = [
            install_dir.join("assets").join("SoloDawn.ico"),
            install_dir.join("assets").join("solodawn.ico"),
            install_dir.join("assets").join("solodawn.png"),
        ];
        for icon_path in &candidates {
            if icon_path.exists() {
                if let Ok(img) = image::open(icon_path) {
                    // Resize to 32x32 for tray (common size)
                    let resized = img.resize_exact(32, 32, image::imageops::FilterType::Lanczos3);
                    let rgba_image = resized.to_rgba8();
                    let (width, height) = rgba_image.dimensions();
                    if let Ok(icon) = Icon::from_rgba(rgba_image.into_raw(), width, height) {
                        return icon;
                    }
                }
            }
        }
        Self::create_default_icon()
    }
}

// ---------------------------------------------------------------------------
// winit ApplicationHandler
// ---------------------------------------------------------------------------

struct TrayAppHandler {
    app: TrayApp,
}

impl TrayAppHandler {
    /// Detect if the system locale is Chinese.
    fn is_chinese_locale() -> bool {
        #[cfg(target_os = "windows")]
        {
            // Use GetUserDefaultUILanguage to detect Chinese
            // Chinese Simplified: 0x0804, Chinese Traditional: 0x0404
            unsafe extern "system" {
                fn GetUserDefaultUILanguage() -> u16;
            }
            let lang = unsafe { GetUserDefaultUILanguage() };
            let primary = lang & 0x00FF;
            primary == 0x04 // LANG_CHINESE
        }
        #[cfg(not(target_os = "windows"))]
        {
            false
        }
    }
}

impl ApplicationHandler for TrayAppHandler {
    fn resumed(&mut self, _event_loop: &ActiveEventLoop) {
        // Detect Chinese locale for menu labels
        let is_chinese = Self::is_chinese_locale();

        let label_open = if is_chinese {
            "打开 SoloDawn"
        } else {
            "Open SoloDawn"
        };
        let label_start = if is_chinese {
            "启动服务"
        } else {
            "Start Server"
        };
        let label_stop = if is_chinese {
            "停止服务"
        } else {
            "Stop Server"
        };
        let label_quit = if is_chinese { "退出" } else { "Quit" };

        // Build the context menu
        let menu = Menu::new();
        let item_open = MenuItem::with_id(self.app.menu_open_id.clone(), label_open, true, None);
        let item_start = MenuItem::with_id(self.app.menu_start_id.clone(), label_start, true, None);
        let item_stop = MenuItem::with_id(self.app.menu_stop_id.clone(), label_stop, true, None);
        let item_quit = MenuItem::with_id(self.app.menu_quit_id.clone(), label_quit, true, None);

        let _ = menu.append(&item_open);
        let _ = menu.append(&PredefinedMenuItem::separator());
        let _ = menu.append(&item_start);
        let _ = menu.append(&item_stop);
        let _ = menu.append(&PredefinedMenuItem::separator());
        let _ = menu.append(&item_quit);

        let icon = TrayApp::load_icon(&self.app.install_dir);

        match TrayIconBuilder::new()
            .with_menu(Box::new(menu))
            .with_tooltip("SoloDawn")
            .with_icon(icon)
            .build()
        {
            Ok(tray) => {
                self.app._tray_icon = Some(tray);
                tracing::info!("Tray icon created");
            }
            Err(e) => {
                tracing::error!("Failed to create tray icon: {}", e);
            }
        }

        // Auto-start server on launch
        self.app.start_server();
    }

    fn window_event(
        &mut self,
        _event_loop: &ActiveEventLoop,
        _window_id: WindowId,
        _event: WindowEvent,
    ) {
        // No windows to handle
    }

    fn about_to_wait(&mut self, event_loop: &ActiveEventLoop) {
        // Process menu events
        while let Ok(event) = MenuEvent::receiver().try_recv() {
            if event.id == self.app.menu_open_id {
                self.app.open_browser();
            } else if event.id == self.app.menu_start_id {
                self.app.start_server();
            } else if event.id == self.app.menu_stop_id {
                self.app.stop_server();
            } else if event.id == self.app.menu_quit_id {
                self.app.stop_server();
                event_loop.exit();
            }
        }

        // Check if server process has exited unexpectedly
        let mut guard = self
            .app
            .server_process
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        if let Some(ref mut child) = *guard {
            match child.try_wait() {
                Ok(Some(status)) => {
                    tracing::warn!("Server exited with status: {}", status);
                    *guard = None;
                    SERVER_RUNNING.store(false, Ordering::SeqCst);
                }
                Ok(None) => {} // Still running
                Err(e) => {
                    tracing::error!("Error checking server status: {}", e);
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Entry point
// ---------------------------------------------------------------------------

fn main() {
    // Determine install directory (parent of this executable)
    let install_dir = env::current_exe()
        .ok()
        .and_then(|p| p.parent().map(|p| p.to_path_buf()))
        .unwrap_or_else(|| env::current_dir().expect("Cannot determine working directory"));

    // Initialize logging to file (since windows_subsystem = "windows" has no console)
    let logs_dir = install_dir.join("logs");
    let _ = fs::create_dir_all(&logs_dir);
    let tray_log_path = logs_dir.join("tray.log");

    let log_file = fs::File::create(&tray_log_path).ok();
    if let Some(file) = log_file {
        tracing_subscriber::fmt()
            .with_env_filter(
                tracing_subscriber::EnvFilter::try_from_default_env()
                    .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
            )
            .with_writer(file)
            .with_ansi(false)
            .init();
    } else {
        tracing_subscriber::fmt()
            .with_env_filter(
                tracing_subscriber::EnvFilter::try_from_default_env()
                    .unwrap_or_else(|_| tracing_subscriber::EnvFilter::new("info")),
            )
            .init();
    }

    tracing::info!("Install directory: {}", install_dir.display());

    // Load .env file
    let env_path = install_dir.join(".env");
    if env_path.exists() {
        tracing::info!(".env file found at: {}", env_path.display());
        let _ = dotenv::from_path(&env_path);
    } else {
        tracing::warn!(".env file NOT found at: {}", env_path.display());
    }

    let app = TrayApp::new(install_dir);

    let event_loop = EventLoop::new().expect("Failed to create event loop");
    event_loop.set_control_flow(winit::event_loop::ControlFlow::Wait);

    let mut handler = TrayAppHandler { app };

    if let Err(e) = event_loop.run_app(&mut handler) {
        tracing::error!("Event loop error: {}", e);
    }
}
