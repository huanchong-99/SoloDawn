<#
.SYNOPSIS
    GitCortex Windows One-Click Environment Setup
    GitCortex Windows 一键开发环境配置

.DESCRIPTION
    Installs all required development tools and optionally AI CLI tools
    on a bare Windows machine. Supports Chinese and English.

.EXAMPLE
    # Interactive mode (recommended for beginners)
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1

    # Non-interactive: install all required tools, skip AI CLIs
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1 -NonInteractive

    # Install everything including all AI CLIs
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1 -AllAiClis

    # English mode
    powershell -ExecutionPolicy Bypass -File scripts\setup-windows.ps1 -Lang en
#>
param(
    [ValidateSet("zh", "en")]
    [string]$Lang = "",

    [switch]$NonInteractive,

    [switch]$SkipAiClis,

    [switch]$AllAiClis,

    [switch]$SkipProjectSetup,

    [string]$RustToolchain = "nightly-2025-12-04"
)

# ────────────────────────────────────────
# Self-elevate to Administrator if not already
# winget install / VS Build Tools require admin privileges.
#
# NOTE: Right-click "Run with PowerShell" uses:
#   powershell -Command "& 'path\to\script.ps1'"
# This makes $PSCommandPath and $MyInvocation.MyCommand.Path both $null.
# We parse [Environment]::CommandLine as a fallback to find the script path.
# ────────────────────────────────────────

# Resolve script path robustly
$scriptPath = ""
if ($PSCommandPath) {
    $scriptPath = $PSCommandPath
} elseif ($MyInvocation.MyCommand.Path) {
    $scriptPath = $MyInvocation.MyCommand.Path
} elseif ($MyInvocation.ScriptName) {
    $scriptPath = $MyInvocation.ScriptName
}

# Fallback: parse process command line (handles right-click "Run with PowerShell")
if (-not $scriptPath -or -not (Test-Path -LiteralPath $scriptPath -ErrorAction SilentlyContinue)) {
    $cmdLine = [System.Environment]::CommandLine
    if ($cmdLine -match "& '([^']+\.ps1)'") {
        $scriptPath = $Matches[1]
    } elseif ($cmdLine -match '& "([^"]+\.ps1)"') {
        $scriptPath = $Matches[1]
    } elseif ($cmdLine -match '-File\s+"?([^"]+\.ps1)"?') {
        $scriptPath = $Matches[1]
    }
}

$isAdmin = ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole(
    [Security.Principal.WindowsBuiltInRole]::Administrator
)
if (-not $isAdmin) {
    if (-not $scriptPath -or -not (Test-Path -LiteralPath $scriptPath -ErrorAction SilentlyContinue)) {
        Write-Host ""
        Write-Host "[FAIL] Cannot resolve script path for elevation." -ForegroundColor Red
        Write-Host ""
        Write-Host "Please run from an admin PowerShell:" -ForegroundColor Yellow
        Write-Host '  powershell -ExecutionPolicy Bypass -File "path\to\setup-windows.ps1"' -ForegroundColor White
        Write-Host ""
        Write-Host "Or double-click setup-windows.cmd instead." -ForegroundColor Yellow
        Write-Host ""
        cmd /c pause
        exit 1
    }

    # Build argument list — use array form to avoid quoting issues
    $argList = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", "`"$scriptPath`"")
    if ($Lang)             { $argList += @("-Lang", $Lang) }
    if ($NonInteractive)   { $argList += "-NonInteractive" }
    if ($SkipAiClis)       { $argList += "-SkipAiClis" }
    if ($AllAiClis)        { $argList += "-AllAiClis" }
    if ($SkipProjectSetup) { $argList += "-SkipProjectSetup" }
    if ($RustToolchain -ne "nightly-2025-12-04") { $argList += @("-RustToolchain", $RustToolchain) }

    Write-Host "Requesting administrator privileges... / 正在请求管理员权限..." -ForegroundColor Yellow
    try {
        Start-Process powershell.exe -ArgumentList $argList -Verb RunAs -Wait
    } catch {
        Write-Host ""
        Write-Host "[FAIL] Need administrator privileges. / 需要管理员权限。" -ForegroundColor Red
        Write-Host ""
        Write-Host "Please double-click setup-windows.cmd or run from admin PowerShell."
        Write-Host "请双击 setup-windows.cmd 或以管理员身份运行 PowerShell。"
    }
    Write-Host ""
    cmd /c pause
    exit
}

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
try {
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    $OutputEncoding = $utf8
    [Console]::OutputEncoding = $utf8
} catch { }

# Pause before exit so the window stays open
# Uses cmd /c pause — works reliably even in non-interactive -Command mode
function Pause-BeforeExit {
    param([int]$ExitCode = 0)
    Write-Host ""
    cmd /c pause
    exit $ExitCode
}

# Global error trap: if anything goes wrong before we reach the end, show the error and pause
trap {
    Write-Host ""
    Write-Host "[FATAL] $_" -ForegroundColor Red
    Write-Host $_.ScriptStackTrace -ForegroundColor DarkRed
    Pause-BeforeExit 1
}

# ────────────────────────────────────────
# i18n Messages
# ────────────────────────────────────────

