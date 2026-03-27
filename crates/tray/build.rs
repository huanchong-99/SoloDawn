fn main() {
    #[cfg(windows)]
    {
        let mut res = winresource::WindowsResource::new();
        // Embed application icon — shows in Explorer, taskbar, and shortcuts
        res.set_icon("../../installer/assets/solodawn.ico");
        res.set("ProductName", "SoloDawn");
        res.set("FileDescription", "SoloDawn System Tray");
        if let Err(e) = res.compile() {
            eprintln!("cargo:warning=Failed to embed icon resource: {e}");
        }
    }
}