$script:Messages = @{
    zh = @{
        LANG_CHOICE_TITLE          = "请选择语言 / Choose language:"
        LANG_CHOICE_PROMPT         = "输入 1/2（默认 1）"
        YESNO_INVALID              = "请输入 y 或 n。"

        TITLE                      = "=== GitCortex Windows 开发环境一键配置 ==="
        SUBTITLE                   = "本脚本将安装 GitCortex 所需的全部开发工具"

        ERR_WINGET_NOT_FOUND       = "未找到 winget，正在自动安装..."
        WINGET_INSTALLING          = "正在下载并安装 winget 及其依赖项..."
        WINGET_DEP_VCLIBS          = "正在下载 VCLibs 框架..."
        WINGET_DEP_XAML            = "正在下载 Microsoft.UI.Xaml..."
        WINGET_DEP_WINGET          = "正在下载 winget..."
        WINGET_INSTALL_OK          = "winget 安装成功"
        WINGET_INSTALL_FAILED      = "winget 自动安装失败。请手动安装 App Installer 后重试。"
        ERR_PS_VERSION             = "需要 PowerShell 5.1 或更高版本，当前版本: {0}"
        ERR_INSTALL_FAILED         = "{0} 安装失败"
        ERR_RETRY                  = "第 {0}/{1} 次重试..."
        ERR_NOT_ADMIN              = "某些工具（如 VS Build Tools）可能需要管理员权限。建议以管理员身份运行此脚本。"

        PHASE_CORE                 = "阶段 1/4: 安装核心系统工具"
        PHASE_RUST                 = "阶段 2/4: 安装 Rust 工具链"
        PHASE_NODE                 = "阶段 3/4: 安装 Node.js 工具"
        PHASE_AI                   = "阶段 4/4: 安装 AI CLI 工具"
        PHASE_PROJECT              = "项目初始化"

        DETECTING                  = "正在检测已安装的工具..."
        ALREADY_INSTALLED          = "{0} 已安装 ({1})，跳过"
        INSTALLING                 = "正在安装 {0}..."
        INSTALL_OK                 = "{0} 安装成功"
        INSTALL_SLOW_WARN          = "注意: {0} 需要从源码编译，可能需要 5-10 分钟，请耐心等待..."
        VS_SLOW_WARN               = "注意: VS Build Tools 安装可能需要 10-20 分钟，请耐心等待..."
        PATH_REFRESHED             = "PATH 环境变量已刷新"

        AI_CLI_TITLE               = "选择要安装的 AI CLI 工具"
        AI_CLI_TOGGLE              = "输入编号切换选择, a=全选, n=全不选, 回车=确认:"
        AI_CLI_NONE_SELECTED       = "未选择任何 AI CLI，跳过安装"
        AI_CLI_INSTALLING          = "将安装 {0} 个 AI CLI..."
        AI_CLI_GH_NEEDED           = "GitHub Copilot 需要 GitHub CLI，正在安装..."

        PROMPT_CONTINUE            = "是否继续？(Y/n)"
        PROMPT_PROJECT_SETUP       = "是否初始化项目？（运行 pnpm install + prepare-db）"
        PROMPT_ENCRYPTION_KEY      = "请输入 GITCORTEX_ENCRYPTION_KEY（必须 32 个字符，直接回车自动生成）:"
        KEY_GENERATED              = "已自动生成加密密钥"

        VERIFY_TITLE               = "=== 安装验证报告 ==="
        VERIFY_TOOL                = "工具"
        VERIFY_STATUS              = "状态"
        VERIFY_VERSION             = "版本"
        VERIFY_OK                  = "OK"
        VERIFY_FAIL                = "FAILED"
        VERIFY_SKIP                = "SKIPPED"
        VERIFY_ALREADY             = "OK"
        VERIFY_SUMMARY             = "结果: {0}/{1} 已安装, {2} 跳过, {3} 失败"

        DONE_SUCCESS               = "环境配置完成！"
        DONE_PARTIAL               = "部分工具安装失败，请查看上方报告。"
        NEXT_STEPS                 = "后续步骤:"
        NEXT_STEP_1                = "1. 关闭并重新打开终端（确保 PATH 生效）"
        NEXT_STEP_2                = "2. 启动开发服务器: pnpm run dev"
        NEXT_STEP_3                = "3. 打开浏览器访问: http://localhost:23457"
        NEXT_STEP_4                = ""
        NEXT_STEP_5                = ""
    }
    en = @{
        LANG_CHOICE_TITLE          = "Choose language / 请选择语言:"
        LANG_CHOICE_PROMPT         = "Enter 1/2 (default 2)"
        YESNO_INVALID              = "Please answer y or n."

        TITLE                      = "=== GitCortex Windows Development Environment Setup ==="
        SUBTITLE                   = "This script will install all development tools required by GitCortex"

        ERR_WINGET_NOT_FOUND       = "winget not found, installing automatically..."
        WINGET_INSTALLING          = "Downloading and installing winget with dependencies..."
        WINGET_DEP_VCLIBS          = "Downloading VCLibs framework..."
        WINGET_DEP_XAML            = "Downloading Microsoft.UI.Xaml..."
        WINGET_DEP_WINGET          = "Downloading winget..."
        WINGET_INSTALL_OK          = "winget installed successfully"
        WINGET_INSTALL_FAILED      = "Failed to install winget automatically. Please install App Installer manually and retry."
        ERR_PS_VERSION             = "PowerShell 5.1+ required. Current version: {0}"
        ERR_INSTALL_FAILED         = "Failed to install {0}"
        ERR_RETRY                  = "Retry {0}/{1}..."
        ERR_NOT_ADMIN              = "Some tools (e.g., VS Build Tools) may require admin privileges. Consider running as Administrator."

        PHASE_CORE                 = "Phase 1/4: Installing core system tools"
        PHASE_RUST                 = "Phase 2/4: Installing Rust toolchain"
        PHASE_NODE                 = "Phase 3/4: Installing Node.js tools"
        PHASE_AI                   = "Phase 4/4: Installing AI CLI tools"
        PHASE_PROJECT              = "Project initialization"

        DETECTING                  = "Detecting installed tools..."
        ALREADY_INSTALLED          = "{0} already installed ({1}), skipping"
        INSTALLING                 = "Installing {0}..."
        INSTALL_OK                 = "{0} installed successfully"
        INSTALL_SLOW_WARN          = "Note: {0} compiles from source and may take 5-10 minutes, please wait..."
        VS_SLOW_WARN               = "Note: VS Build Tools installation may take 10-20 minutes, please wait..."
        PATH_REFRESHED             = "PATH environment variable refreshed"

        AI_CLI_TITLE               = "Select AI CLI tools to install"
        AI_CLI_TOGGLE              = "Enter number to toggle, a=all, n=none, Enter=confirm:"
        AI_CLI_NONE_SELECTED       = "No AI CLI selected, skipping"
        AI_CLI_INSTALLING          = "Installing {0} AI CLI(s)..."
        AI_CLI_GH_NEEDED           = "GitHub Copilot requires GitHub CLI, installing..."

        PROMPT_CONTINUE            = "Continue? (Y/n)"
        PROMPT_PROJECT_SETUP       = "Initialize project? (run pnpm install + prepare-db)"
        PROMPT_ENCRYPTION_KEY      = "Enter GITCORTEX_ENCRYPTION_KEY (exactly 32 chars, press Enter to auto-generate):"
        KEY_GENERATED              = "Auto-generated encryption key"

        VERIFY_TITLE               = "=== Installation Verification Report ==="
        VERIFY_TOOL                = "Tool"
        VERIFY_STATUS              = "Status"
        VERIFY_VERSION             = "Version"
        VERIFY_OK                  = "OK"
        VERIFY_FAIL                = "FAILED"
        VERIFY_SKIP                = "SKIPPED"
        VERIFY_ALREADY             = "OK"
        VERIFY_SUMMARY             = "Result: {0}/{1} installed, {2} skipped, {3} failed"

        DONE_SUCCESS               = "Environment setup complete!"
        DONE_PARTIAL               = "Some tools failed to install. See report above."
        NEXT_STEPS                 = "Next steps:"
        NEXT_STEP_1                = "1. Close and reopen your terminal (to pick up PATH changes)"
        NEXT_STEP_2                = "2. Start dev servers: pnpm run dev"
        NEXT_STEP_3                = "3. Open browser: http://localhost:23457"
        NEXT_STEP_4                = ""
        NEXT_STEP_5                = ""
    }
}

$script:CurrentLang = $Lang

# ────────────────────────────────────────
# AI CLI Data
# ────────────────────────────────────────

$script:AiClis = @(
    @{ Key = "claude-code";  Name = "Claude Code";    Pkg = "@anthropic-ai/claude-code"; Cmd = "claude";       Type = "npm" }
    @{ Key = "gemini-cli";   Name = "Gemini CLI";     Pkg = "@google/gemini-cli";        Cmd = "gemini";       Type = "npm" }
    @{ Key = "codex";        Name = "Codex";           Pkg = "@openai/codex";             Cmd = "codex";        Type = "npm" }
    @{ Key = "amp";          Name = "Amp";             Pkg = "@sourcegraph/amp";          Cmd = "amp";          Type = "npm" }
    @{ Key = "qwen-code";    Name = "Qwen Code";      Pkg = "@qwen-code/qwen-code";     Cmd = "qwen";         Type = "npm" }
    @{ Key = "copilot";      Name = "GitHub Copilot"; Pkg = "github/gh-copilot";         Cmd = "gh";           Type = "gh-ext" }
    @{ Key = "droid";        Name = "Droid";           Pkg = "@kilocode/cli";             Cmd = "droid";        Type = "npm" }
    @{ Key = "opencode";     Name = "Opencode";       Pkg = "opencode-ai";               Cmd = "opencode";     Type = "npm" }
    @{ Key = "cursor-agent"; Name = "Cursor Agent";   Pkg = "cursor-agent";              Cmd = "cursor-agent"; Type = "npm" }
)

# Track results: key -> @{ Status = "OK"|"FAILED"|"SKIPPED"; Version = "..." }
$script:Results = [ordered]@{}

# ────────────────────────────────────────
# Infrastructure Functions
# ────────────────────────────────────────

function T {
    param([string]$Key)
    $langTable = $script:Messages[$script:CurrentLang]
    if ($null -ne $langTable -and $langTable.ContainsKey($Key)) {
        return [string]$langTable[$Key]
    }
    return $Key
}

function Tf {
    param(
        [string]$Key,
        [object[]]$FormatArgs = @()
    )
    if ($null -eq $FormatArgs -or $FormatArgs.Count -eq 0) {
        return (T $Key)
    }
    return [string]::Format((T $Key), $FormatArgs)
}

function Select-Language {
    if ($script:CurrentLang -in @("zh", "en")) { return }

    if ($NonInteractive) {
        $script:CurrentLang = "en"
        return
    }

    Write-Host ""
    Write-Host "请选择语言 / Choose language:" -ForegroundColor White
    Write-Host "  1) 中文"
    Write-Host "  2) English"

    while ($true) {
        $choice = Read-Host "输入 1/2（默认 1） / Enter 1/2 (default 1)"
        if ([string]::IsNullOrWhiteSpace($choice) -or $choice.Trim() -eq "1") {
            $script:CurrentLang = "zh"
            return
        }
        if ($choice.Trim() -eq "2") {
            $script:CurrentLang = "en"
            return
        }
    }
}

function Write-Info {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Cyan
}

function Write-Warn {
    param([string]$Message)
    Write-Host "[WARN] $Message" -ForegroundColor Yellow
}

function Write-Ok {
    param([string]$Message)
    Write-Host "[ OK ] $Message" -ForegroundColor Green
}

function Write-Err {
    param([string]$Message)
    Write-Host "[FAIL] $Message" -ForegroundColor Red
}

function Write-Step {
    param(
        [int]$Current,
        [int]$Total,
        [string]$Message
    )
    Write-Host ""
    Write-Host "[$Current/$Total] $Message" -ForegroundColor Magenta
    Write-Host ("-" * 50) -ForegroundColor DarkGray
}

function Read-YesNo {
    param(
        [string]$Prompt,
        [bool]$Default = $true
    )

    if ($NonInteractive) { return $Default }

    $suffix = if ($Default) { "(Y/n)" } else { "(y/N)" }
    while ($true) {
        $answer = Read-Host "$Prompt $suffix"
        if ([string]::IsNullOrWhiteSpace($answer)) { return $Default }
        $a = $answer.Trim().ToLower()
        if ($a -eq "y" -or $a -eq "yes") { return $true }
        if ($a -eq "n" -or $a -eq "no") { return $false }
        Write-Warn (T "YESNO_INVALID")
    }
}

# ────────────────────────────────────────
# Detection Functions
# ────────────────────────────────────────

function Test-CommandExists {
    param([string]$Command)
    $null -ne (Get-Command $Command -ErrorAction SilentlyContinue)
}

function Get-InstalledVersion {
    param(
        [string]$Command,
        [string[]]$VersionArgs = @("--version")
    )
    try {
        $output = & $Command @VersionArgs 2>&1 | Out-String
        $firstLine = ($output -split "`n")[0].Trim()
        if ($firstLine) { return $firstLine }
    } catch { }
    return $null
}

function Test-WingetAvailable {
    if (-not (Test-CommandExists "winget")) {
        return $false
    }
    try {
        $null = winget --version 2>&1
        return $true
    } catch {
        return $false
    }
}

function Test-VsBuildTools {
    # Check via vswhere (most reliable)
    $vswhere = "${env:ProgramFiles(x86)}\Microsoft Visual Studio\Installer\vswhere.exe"
    if (Test-Path $vswhere) {
        $result = & $vswhere -products 'Microsoft.VisualStudio.Product.BuildTools' `
                  -requires 'Microsoft.VisualStudio.Workload.VCTools' `
                  -property installationPath 2>$null
        if (-not [string]::IsNullOrWhiteSpace($result)) { return $true }

        # Also check for full Visual Studio (has VCTools too)
        $result = & $vswhere -requires 'Microsoft.VisualStudio.Workload.VCTools' `
                  -property installationPath 2>$null
        if (-not [string]::IsNullOrWhiteSpace($result)) { return $true }
    }
    return $false
}

# ────────────────────────────────────────
# Install winget (if missing)
# Downloads from GitHub + Microsoft CDN
# ────────────────────────────────────────

function Install-Winget {
    Write-Info (T "WINGET_INSTALLING")

    # Ensure TLS 1.2 for older Windows 10
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

    $oldProgress = $ProgressPreference
    $ProgressPreference = "SilentlyContinue"  # speeds up Invoke-WebRequest

    $tempDir = Join-Path $env:TEMP "winget-install-$(Get-Random)"
    New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

    try {
        # 1. Query GitHub API for latest winget-cli release
        Write-Info (T "WINGET_DEP_WINGET")
        $releaseJson = Invoke-WebRequest -Uri "https://api.github.com/repos/microsoft/winget-cli/releases/latest" -UseBasicParsing | ConvertFrom-Json

        $bundleAsset = $releaseJson.assets | Where-Object { $_.name -like "*.msixbundle" } | Select-Object -First 1
        $depsAsset   = $releaseJson.assets | Where-Object { $_.name -eq "DesktopAppInstaller_Dependencies.zip" } | Select-Object -First 1
        $licenseAsset = $releaseJson.assets | Where-Object { $_.name -like "*License*.xml" } | Select-Object -First 1

        if (-not $bundleAsset) { throw "Cannot find msixbundle asset in winget-cli release" }

        # 2. Download the dependencies zip (contains VCLibs + UWPDesktop + WindowsAppRuntime, all architectures)
        Write-Info (T "WINGET_DEP_VCLIBS")
        if ($depsAsset) {
            $depsZip = Join-Path $tempDir "deps.zip"
            $depsExtract = Join-Path $tempDir "deps"
            Invoke-WebRequest -Uri $depsAsset.browser_download_url -OutFile $depsZip
            Expand-Archive -Path $depsZip -DestinationPath $depsExtract -Force

            # Install all x64 dependencies
            $depFiles = Get-ChildItem -Path (Join-Path $depsExtract "x64") -Filter "*.appx" -ErrorAction SilentlyContinue
            foreach ($dep in $depFiles) {
                Write-Info "  Installing $($dep.Name)..."
                try {
                    Add-AppxPackage -Path $dep.FullName -ErrorAction Stop
                } catch {
                    Write-Warn "  $($dep.Name): $($_.Exception.Message)"
                }
            }
        } else {
            # Fallback: download VCLibs from Microsoft CDN
            $vcLibsPath = Join-Path $tempDir "Microsoft.VCLibs.x64.14.00.Desktop.appx"
            Invoke-WebRequest -Uri "https://aka.ms/Microsoft.VCLibs.x64.14.00.Desktop.appx" -OutFile $vcLibsPath
            Add-AppxPackage -Path $vcLibsPath -ErrorAction Stop
        }

        # 3. Download and install winget msixbundle
        Write-Info (T "WINGET_DEP_XAML")
        $wingetBundle = Join-Path $tempDir "Microsoft.DesktopAppInstaller.msixbundle"
        Invoke-WebRequest -Uri $bundleAsset.browser_download_url -OutFile $wingetBundle
        Add-AppxPackage -Path $wingetBundle -ErrorAction Stop

        # 4. Provision for all users (best effort, needs license)
        if ($licenseAsset) {
            $licensePath = Join-Path $tempDir "License1.xml"
            Invoke-WebRequest -Uri $licenseAsset.browser_download_url -OutFile $licensePath
            try {
                Add-AppxProvisionedPackage -Online -PackagePath $wingetBundle -LicensePath $licensePath -ErrorAction Stop | Out-Null
            } catch {
                # Normal on some Windows 10 builds
            }
        }

        # 5. Add WindowsApps to PATH for current session
        $windowsApps = Join-Path $env:LOCALAPPDATA "Microsoft\WindowsApps"
        if ($env:Path -notlike "*$windowsApps*") {
            $env:Path = "$windowsApps;$env:Path"
        }

        # 6. Verify
        $wingetExe = Join-Path $windowsApps "winget.exe"
        if (Test-Path $wingetExe) {
            Write-Ok (T "WINGET_INSTALL_OK")
            return $true
        }
        if (Test-CommandExists "winget") {
            Write-Ok (T "WINGET_INSTALL_OK")
            return $true
        }

        Write-Err (T "WINGET_INSTALL_FAILED")
        return $false
    } catch {
        Write-Err (T "WINGET_INSTALL_FAILED")
        Write-Err $_.Exception.Message
        return $false
    } finally {
        $ProgressPreference = $oldProgress
        Remove-Item $tempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

# ────────────────────────────────────────
# PATH Refresh
# ────────────────────────────────────────

function Update-PathEnvironment {
    $machinePath = [System.Environment]::GetEnvironmentVariable("Path", "Machine")
    $userPath = [System.Environment]::GetEnvironmentVariable("Path", "User")
    $env:Path = "$machinePath;$userPath"

    # Ensure cargo bin is in PATH
    $cargoBin = if ($env:CARGO_HOME) { "$env:CARGO_HOME\bin" } else { "$env:USERPROFILE\.cargo\bin" }
    if (Test-Path $cargoBin) {
        if ($env:Path -notlike "*$cargoBin*") {
            $env:Path = "$cargoBin;$env:Path"
        }
    }

    # Ensure npm global bin is in PATH
    try {
        if (Test-CommandExists "npm") {
            $npmPrefix = (& npm config get prefix 2>$null)
            if ($npmPrefix) {
                $npmPrefix = $npmPrefix.Trim()
                if ($npmPrefix -and ($env:Path -notlike "*$npmPrefix*")) {
                    $env:Path = "$npmPrefix;$env:Path"
                }
            }
        }
    } catch { }

    # Refresh CARGO_HOME / RUSTUP_HOME from registry
    $cargoHome = [System.Environment]::GetEnvironmentVariable("CARGO_HOME", "User")
    if ($cargoHome) { $env:CARGO_HOME = $cargoHome }
    $rustupHome = [System.Environment]::GetEnvironmentVariable("RUSTUP_HOME", "User")
    if ($rustupHome) { $env:RUSTUP_HOME = $rustupHome }

    Write-Info (T "PATH_REFRESHED")
}

# ────────────────────────────────────────
# Installation Functions
# ────────────────────────────────────────

function Invoke-WithRetry {
    param(
        [scriptblock]$Action,
        [string]$DisplayName,
        [int]$MaxRetries = 3
    )
    for ($attempt = 1; $attempt -le $MaxRetries; $attempt++) {
        # Temporarily allow stderr from external commands without triggering exceptions
        $oldEAP = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        $global:LASTEXITCODE = 0
        try {
            & $Action
        } catch {
            # Ignore — we check $LASTEXITCODE below
        }
        $ec = $global:LASTEXITCODE
        $ErrorActionPreference = $oldEAP

        if ($ec -eq 0) {
            return $true
        }

        if ($attempt -lt $MaxRetries) {
            Write-Warn (Tf "ERR_RETRY" @($attempt, $MaxRetries))
            Start-Sleep -Seconds ([math]::Pow(2, $attempt))
        }
    }
    Write-Err (Tf "ERR_INSTALL_FAILED" @($DisplayName))
    return $false
}

function Install-WithWinget {
    param(
        [string]$PackageId,
        [string]$DisplayName,
        [string]$Override = ""
    )

    Write-Info (Tf "INSTALLING" @($DisplayName))

    $args_list = @("install", "--id", $PackageId, "-e", "--source", "winget",
                   "--accept-source-agreements", "--accept-package-agreements")
    if ($Override) {
        $args_list += @("--override", $Override)
    }

    $ok = Invoke-WithRetry -DisplayName $DisplayName -Action {
        & winget @args_list 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
    }

    if ($ok) {
        Write-Ok (Tf "INSTALL_OK" @($DisplayName))
    }
    return $ok
}

function Install-WithNpm {
    param(
        [string]$Package,
        [string]$DisplayName
    )

    Write-Info (Tf "INSTALLING" @($DisplayName))

    $ok = Invoke-WithRetry -DisplayName $DisplayName -Action {
        & npm install -g $Package 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
    }

    if ($ok) {
        Write-Ok (Tf "INSTALL_OK" @($DisplayName))
    }
    return $ok
}

function Install-WithCargo {
    param(
        [string]$Package,
        [string]$DisplayName,
        [string]$Features = ""
    )

    Write-Info (Tf "INSTALLING" @($DisplayName))
    Write-Warn (Tf "INSTALL_SLOW_WARN" @($DisplayName))

    $ok = Invoke-WithRetry -DisplayName $DisplayName -MaxRetries 2 -Action {
        if ($Features) {
            & cargo install $Package --features $Features 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
        } else {
            & cargo install $Package 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
        }
    }

    if ($ok) {
        Write-Ok (Tf "INSTALL_OK" @($DisplayName))
    }
    return $ok
}

function Record-Result {
    param(
        [string]$ToolName,
        [string]$Status,
        [string]$Version = "-"
    )
    $script:Results[$ToolName] = @{ Status = $Status; Version = $Version }
}

# ────────────────────────────────────────
# AI CLI Menu
# ────────────────────────────────────────

function Show-AiCliMenu {
    $count = $script:AiClis.Count
    $selected = @($false) * $count

    while ($true) {
        Write-Host ""
        Write-Host "=== $(T 'AI_CLI_TITLE') ===" -ForegroundColor White
        Write-Host ""

        for ($i = 0; $i -lt $count; $i++) {
            $cli = $script:AiClis[$i]
            $mark = if ($selected[$i]) { "x" } else { " " }
            $pkgInfo = if ($cli.Type -eq "gh-ext") { "gh extension" } else { $cli.Pkg }
            $num = $i + 1
            $line = "  [$mark] $num. $($cli.Name)"
            $line = $line.PadRight(32)
            $line += "($pkgInfo)"
            if ($selected[$i]) {
                Write-Host $line -ForegroundColor Green
            } else {
                Write-Host $line
            }
        }

        Write-Host ""
        $input = Read-Host (T "AI_CLI_TOGGLE")

        if ([string]::IsNullOrWhiteSpace($input)) {
            # Confirm selection
            break
        }

        $cmd = $input.Trim().ToLower()
        if ($cmd -eq "a") {
            $selected = @($true) * $count
            continue
        }
        if ($cmd -eq "n") {
            $selected = @($false) * $count
            continue
        }

        # Try parse as number
        $num = 0
        if ([int]::TryParse($cmd, [ref]$num)) {
            if ($num -ge 1 -and $num -le $count) {
                $selected[$num - 1] = -not $selected[$num - 1]
            }
        }
    }

    # Return selected CLI objects
    $result = @()
    for ($i = 0; $i -lt $count; $i++) {
        if ($selected[$i]) {
            $result += $script:AiClis[$i]
        }
    }
    return $result
}

# ────────────────────────────────────────
# Verification Report
# ────────────────────────────────────────

function Show-VerificationReport {
    Write-Host ""
    Write-Host (T "VERIFY_TITLE") -ForegroundColor White
    Write-Host ""

    $colTool = (T "VERIFY_TOOL").PadRight(24)
    $colStatus = (T "VERIFY_STATUS").PadRight(10)
    $colVersion = T "VERIFY_VERSION"
    Write-Host "  $colTool$colStatus$colVersion" -ForegroundColor White
    Write-Host "  $("-" * 24)$("-" * 10)$("-" * 20)" -ForegroundColor DarkGray

    $okCount = 0
    $failCount = 0
    $skipCount = 0
    $total = $script:Results.Count

    foreach ($entry in $script:Results.GetEnumerator()) {
        $toolName = $entry.Key.PadRight(24)
        $status = $entry.Value.Status
        $version = $entry.Value.Version

        $statusText = ""
        $color = "White"
        switch ($status) {
            "OK" {
                $statusText = (T "VERIFY_OK").PadRight(10)
                $color = "Green"
                $okCount++
            }
            "FAILED" {
                $statusText = (T "VERIFY_FAIL").PadRight(10)
                $color = "Red"
                $failCount++
            }
            "SKIPPED" {
                $statusText = (T "VERIFY_SKIP").PadRight(10)
                $color = "Yellow"
                $skipCount++
            }
        }

        Write-Host "  $toolName" -NoNewline
        Write-Host $statusText -NoNewline -ForegroundColor $color
        Write-Host $version
    }

    Write-Host ""
    Write-Host "  $(Tf 'VERIFY_SUMMARY' @($okCount, $total, $skipCount, $failCount))" -ForegroundColor White
    Write-Host ""

    return $failCount
}

# ────────────────────────────────────────
# Pre-scan: Detect what's already installed
# ────────────────────────────────────────

function Get-PreInstallStatus {
    Write-Info (T "DETECTING")

    $status = [ordered]@{
        Git          = $false
        NodeJs       = $false
        VsBuildTools = $false
        Rustup       = $false
        RustNightly  = $false
        CargoWatch   = $false
        SqlxCli      = $false
        Pnpm         = $false
    }

    if (Test-CommandExists "git") {
        $v = Get-InstalledVersion "git" @("--version")
        $status.Git = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("Git", $v))
    }

    if (Test-CommandExists "node") {
        $v = Get-InstalledVersion "node" @("--version")
        $status.NodeJs = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("Node.js", $v))
    }

    if (Test-VsBuildTools) {
        $status.VsBuildTools = "detected"
        Write-Ok (Tf "ALREADY_INSTALLED" @("VS Build Tools", "detected"))
    }

    if (Test-CommandExists "rustup") {
        $v = Get-InstalledVersion "rustup" @("--version")
        $status.Rustup = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("rustup", $v))

        # Check if the required nightly is installed
        try {
            $toolchains = rustup toolchain list 2>&1 | Out-String
            if ($toolchains -like "*$RustToolchain*") {
                $status.RustNightly = $RustToolchain
                Write-Ok (Tf "ALREADY_INSTALLED" @("Rust $RustToolchain", "installed"))
            }
        } catch { }
    }

    if (Test-CommandExists "cargo-watch") {
        $v = Get-InstalledVersion "cargo-watch" @("--version")
        $status.CargoWatch = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("cargo-watch", $v))
    }

    if (Test-CommandExists "sqlx") {
        $v = Get-InstalledVersion "sqlx" @("--version")
        $status.SqlxCli = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("sqlx-cli", $v))
    }

    if (Test-CommandExists "pnpm") {
        $v = Get-InstalledVersion "pnpm" @("--version")
        $status.Pnpm = $v
        Write-Ok (Tf "ALREADY_INSTALLED" @("pnpm", $v))
    }

    return $status
}

# ════════════════════════════════════════
# MAIN
# ════════════════════════════════════════

Select-Language

Write-Host ""
Write-Host (T "TITLE") -ForegroundColor Cyan
Write-Host (T "SUBTITLE") -ForegroundColor DarkGray
Write-Host ""

# Check PowerShell version
if ($PSVersionTable.PSVersion.Major -lt 5) {
    Write-Err (Tf "ERR_PS_VERSION" @($PSVersionTable.PSVersion.ToString()))
    Pause-BeforeExit 1
}

# Check winget — install automatically if missing
if (-not (Test-WingetAvailable)) {
    Write-Warn (T "ERR_WINGET_NOT_FOUND")
    $wingetOk = Install-Winget
    if (-not $wingetOk -or -not (Test-WingetAvailable)) {
        Write-Err (T "WINGET_INSTALL_FAILED")
        Pause-BeforeExit 1
    }
}

# Pre-scan
$preStatus = Get-PreInstallStatus

# ─── Phase 1: Core System Tools ───

Write-Step 1 4 (T "PHASE_CORE")

# Git
if (-not $preStatus.Git) {
    $ok = Install-WithWinget "Git.Git" "Git"
    Update-PathEnvironment
    if ($ok -and (Test-CommandExists "git")) {
        $v = Get-InstalledVersion "git" @("--version")
        Record-Result "Git" "OK" $v
    } else {
        Record-Result "Git" "FAILED"
    }
} else {
    Record-Result "Git" "OK" $preStatus.Git
}

# Node.js
if (-not $preStatus.NodeJs) {
    $ok = Install-WithWinget "OpenJS.NodeJS.LTS" "Node.js LTS"
    Update-PathEnvironment
    if ($ok -and (Test-CommandExists "node")) {
        $v = Get-InstalledVersion "node" @("--version")
        Record-Result "Node.js" "OK" $v
    } else {
        Record-Result "Node.js" "FAILED"
    }
} else {
    Record-Result "Node.js" "OK" $preStatus.NodeJs
}

# VS Build Tools
if (-not $preStatus.VsBuildTools) {
    Write-Warn (T "VS_SLOW_WARN")
    $ok = Install-WithWinget "Microsoft.VisualStudio.2022.BuildTools" "VS Build Tools 2022" `
        "--wait --quiet --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
    if ($ok -and (Test-VsBuildTools)) {
        Record-Result "VS Build Tools" "OK" "detected"
    } else {
        Record-Result "VS Build Tools" "FAILED"
    }
} else {
    Record-Result "VS Build Tools" "OK" "detected"
}

Update-PathEnvironment

# ─── Phase 2: Rust Toolchain ───

Write-Step 2 4 (T "PHASE_RUST")

# rustup
$rustReady = $false
if (-not $preStatus.Rustup) {
    $ok = Install-WithWinget "Rustlang.Rustup" "rustup"
    Update-PathEnvironment

    # winget installs rustup-init but may not run it. Ensure rustup is initialized.
    if (-not (Test-CommandExists "rustup")) {
        # Try to find and run rustup-init manually
        $rustupInit = Join-Path $env:USERPROFILE ".cargo\bin\rustup-init.exe"
        if (-not (Test-Path $rustupInit)) {
            # Also check Program Files
            $candidates = @(
                "${env:ProgramFiles}\Rust stable MSVC\rustup-init.exe",
                "${env:ProgramFiles}\rustup\bin\rustup-init.exe"
            )
            foreach ($c in $candidates) {
                if (Test-Path $c) { $rustupInit = $c; break }
            }
        }
        if (Test-Path $rustupInit) {
            Write-Info "Running rustup-init..."
            & $rustupInit -y --default-toolchain none 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
            Update-PathEnvironment
        }
    }

    # Also ensure .cargo\bin is in PATH even if dir was just created
    $cargoBin = "$env:USERPROFILE\.cargo\bin"
    if ((Test-Path $cargoBin) -and ($env:Path -notlike "*$cargoBin*")) {
        $env:Path = "$cargoBin;$env:Path"
    }

    if (Test-CommandExists "rustup") {
        $rustReady = $true
    } else {
        Record-Result "rustup" "FAILED"
        Record-Result "Rust ($RustToolchain)" "SKIPPED"
        Record-Result "cargo-watch" "SKIPPED"
        Record-Result "sqlx-cli" "SKIPPED"
    }
} else {
    $rustReady = $true
}

# Install nightly toolchain (if rustup is available)
if ($rustReady) {
    $v = Get-InstalledVersion "rustup" @("--version")
    Record-Result "rustup" "OK" $v

    if (-not $preStatus.RustNightly) {
        Write-Info (Tf "INSTALLING" @("Rust $RustToolchain"))
        # Temporarily allow stderr output without triggering exceptions
        $oldEAP = $ErrorActionPreference
        $ErrorActionPreference = "Continue"
        & rustup install $RustToolchain 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
        $rustInstallExitCode = $LASTEXITCODE
        & rustup default $RustToolchain 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
        $ErrorActionPreference = $oldEAP

        # Verify the toolchain was actually installed
        $toolchains = & rustup toolchain list 2>&1 | Out-String
        if ($toolchains -like "*$RustToolchain*") {
            Write-Ok (Tf "INSTALL_OK" @("Rust $RustToolchain"))
            Record-Result "Rust ($RustToolchain)" "OK" $RustToolchain
        } else {
            Write-Err (Tf "ERR_INSTALL_FAILED" @("Rust $RustToolchain"))
            Record-Result "Rust ($RustToolchain)" "FAILED"
        }
    } else {
        Record-Result "Rust ($RustToolchain)" "OK" $preStatus.RustNightly
    }

    Update-PathEnvironment

    # cargo-watch
    if (-not $preStatus.CargoWatch) {
        $ok = Install-WithCargo "cargo-watch" "cargo-watch"
        if ($ok -and (Test-CommandExists "cargo-watch")) {
            $v = Get-InstalledVersion "cargo-watch" @("--version")
            Record-Result "cargo-watch" "OK" $v
        } else {
            Record-Result "cargo-watch" "FAILED"
        }
    } else {
        Record-Result "cargo-watch" "OK" $preStatus.CargoWatch
    }

    # sqlx-cli
    if (-not $preStatus.SqlxCli) {
        $ok = Install-WithCargo "sqlx-cli" "sqlx-cli" "sqlite"
        if ($ok -and (Test-CommandExists "sqlx")) {
            $v = Get-InstalledVersion "sqlx" @("--version")
            Record-Result "sqlx-cli" "OK" $v
        } else {
            Record-Result "sqlx-cli" "FAILED"
        }
    } else {
        Record-Result "sqlx-cli" "OK" $preStatus.SqlxCli
    }
}

Update-PathEnvironment

# ─── Phase 3: Node.js Tools ───

Write-Step 3 4 (T "PHASE_NODE")

if (-not $preStatus.Pnpm) {
    if (Test-CommandExists "npm") {
        $ok = Install-WithNpm "pnpm" "pnpm"
        Update-PathEnvironment
        if ($ok -and (Test-CommandExists "pnpm")) {
            $v = Get-InstalledVersion "pnpm" @("--version")
            Record-Result "pnpm" "OK" $v
        } else {
            Record-Result "pnpm" "FAILED"
        }
    } else {
        Write-Err (Tf "ERR_INSTALL_FAILED" @("pnpm (npm not available)"))
        Record-Result "pnpm" "FAILED"
    }
} else {
    Record-Result "pnpm" "OK" $preStatus.Pnpm
}

# ─── Phase 4: AI CLI Tools ───

Write-Step 4 4 (T "PHASE_AI")

if ($SkipAiClis) {
    Write-Info (T "AI_CLI_NONE_SELECTED")
    foreach ($cli in $script:AiClis) {
        Record-Result $cli.Name "SKIPPED"
    }
} else {
    # Determine which CLIs to install
    $selectedClis = @()
    if ($AllAiClis) {
        $selectedClis = $script:AiClis
    } elseif ($NonInteractive) {
        Write-Info (T "AI_CLI_NONE_SELECTED")
        foreach ($cli in $script:AiClis) {
            Record-Result $cli.Name "SKIPPED"
        }
    } else {
        $selectedClis = Show-AiCliMenu
    }

    if ($selectedClis.Count -eq 0 -and -not $NonInteractive -and -not $AllAiClis) {
        Write-Info (T "AI_CLI_NONE_SELECTED")
        foreach ($cli in $script:AiClis) {
            if (-not $script:Results.Contains($cli.Name)) {
                Record-Result $cli.Name "SKIPPED"
            }
        }
    } else {
        Write-Info (Tf "AI_CLI_INSTALLING" @($selectedClis.Count))

        # Mark unselected as SKIPPED
        foreach ($cli in $script:AiClis) {
            $isSelected = $false
            foreach ($sel in $selectedClis) {
                if ($sel.Key -eq $cli.Key) { $isSelected = $true; break }
            }
            if (-not $isSelected -and -not $script:Results.Contains($cli.Name)) {
                Record-Result $cli.Name "SKIPPED"
            }
        }

        foreach ($cli in $selectedClis) {
            # Check if already installed
            if ($cli.Type -eq "gh-ext") {
                # GitHub Copilot special handling
                if (-not (Test-CommandExists "gh")) {
                    Write-Info (T "AI_CLI_GH_NEEDED")
                    $ok = Install-WithWinget "GitHub.cli" "GitHub CLI"
                    Update-PathEnvironment
                    if (-not $ok -or -not (Test-CommandExists "gh")) {
                        Write-Err (Tf "ERR_INSTALL_FAILED" @("GitHub CLI"))
                        Record-Result $cli.Name "FAILED"
                        continue
                    }
                }

                Write-Info (Tf "INSTALLING" @($cli.Name))
                try {
                    & gh extension install $cli.Pkg 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
                    Write-Ok (Tf "INSTALL_OK" @($cli.Name))
                    Record-Result $cli.Name "OK" "gh extension"
                } catch {
                    Write-Err (Tf "ERR_INSTALL_FAILED" @($cli.Name))
                    Record-Result $cli.Name "FAILED"
                }
            } else {
                # npm-based CLI
                if (Test-CommandExists $cli.Cmd) {
                    $v = Get-InstalledVersion $cli.Cmd @("--version")
                    Write-Ok (Tf "ALREADY_INSTALLED" @($cli.Name, $v))
                    Record-Result $cli.Name "OK" $v
                } else {
                    $ok = Install-WithNpm $cli.Pkg $cli.Name
                    Update-PathEnvironment
                    if ($ok -and (Test-CommandExists $cli.Cmd)) {
                        $v = Get-InstalledVersion $cli.Cmd @("--version")
                        Record-Result $cli.Name "OK" $v
                    } elseif ($ok) {
                        # npm succeeded but command not found (PATH issue)
                        Record-Result $cli.Name "OK" "installed (restart terminal)"
                    } else {
                        Record-Result $cli.Name "FAILED"
                    }
                }
            }
        }
    }
}

# ─── Project Setup ───

if (-not $SkipProjectSetup) {
    # Try to find the project root (scripts/ is inside the project or inside {app}\scripts\)
    $scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
    if (-not $scriptDir) { $scriptDir = Split-Path -Parent $scriptPath }
    $projectDir = Split-Path -Parent $scriptDir
    $packageJson = Join-Path $projectDir "package.json"

    if ((Test-Path $packageJson) -and (Test-CommandExists "pnpm")) {
        Write-Host ""
        Write-Info (T "PHASE_PROJECT")
        Write-Host ("-" * 50) -ForegroundColor DarkGray

        # 1. Generate encryption key and set as persistent env var
        if (-not $env:GITCORTEX_ENCRYPTION_KEY) {
            $chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
            $key = -join (1..32 | ForEach-Object { $chars[(Get-Random -Maximum $chars.Length)] })
            $env:GITCORTEX_ENCRYPTION_KEY = $key
            # Persist to user environment so it survives terminal restarts
            [System.Environment]::SetEnvironmentVariable("GITCORTEX_ENCRYPTION_KEY", $key, "User")
            Write-Ok (T "KEY_GENERATED")
        }

        # 2. pnpm install
        Write-Info (Tf "INSTALLING" @("node_modules"))
        try {
            Push-Location $projectDir
            & pnpm install 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
            Write-Ok (Tf "INSTALL_OK" @("node_modules"))
        } catch {
            Write-Err (Tf "ERR_INSTALL_FAILED" @("pnpm install"))
        } finally {
            Pop-Location
        }

        # 3. prepare-db
        Write-Info (Tf "INSTALLING" @("database"))
        try {
            Push-Location $projectDir
            & pnpm run prepare-db 2>&1 | ForEach-Object { Write-Host "  $_" -ForegroundColor DarkGray }
            Write-Ok (Tf "INSTALL_OK" @("database"))
        } catch {
            Write-Err (Tf "ERR_INSTALL_FAILED" @("prepare-db"))
        } finally {
            Pop-Location
        }
    }
}

# ─── Verification Report ───

$failCount = Show-VerificationReport

Write-Host ""
if ($failCount -eq 0) {
    Write-Ok (T "DONE_SUCCESS")
} else {
    Write-Warn (T "DONE_PARTIAL")
}

Write-Host ""
Write-Host (T "NEXT_STEPS") -ForegroundColor White
foreach ($stepKey in @("NEXT_STEP_1", "NEXT_STEP_2", "NEXT_STEP_3", "NEXT_STEP_4", "NEXT_STEP_5")) {
    $stepText = T $stepKey
    if ($stepText) { Write-Host "  $stepText" }
}
Write-Host ""

Pause-BeforeExit $failCount
