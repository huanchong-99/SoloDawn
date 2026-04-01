param(
    [ValidateSet("install", "update")]
    [string]$Mode = "",
    [string]$HostWorkspaceRoot,
    [string]$Port = "23456",
    [string]$RustLog = "info",
    [switch]$InstallAiClis,
    [string]$DockerApiToken = "",
    [string]$EncryptionKey,
    [string]$AnthropicApiKey = "",
    [string]$OpenAiApiKey = "",
    [string]$GoogleApiKey = "",
    [switch]$SkipBuild,
    [switch]$SkipStart,
    [switch]$ResetDataVolume,
    [switch]$NonInteractive,
    [switch]$Force,
    [switch]$PullLatest,
    [switch]$PullBaseImages,
    [switch]$AllowDirty,
    [switch]$EnableAutoSetupProjects,
    [ValidateSet("official", "china")]
    [string]$BuildNetworkProfile = "",
    [string]$ImageRegistry = "",
    [string]$ImageNamespace = "",
    [ValidateSet("always", "missing", "never")]
    [string]$ImagePullPolicy = "missing",
    [switch]$PreferPrebuiltImage,
    [ValidateSet("zh", "en")]
    [string]$Lang = ""
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"
try {
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    $OutputEncoding = $utf8
    [Console]::OutputEncoding = $utf8
} catch {
    # Best-effort only; continue even if host console doesn't allow changing encoding.
}

$script:Messages = @{
    zh = @{
        YESNO_INVALID = "请输入 y 或 n。"
        LANG_CHOICE_TITLE = "请选择语言 / Choose language:"
        LANG_CHOICE_PROMPT = "输入 1/2（默认 1）"

        ERR_COMPOSE_NOT_FOUND = "未找到 Compose 文件: {0}"
        ERR_DOCKER_NOT_FOUND = "系统 PATH 中未找到 Docker。请先安装并启动 Docker Desktop。"
        ERR_HOST_PATH_MISSING = "主机工作区路径不存在: {0}"
        ERR_HOST_PATH_REQUIRED = "没有可用的主机工作区路径，安装终止。"
        ERR_KEY_LEN = "SOLODAWN_ENCRYPTION_KEY 必须恰好 32 个字符。"
        ERR_ENV_EXISTS = ".env 已存在于 {0}。如需覆盖，请加 -Force。"
        ERR_UPDATE_ENV_MISSING = "更新模式需要现有的 .env 文件，但未找到: {0}"
        ERR_INSTALL_CANCELLED = "用户取消安装。"
        ERR_COMPOSE_CONFIG = "docker compose 配置校验失败。"
        ERR_BUILD_FAILED = "docker compose build 失败。"
        ERR_UP_FAILED = "docker compose up 失败。"
        ERR_DOWN_FAILED = "docker compose down -v 失败。"

        TITLE = "=== SoloDawn Docker 一键安装 ==="
        ISOLATION_TITLE = "隔离模型："
        ISOLATION_1 = "1) 容器始终可访问自身文件系统。"
        ISOLATION_2 = "2) 容器可访问挂载到 {0} 的主机目录。"
        ISOLATION_3 = "3) 默认不会访问你的整块磁盘。"

        PROMPT_HOST_WORKSPACE = "要挂载到容器 {0} 的主机目录"
        PROMPT_PORT = "SoloDawn 主机端口"
        PROMPT_RUST_LOG = "RUST_LOG 日志级别"
        PROMPT_INSTALL_AI_CLIS = "构建镜像时安装 AI CLI（更慢，但更省事）"
        PROMPT_WEAK_NETWORK_PROFILE = "启用弱网/中国网络优化（推荐中国用户开启）"
        PROMPT_AUTO_SETUP_PROJECTS = "首次启动自动创建项目（最多 3 个）"
        PROMPT_RUN_BUILD = "现在执行 docker compose build"
        PROMPT_RUN_UP = "现在执行 docker compose up -d"
        PROMPT_RESET_DATA_VOLUME = "启动前清理旧容器和数据卷（会删除已有项目数据）"
        PROMPT_AUTO_KEY = "自动生成 32 位加密密钥"
        PROMPT_INPUT_KEY = "输入 SOLODAWN_ENCRYPTION_KEY（必须 32 个字符）"
        PROMPT_SET_API_TOKEN = "是否配置 Docker API Bearer Token"
        PROMPT_SET_ANTHROPIC = "现在设置 ANTHROPIC_API_KEY"
        PROMPT_SET_OPENAI = "现在设置 OPENAI_API_KEY"
        PROMPT_SET_GOOGLE = "现在设置 GOOGLE_API_KEY"
        PROMPT_CREATE_MISSING_PATH = "目录不存在，是否立即创建"
        PROMPT_OVERWRITE_ENV = ".env 已存在，是否覆盖"
        PROMPT_RUN_UPDATE_FLOW = "检测到已有 Docker 配置，是否改为执行更新流程"
        PROMPT_USE_PREBUILT_IMAGE = "优先尝试拉取预构建镜像（推荐弱网用户开启）"

        INFO_KEY_GENERATED = "已生成加密密钥。"
        INFO_KEY_GENERATED_NON_INTERACTIVE = "非交互模式：已自动生成 32 位加密密钥。"
        INFO_PATH_CREATED = "已创建目录: {0}"
        INFO_MOUNT_SUMMARY = "挂载摘要："
        INFO_VALIDATING = "正在校验 compose 配置..."
        INFO_BUILDING = "正在构建 Docker 镜像..."
        INFO_BUILD_NETWORK_PROFILE = "构建网络配置: {0}"
        INFO_BUILD_WATCHDOG = "构建已超过 5 分钟，开始巡检是否卡住..."
        INFO_BUILD_POLL = "构建仍在进行，已耗时 {0}，距上次输出 {1} 秒。"
        INFO_BUILD_RETRY = "检测到可重试的构建失败，准备进行第 {0}/{1} 次重试..."
        INFO_PRUNING_BUILD_CACHE = "正在清理 BuildKit 执行缓存后重试..."
        INFO_STARTING = "正在启动容器..."
        INFO_RESETTING_DATA = "正在清理旧容器和数据卷..."
        INFO_CHECKING_READY = "正在检查服务就绪: {0}"
        INFO_ENV_REUSED = "保留已有配置文件: {0}"
        INFO_EXISTING_ENV = "检测到现有 Docker 配置文件: {0}"
        INFO_HANDOFF_UPDATE = "将复用当前 Docker 配置并切换到更新流程..."
        INFO_TRY_PULL_PREBUILT = "正在尝试拉取预构建镜像: {0}"
        INFO_PREBUILT_PULL_FAILED = "预构建镜像拉取失败，将回退到本地构建。"
        INFO_PREBUILT_PRESENT = "检测到本地已存在预构建镜像: {0}"
        INFO_PREBUILT_USED = "已使用预构建镜像，跳过本地构建。"

        OK_ENV_WRITTEN = "已写入配置文件: {0}"
        OK_COMPOSE_VALID = "Compose 配置校验通过。"
        OK_BUILD_DONE = "镜像构建完成。"
        OK_STARTED = "容器已启动。"
        OK_READY = "服务已就绪。"
        OK_DONE = "安装完成。"

        WARN_READY_TIMEOUT = "服务未在预期时间内就绪，请查看日志: docker compose -f {0} logs -f"
        WARN_KEY_LEN = "密钥长度必须恰好为 32。"
        WARN_BUILD_STALLED = "构建连续 {0} 秒无输出，判定为卡住。"
        WARN_BUILD_LOG_TAIL = "最后 60 行构建日志："

        INFO_EXISTING_IMAGE = "检测到已有镜像: {0}"
        INFO_BUILD_SKIPPED = "已跳过构建，复用现有镜像。"
        PROMPT_REBUILD_IMAGE = "是否重新构建镜像（选 N 复用现有镜像）"

        OPEN_URL = "访问地址: http://localhost:{0}"
        USEFUL_COMMANDS = "常用命令："
        UPDATE_COMMAND = "后续更新命令："
    }
    en = @{
        YESNO_INVALID = "Please answer y or n."
        LANG_CHOICE_TITLE = "Choose language / 请选择语言:"
        LANG_CHOICE_PROMPT = "Enter 1/2 (default 2)"

        ERR_COMPOSE_NOT_FOUND = "Compose file not found: {0}"
        ERR_DOCKER_NOT_FOUND = "Docker is not available in PATH. Please install and start Docker Desktop first."
        ERR_HOST_PATH_MISSING = "Host workspace path does not exist: {0}"
        ERR_HOST_PATH_REQUIRED = "Cannot continue without host workspace path."
        ERR_KEY_LEN = "SOLODAWN_ENCRYPTION_KEY must be exactly 32 chars."
        ERR_ENV_EXISTS = ".env already exists at {0}. Re-run with -Force to overwrite."
        ERR_UPDATE_ENV_MISSING = "Update mode requires an existing .env file, but none was found: {0}"
        ERR_INSTALL_CANCELLED = "Installation cancelled by user."
        ERR_COMPOSE_CONFIG = "docker compose config validation failed."
        ERR_BUILD_FAILED = "docker compose build failed."
        ERR_UP_FAILED = "docker compose up failed."
        ERR_DOWN_FAILED = "docker compose down -v failed."

        TITLE = "=== SoloDawn Docker One-Click Installer ==="
        ISOLATION_TITLE = "Isolation model:"
        ISOLATION_1 = "1) Container can always access its own filesystem."
        ISOLATION_2 = "2) Container can also access host path mounted to {0}."
        ISOLATION_3 = "3) By default, it cannot access your full disk."

        PROMPT_HOST_WORKSPACE = "Host folder to mount into container {0}"
        PROMPT_PORT = "Host port for SoloDawn"
        PROMPT_RUST_LOG = "RUST_LOG level"
        PROMPT_INSTALL_AI_CLIS = "Install AI CLIs during image build (slower but turnkey)"
        PROMPT_WEAK_NETWORK_PROFILE = "Enable weak-network / China optimizations (recommended for users in China)"
        PROMPT_AUTO_SETUP_PROJECTS = "Auto-create starter projects on first launch (up to 3)"
        PROMPT_RUN_BUILD = "Run docker compose build now"
        PROMPT_RUN_UP = "Run docker compose up -d now"
        PROMPT_RESET_DATA_VOLUME = "Clean existing containers and data volume first (deletes existing projects)"
        PROMPT_AUTO_KEY = "Auto-generate a 32-char encryption key"
        PROMPT_INPUT_KEY = "Enter SOLODAWN_ENCRYPTION_KEY (exactly 32 chars)"
        PROMPT_SET_API_TOKEN = "Configure Docker API Bearer token"
        PROMPT_SET_ANTHROPIC = "Set ANTHROPIC_API_KEY now"
        PROMPT_SET_OPENAI = "Set OPENAI_API_KEY now"
        PROMPT_SET_GOOGLE = "Set GOOGLE_API_KEY now"
        PROMPT_CREATE_MISSING_PATH = "Path does not exist. Create it now"
        PROMPT_OVERWRITE_ENV = ".env already exists. Overwrite it"
        PROMPT_RUN_UPDATE_FLOW = "Existing Docker config detected. Switch to update flow instead"
        PROMPT_USE_PREBUILT_IMAGE = "Prefer pulling prebuilt image first (recommended on weak networks)"

        INFO_KEY_GENERATED = "Encryption key generated."
        INFO_KEY_GENERATED_NON_INTERACTIVE = "Non-interactive mode: generated 32-char encryption key."
        INFO_PATH_CREATED = "Created: {0}"
        INFO_MOUNT_SUMMARY = "Mount summary:"
        INFO_VALIDATING = "Validating compose file..."
        INFO_BUILDING = "Building Docker image..."
        INFO_BUILD_NETWORK_PROFILE = "Build network profile: {0}"
        INFO_BUILD_WATCHDOG = "Build has run for over 5 minutes. Starting stall watchdog..."
        INFO_BUILD_POLL = "Build still running. Elapsed {0}, last output {1}s ago."
        INFO_BUILD_RETRY = "Detected a retryable build failure. Starting retry {0}/{1}..."
        INFO_PRUNING_BUILD_CACHE = "Pruning BuildKit exec cache before retry..."
        INFO_STARTING = "Starting containers..."
        INFO_RESETTING_DATA = "Removing existing containers and data volume..."
        INFO_CHECKING_READY = "Checking readiness: {0}"
        INFO_ENV_REUSED = "Keeping existing config file: {0}"
        INFO_EXISTING_ENV = "Detected existing Docker config file: {0}"
        INFO_HANDOFF_UPDATE = "Reusing the current Docker config and switching to update flow..."
        INFO_TRY_PULL_PREBUILT = "Trying to pull prebuilt image: {0}"
        INFO_PREBUILT_PULL_FAILED = "Failed to pull prebuilt image. Falling back to local build."
        INFO_PREBUILT_PRESENT = "Prebuilt image already exists locally: {0}"
        INFO_PREBUILT_USED = "Using prebuilt image, local build skipped."

        OK_ENV_WRITTEN = "Wrote config file: {0}"
        OK_COMPOSE_VALID = "Compose configuration is valid."
        OK_BUILD_DONE = "Build completed."
        OK_STARTED = "Container started."
        OK_READY = "Service is ready."
        OK_DONE = "Done."

        WARN_READY_TIMEOUT = "Service did not become ready in time. Check: docker compose -f {0} logs -f"
        WARN_KEY_LEN = "Key length must be exactly 32."
        WARN_BUILD_STALLED = "Build produced no output for {0}s and is treated as stalled."
        WARN_BUILD_LOG_TAIL = "Last 60 build log lines:"

        INFO_EXISTING_IMAGE = "Existing image detected: {0}"
        INFO_BUILD_SKIPPED = "Build skipped, reusing existing image."
        PROMPT_REBUILD_IMAGE = "Rebuild image (choose N to reuse existing)"

        OPEN_URL = "Open: http://localhost:{0}"
        USEFUL_COMMANDS = "Useful commands:"
        UPDATE_COMMAND = "Next update command:"
    }
}

$script:CurrentLang = $Lang

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
    if ($script:CurrentLang -in @("zh", "en")) {
        return
    }

    if ($NonInteractive) {
        $script:CurrentLang = "en"
        return
    }

    Write-Host ""
    Write-Host "请选择语言 / Choose language:" -ForegroundColor White
    Write-Host "1) 中文"
    Write-Host "2) English"

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
    Write-Host "[OK] $Message" -ForegroundColor Green
}

function Format-Duration {
    param([TimeSpan]$Duration)

    if ($Duration.TotalHours -ge 1) {
        return $Duration.ToString("hh\:mm\:ss")
    }

    return $Duration.ToString("mm\:ss")
}

function Write-NewProcessOutput {
    param(
        [string]$Path,
        [ref]$LineCount
    )

    if (-not (Test-Path -LiteralPath $Path)) {
        return
    }

    $lines = @(Get-Content -LiteralPath $Path)
    if ($lines.Count -le $LineCount.Value) {
        return
    }

    for ($i = $LineCount.Value; $i -lt $lines.Count; $i++) {
        Write-Host $lines[$i]
    }

    $LineCount.Value = $lines.Count
}

function Get-LatestOutputTime {
    param(
        [string[]]$Paths,
        [datetime]$Fallback
    )

    $latest = $Fallback
    foreach ($path in $Paths) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }

        $item = Get-Item -LiteralPath $path
        if ($item.Length -gt 0 -and $item.LastWriteTime -gt $latest) {
            $latest = $item.LastWriteTime
        }
    }

    return $latest
}

function Read-FileTextWithRetry {
    param(
        [string]$Path,
        [int]$MaxAttempts = 5,
        [int]$DelayMilliseconds = 200
    )

    for ($attempt = 1; $attempt -le $MaxAttempts; $attempt++) {
        try {
            return [System.IO.File]::ReadAllText($Path)
        }
        catch [System.IO.IOException] {
            if ($attempt -eq $MaxAttempts) {
                throw
            }

            Start-Sleep -Milliseconds $DelayMilliseconds
        }
    }

    return ""
}

function Get-CombinedProcessOutput {
    param([string[]]$Paths)

    $chunks = @()
    foreach ($path in $Paths) {
        if (-not (Test-Path -LiteralPath $path)) {
            continue
        }

        $content = Read-FileTextWithRetry -Path $path
        if (-not [string]::IsNullOrWhiteSpace($content)) {
            $chunks += $content
        }
    }

    return ($chunks -join [Environment]::NewLine)
}

function Get-OutputTail {
    param(
        [string]$Output,
        [int]$MaxLines = 60
    )

    if ([string]::IsNullOrWhiteSpace($Output)) {
        return ""
    }

    $lines = @($Output -split "`r?`n")
    if ($lines.Count -eq 0) {
        return ""
    }

    $start = [Math]::Max(0, $lines.Count - $MaxLines)
    return (($lines[$start..($lines.Count - 1)]) -join [Environment]::NewLine)
}

function Test-RetryableBuildFailure {
    param(
        [string]$Output,
        [bool]$Stalled
    )

    if ($Stalled) {
        return $true
    }

    $patterns = @(
        "ETIMEDOUT",
        "ECONNRESET",
        "ECONNREFUSED",
        "EAI_AGAIN",
        "socket hang up",
        "TLS handshake timeout",
        "i/o timeout",
        "Temporary failure resolving",
        "Could not resolve",
        "no such host",
        "connection reset by peer",
        "Connection timed out",
        "network is unreachable",
        "unexpected EOF",
        "context deadline exceeded",
        "502 Bad Gateway",
        "503 Service Unavailable",
        "504 Gateway Timeout",
        "failed to fetch",
        "Unable to update https://github.com",
        "failed to clone into",
        "could not execute process `git fetch",
        "failed to resolve source metadata",
        "error pulling image configuration",
        "error reading from server: EOF",
        "rpc error: code = Unavailable",
        "failed to receive status"
    )

    foreach ($pattern in $patterns) {
        if ($Output -match $pattern) {
            return $true
        }
    }

    return $false
}

function Invoke-ProcessWithWatchdog {
    param(
        [string]$FilePath,
        [string[]]$ArgumentList,
        [int]$StallTimeoutSeconds = 300,
        [int]$PollIntervalSeconds = 15
    )

    $stdoutFile = [System.IO.Path]::GetTempFileName()
    $stderrFile = [System.IO.Path]::GetTempFileName()
    $stdoutLineCount = 0
    $stderrLineCount = 0
    $startedAt = Get-Date
    $lastOutputAt = $startedAt
    $watchdogStarted = $false
    $stalled = $false

    try {
        $process = Start-Process -FilePath $FilePath -ArgumentList $ArgumentList -PassThru -NoNewWindow `
            -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile

        while (-not $process.HasExited) {
            Start-Sleep -Seconds $PollIntervalSeconds
            $process.Refresh()

            Write-NewProcessOutput -Path $stdoutFile -LineCount ([ref]$stdoutLineCount)
            Write-NewProcessOutput -Path $stderrFile -LineCount ([ref]$stderrLineCount)

            $lastOutputAt = Get-LatestOutputTime -Paths @($stdoutFile, $stderrFile) -Fallback $lastOutputAt
            $elapsed = (Get-Date) - $startedAt

            if ($elapsed.TotalSeconds -ge $StallTimeoutSeconds) {
                if (-not $watchdogStarted) {
                    Write-Info (T "INFO_BUILD_WATCHDOG")
                    $watchdogStarted = $true
                }

                $silentFor = (Get-Date) - $lastOutputAt
                Write-Info (Tf "INFO_BUILD_POLL" @((Format-Duration $elapsed), [int][Math]::Floor($silentFor.TotalSeconds)))

                if ($silentFor.TotalSeconds -ge $StallTimeoutSeconds) {
                    $stalled = $true
                    Write-Warn (Tf "WARN_BUILD_STALLED" @([int][Math]::Floor($silentFor.TotalSeconds)))
                    Stop-Process -Id $process.Id -Force -ErrorAction SilentlyContinue
                    break
                }
            }
        }

        $process.WaitForExit()
        $process.Refresh()

        Write-NewProcessOutput -Path $stdoutFile -LineCount ([ref]$stdoutLineCount)
        Write-NewProcessOutput -Path $stderrFile -LineCount ([ref]$stderrLineCount)

        return @{
            ExitCode = if ($stalled) { -1 } elseif ($null -eq $process.ExitCode) { 0 } else { $process.ExitCode }
            Stalled = $stalled
            Output = Get-CombinedProcessOutput -Paths @($stdoutFile, $stderrFile)
        }
    }
    finally {
        Remove-Item -LiteralPath $stdoutFile, $stderrFile -Force -ErrorAction SilentlyContinue
    }
}

function Invoke-ComposeBuildWithRetry {
    param(
        [string]$ComposeFilePath,
        [string]$EnvFilePath,
        [bool]$ShouldPullBaseImages
    )

    $maxAttempts = 3
    $stallTimeoutSeconds = 180
    $pollIntervalSeconds = 10

    for ($attempt = 1; $attempt -le $maxAttempts; $attempt++) {
        if ($attempt -gt 1) {
            Write-Info (Tf "INFO_BUILD_RETRY" @($attempt, $maxAttempts))
        }

        # Pre-pull base images to avoid BuildKit RPC disconnect from concurrent large downloads
        if ($attempt -eq 1) {
            Write-Host "  Pre-pulling base images to prevent concurrent download overload..."
            & docker pull rust:slim-trixie 2>&1 | Select-Object -Last 1
            & docker pull node:22-slim 2>&1 | Select-Object -Last 1
            & docker pull debian:trixie-slim 2>&1 | Select-Object -Last 1
        }

        $buildArgs = @(
            "compose",
            "--ansi", "never",
            "--progress", "plain",
            "-f", $ComposeFilePath,
            "--env-file", $EnvFilePath,
            "build"
        )
        if ($ShouldPullBaseImages) {
            $buildArgs += "--pull"
        }

        $result = Invoke-ProcessWithWatchdog -FilePath "docker" -ArgumentList $buildArgs `
            -StallTimeoutSeconds $stallTimeoutSeconds -PollIntervalSeconds $pollIntervalSeconds

        $retryable = $false
        if (-not $result.Stalled -and $result.ExitCode -eq 0) {
            # Verify image actually exists (BuildKit RPC disconnect can exit 0 without producing image)
            $null = & docker image inspect compose-solodawn:latest 2>$null
            if ($LASTEXITCODE -ne 0) {
                Write-Warn "Build reported success but image not found. Treating as retryable failure."
                $retryable = $true
            }
            else {
                return
            }
        }

        if (-not $retryable) {
            $retryable = Test-RetryableBuildFailure -Output $result.Output -Stalled $result.Stalled
        }
        if (-not $retryable -or $attempt -eq $maxAttempts) {
            $tail = Get-OutputTail -Output $result.Output
            if (-not [string]::IsNullOrWhiteSpace($tail)) {
                Write-Warn (T "WARN_BUILD_LOG_TAIL")
                Write-Host $tail
            }

            throw (T "ERR_BUILD_FAILED")
        }

        Write-Info (T "INFO_PRUNING_BUILD_CACHE")
        & docker builder prune --force --filter type=exec.cachemount | Out-Null
        Start-Sleep -Seconds ([Math]::Min(30, 10 * $attempt))
    }
}

function Read-Default {
    param(
        [string]$Prompt,
        [string]$DefaultValue
    )
    $raw = Read-Host "$Prompt [$DefaultValue]"
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return $DefaultValue
    }
    return $raw.Trim()
}

function Read-YesNo {
    param(
        [string]$Prompt,
        [bool]$DefaultValue
    )
    $hint = if ($DefaultValue) { "Y/n" } else { "y/N" }
    $enterDefault = if ($DefaultValue) { "Y" } else { "N" }
    $enterHint = if ($script:CurrentLang -eq "zh") {
        "回车=$enterDefault"
    }
    else {
        "Enter=$enterDefault"
    }
    while ($true) {
        $raw = Read-Host "$Prompt ($hint, $enterHint)"
        if ([string]::IsNullOrWhiteSpace($raw)) {
            return $DefaultValue
        }

        switch ($raw.Trim().ToLowerInvariant()) {
            "y" { return $true }
            "yes" { return $true }
            "是" { return $true }
            "1" { return $true }
            "n" { return $false }
            "no" { return $false }
            "否" { return $false }
            "0" { return $false }
            default { Write-Warn (T "YESNO_INVALID") }
        }
    }
}

function New-RandomEncryptionKey {
    $chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
    $bytes = New-Object byte[] 32
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)

    $builder = New-Object System.Text.StringBuilder
    foreach ($b in $bytes) {
        $null = $builder.Append($chars[$b % $chars.Length])
    }
    return $builder.ToString()
}

function Get-EnvValue {
    param(
        [string]$Path,
        [string]$Name
    )

    foreach ($line in Get-Content -LiteralPath $Path) {
        if ([string]::IsNullOrWhiteSpace($line) -or $line.TrimStart().StartsWith("#")) {
            continue
        }

        $parts = $line -split "=", 2
        if ($parts.Count -eq 2 -and $parts[0].Trim() -eq $Name) {
            return $parts[1].Trim()
        }
    }

    return $null
}

function Resolve-ComposePath {
    param([string]$PathValue)

    $resolved = Resolve-Path -LiteralPath $PathValue
    $fullPath = [System.IO.Path]::GetFullPath($resolved.Path)
    return $fullPath.Replace("\", "/")
}

function Wait-Ready {
    param(
        [string]$ReadyUrl,
        [int]$MaxSeconds = 120
    )

    $start = Get-Date
    do {
        try {
            $response = Invoke-WebRequest -Uri $ReadyUrl -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                return $true
            }
        }
        catch {
            Start-Sleep -Seconds 2
        }
    } while (((Get-Date) - $start).TotalSeconds -lt $MaxSeconds)

    return $false
}

function Resolve-PrebuiltImageCandidates {
    param(
        [string]$Registry,
        [string]$Namespace,
        [string]$BuildNetworkProfile
    )

    $repo = if ([string]::IsNullOrWhiteSpace($Namespace)) {
        "solodawn"
    }
    else {
        "$Namespace/solodawn"
    }

    $profileTag = if ($BuildNetworkProfile -eq "china") { "china" } else { "official" }

    $candidates = @(
        "$Registry/$repo:latest-$profileTag",
        "$Registry/$repo:latest"
    )

    return $candidates | Select-Object -Unique
}

function Test-ImageExistsLocal {
    param([string]$Image)

    try {
        $null = & docker image inspect $Image 2>&1
        return ($LASTEXITCODE -eq 0)
    } catch {
        return $false
    }
}

function Try-PullPrebuiltImage {
    param(
        [string[]]$Candidates,
        [string]$TargetTag
    )

    foreach ($candidate in $Candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        if (Test-ImageExistsLocal -Image $candidate) {
            Write-Info (Tf "INFO_PREBUILT_PRESENT" @($candidate))
            try {
                & docker image tag $candidate $TargetTag 2>&1 | Out-Null
                if ($LASTEXITCODE -eq 0) {
                    Write-Info (T "INFO_PREBUILT_USED")
                    return $true
                }
            } catch { }
        }

        Write-Info (Tf "INFO_TRY_PULL_PREBUILT" @($candidate))
        try {
            & docker pull $candidate 2>&1
            if ($LASTEXITCODE -eq 0) {
                try {
                    & docker image tag $candidate $TargetTag 2>&1 | Out-Null
                    if ($LASTEXITCODE -eq 0) {
                        Write-Info (T "INFO_PREBUILT_USED")
                        return $true
                    }
                } catch { }
            }
        } catch { }
    }

    Write-Warn (T "INFO_PREBUILT_PULL_FAILED")
    return $false
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$composeDir = Join-Path $repoRoot "docker\compose"
$composeFile = Join-Path $composeDir "docker-compose.yml"
$envFile = Join-Path $composeDir ".env"
$updateScript = Join-Path $scriptDir "update-docker.ps1"
$workspaceMount = "/workspace"
$dataRoot = "/var/lib/solodawn"

Select-Language

if (-not (Test-Path -LiteralPath $composeFile)) {
    throw (Tf "ERR_COMPOSE_NOT_FOUND" @($composeFile))
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw (T "ERR_DOCKER_NOT_FOUND")
}

$envExists = Test-Path -LiteralPath $envFile

Write-Host ""
Write-Host (T "TITLE") -ForegroundColor Magenta
Write-Host ""
Write-Host (T "ISOLATION_TITLE") -ForegroundColor White
Write-Host (T "ISOLATION_1")
Write-Host (Tf "ISOLATION_2" @($workspaceMount))
Write-Host (T "ISOLATION_3")
Write-Host ""

if ($envExists) {
    Write-Info (Tf "INFO_EXISTING_ENV" @($envFile))
}

if ([string]::IsNullOrWhiteSpace($Mode)) {
    if ($envExists) {
        if ($NonInteractive) {
            $Mode = "update"
        }
        else {
            $Mode = if (Read-YesNo (T "PROMPT_RUN_UPDATE_FLOW") $true) {
                "update"
            }
            else {
                "install"
            }
        }
    }
    else {
        $Mode = "install"
    }
}

if ($Mode -eq "update") {
    if (-not $envExists) {
        throw (Tf "ERR_UPDATE_ENV_MISSING" @($envFile))
    }

    Write-Info (T "INFO_HANDOFF_UPDATE")
    $updateArgs = @{
        ComposeFile = $composeFile
        EnvFile = $envFile
        Lang = $script:CurrentLang
    }

    if ($PSBoundParameters.ContainsKey("Port") -and -not [string]::IsNullOrWhiteSpace($Port)) {
        $updateArgs.Port = $Port
    }
    if ($PullLatest) {
        $updateArgs.PullLatest = $true
    }
    if ($PullBaseImages) {
        $updateArgs.PullBaseImages = $true
    }
    if ($SkipBuild) {
        $updateArgs.SkipBuild = $true
    }
    if ($AllowDirty) {
        $updateArgs.AllowDirty = $true
    }

    & $updateScript @updateArgs
    exit $LASTEXITCODE
}

if ([string]::IsNullOrWhiteSpace($HostWorkspaceRoot)) {
    $HostWorkspaceRoot = $repoRoot
}

$autoSetupProjectsEnabled = $EnableAutoSetupProjects.IsPresent
$resetDataVolume = $ResetDataVolume.IsPresent
$resolvedBuildNetworkProfile = if ([string]::IsNullOrWhiteSpace($BuildNetworkProfile)) {
    "official"
}
else {
    $BuildNetworkProfile
}

$resolvedImageRegistry = if ([string]::IsNullOrWhiteSpace($ImageRegistry)) {
    "ghcr.io"
}
else {
    $ImageRegistry.Trim().TrimEnd("/")
}
$resolvedImageNamespace = if ([string]::IsNullOrWhiteSpace($ImageNamespace)) {
    "huanchong-99"
}
else {
    $ImageNamespace.Trim().Trim("/")
}
$preferPrebuiltImageEnabled = $PreferPrebuiltImage.IsPresent
if ($NonInteractive -and -not $PreferPrebuiltImage.IsPresent) {
    $preferPrebuiltImageEnabled = ($resolvedBuildNetworkProfile -eq "china")
}

if (-not $NonInteractive) {
    $HostWorkspaceRoot = Read-Default (Tf "PROMPT_HOST_WORKSPACE" @($workspaceMount)) $HostWorkspaceRoot
    $Port = Read-Default (T "PROMPT_PORT") $Port
    $RustLog = Read-Default (T "PROMPT_RUST_LOG") $RustLog
    $InstallAiClis = Read-YesNo (T "PROMPT_INSTALL_AI_CLIS") $InstallAiClis.IsPresent
    $resolvedBuildNetworkProfile = if (Read-YesNo (T "PROMPT_WEAK_NETWORK_PROFILE") ($resolvedBuildNetworkProfile -eq "china")) {
        "china"
    }
    else {
        "official"
    }
    $preferPrebuiltImageEnabled = Read-YesNo (T "PROMPT_USE_PREBUILT_IMAGE") ($resolvedBuildNetworkProfile -eq "china")
    $autoSetupProjectsEnabled = Read-YesNo (T "PROMPT_AUTO_SETUP_PROJECTS") $autoSetupProjectsEnabled
    $resetDataVolume = Read-YesNo (T "PROMPT_RESET_DATA_VOLUME") $resetDataVolume
    $SkipBuild = -not (Read-YesNo (T "PROMPT_RUN_BUILD") (-not $SkipBuild.IsPresent))
    $SkipStart = -not (Read-YesNo (T "PROMPT_RUN_UP") (-not $SkipStart.IsPresent))

    if ([string]::IsNullOrWhiteSpace($EncryptionKey)) {
        if (Read-YesNo (T "PROMPT_AUTO_KEY") $true) {
            $EncryptionKey = New-RandomEncryptionKey
            Write-Info (T "INFO_KEY_GENERATED")
        }
        else {
            while ($true) {
                $inputKey = Read-Host (T "PROMPT_INPUT_KEY")
                if ($inputKey.Length -eq 32) {
                    $EncryptionKey = $inputKey
                    break
                }
                Write-Warn (T "WARN_KEY_LEN")
            }
        }
    }

    if ([string]::IsNullOrWhiteSpace($DockerApiToken)) {
        if (Read-YesNo (T "PROMPT_SET_API_TOKEN") $false) {
            $DockerApiToken = Read-Host "SOLODAWN_DOCKER_API_TOKEN"
        }
    }

    if ([string]::IsNullOrWhiteSpace($AnthropicApiKey) -and (Read-YesNo (T "PROMPT_SET_ANTHROPIC") $false)) {
        $AnthropicApiKey = Read-Host "ANTHROPIC_API_KEY"
    }
    if ([string]::IsNullOrWhiteSpace($OpenAiApiKey) -and (Read-YesNo (T "PROMPT_SET_OPENAI") $false)) {
        $OpenAiApiKey = Read-Host "OPENAI_API_KEY"
    }
    if ([string]::IsNullOrWhiteSpace($GoogleApiKey) -and (Read-YesNo (T "PROMPT_SET_GOOGLE") $false)) {
        $GoogleApiKey = Read-Host "GOOGLE_API_KEY"
    }
}
else {
    if ([string]::IsNullOrWhiteSpace($EncryptionKey)) {
        $EncryptionKey = New-RandomEncryptionKey
        Write-Info (T "INFO_KEY_GENERATED_NON_INTERACTIVE")
    }
}

if (-not (Test-Path -LiteralPath $HostWorkspaceRoot)) {
    if ($NonInteractive) {
        throw (Tf "ERR_HOST_PATH_MISSING" @($HostWorkspaceRoot))
    }

    if (Read-YesNo (T "PROMPT_CREATE_MISSING_PATH") $true) {
        $null = New-Item -ItemType Directory -Path $HostWorkspaceRoot -Force
        Write-Info (Tf "INFO_PATH_CREATED" @($HostWorkspaceRoot))
    }
    else {
        throw (T "ERR_HOST_PATH_REQUIRED")
    }
}

if ($EncryptionKey.Length -ne 32) {
    throw (T "ERR_KEY_LEN")
}

$composeHostWorkspaceRoot = Resolve-ComposePath -PathValue $HostWorkspaceRoot
$allowedRoots = "$workspaceMount,$dataRoot"
$installAiClisValue = if ($InstallAiClis) { "1" } else { "0" }
$autoSetupProjectsValue = if ($autoSetupProjectsEnabled) { "1" } else { "0" }

if ((Test-Path -LiteralPath $envFile) -and -not $Force) {
    if ($NonInteractive) {
        throw (Tf "ERR_ENV_EXISTS" @($envFile))
    }
}

$shouldWriteEnv = $true
if ((Test-Path -LiteralPath $envFile) -and -not $Force) {
    if (-not (Read-YesNo (T "PROMPT_OVERWRITE_ENV") $false)) {
        $shouldWriteEnv = $false
        Write-Info (Tf "INFO_ENV_REUSED" @($envFile))
    }
}

if ($shouldWriteEnv) {
$envContent = @"
# Generated by scripts/docker/install-docker.ps1
SOLODAWN_ENCRYPTION_KEY=$EncryptionKey
SOLODAWN_DOCKER_API_TOKEN=$DockerApiToken
ANTHROPIC_API_KEY=$AnthropicApiKey
OPENAI_API_KEY=$OpenAiApiKey
GOOGLE_API_KEY=$GoogleApiKey
PORT=$Port
RUST_LOG=$RustLog
HOST_WORKSPACE_ROOT=$composeHostWorkspaceRoot
SOLODAWN_WORKSPACE_ROOT=$workspaceMount
SOLODAWN_ALLOWED_ROOTS=$allowedRoots
SOLODAWN_BUILD_NETWORK_PROFILE=$resolvedBuildNetworkProfile
SOLODAWN_IMAGE_REGISTRY=$resolvedImageRegistry
SOLODAWN_IMAGE_NAMESPACE=$resolvedImageNamespace
SOLODAWN_IMAGE_PULL_POLICY=$ImagePullPolicy
INSTALL_AI_CLIS=$installAiClisValue
SOLODAWN_AUTO_SETUP_PROJECTS=$autoSetupProjectsValue
"@

[System.IO.File]::WriteAllText($envFile, $envContent, [System.Text.UTF8Encoding]::new($false))
Write-Ok (Tf "OK_ENV_WRITTEN" @($envFile))
}

$effectiveBuildNetworkProfile = $resolvedBuildNetworkProfile
$effectiveImageRegistry = $resolvedImageRegistry
$effectiveImageNamespace = $resolvedImageNamespace
$effectiveImagePullPolicy = $ImagePullPolicy
if (-not $shouldWriteEnv -and (Test-Path -LiteralPath $envFile)) {
    $existingBuildNetworkProfile = Get-EnvValue -Path $envFile -Name "SOLODAWN_BUILD_NETWORK_PROFILE"
    if (-not [string]::IsNullOrWhiteSpace($existingBuildNetworkProfile)) {
        $effectiveBuildNetworkProfile = $existingBuildNetworkProfile
    }
    else {
        $effectiveBuildNetworkProfile = "official"
    }

    $existingImageRegistry = Get-EnvValue -Path $envFile -Name "SOLODAWN_IMAGE_REGISTRY"
    if (-not [string]::IsNullOrWhiteSpace($existingImageRegistry)) {
        $effectiveImageRegistry = $existingImageRegistry.Trim().TrimEnd("/")
    }

    $existingImageNamespace = Get-EnvValue -Path $envFile -Name "SOLODAWN_IMAGE_NAMESPACE"
    if (-not [string]::IsNullOrWhiteSpace($existingImageNamespace)) {
        $effectiveImageNamespace = $existingImageNamespace.Trim().Trim("/")
    }

    $existingPullPolicy = Get-EnvValue -Path $envFile -Name "SOLODAWN_IMAGE_PULL_POLICY"
    if ($existingPullPolicy -in @("always", "missing", "never")) {
        $effectiveImagePullPolicy = $existingPullPolicy
    }
}

Write-Info (T "INFO_MOUNT_SUMMARY")
Write-Host "  Host:      $composeHostWorkspaceRoot"
Write-Host "  Container: $workspaceMount"
Write-Host "  Allowed roots: $allowedRoots"
Write-Info (Tf "INFO_BUILD_NETWORK_PROFILE" @($effectiveBuildNetworkProfile))

Push-Location $repoRoot
try {
    Write-Info (T "INFO_VALIDATING")
    & docker compose -f $composeFile --env-file $envFile config -q
    if ($LASTEXITCODE -ne 0) {
        throw (T "ERR_COMPOSE_CONFIG")
    }
    Write-Ok (T "OK_COMPOSE_VALID")

    if (-not $SkipBuild) {
        $targetComposeImage = "solodawn-solodawn:latest"
        $usedPrebuilt = $false

        if ($preferPrebuiltImageEnabled -and $effectiveImagePullPolicy -ne "never") {
            $pullCandidates = Resolve-PrebuiltImageCandidates -Registry $effectiveImageRegistry -Namespace $effectiveImageNamespace -BuildNetworkProfile $effectiveBuildNetworkProfile

            $shouldTryPull = $false
            switch ($effectiveImagePullPolicy) {
                "always" { $shouldTryPull = $true }
                "missing" {
                    if (-not (Test-ImageExistsLocal -Image $targetComposeImage)) {
                        $shouldTryPull = $true
                    }
                }
            }

            if ($shouldTryPull) {
                $usedPrebuilt = Try-PullPrebuiltImage -Candidates $pullCandidates -TargetTag $targetComposeImage
            }
        }

        $shouldBuild = -not $usedPrebuilt

        if ($shouldBuild) {
            # Smart skip: detect existing image and offer to reuse it
            $existingImage = $null
            try {
                $inspectOutput = & docker images --format "{{.Repository}}:{{.Tag}} {{.CreatedSince}} {{.Size}}" 2>$null |
                    Where-Object { $_ -match "solodawn" } |
                    Select-Object -First 1
                if (-not [string]::IsNullOrWhiteSpace($inspectOutput)) {
                    $existingImage = $inspectOutput
                }
            } catch {
                # Ignore — proceed with build
            }

            if ($null -ne $existingImage -and -not $Force -and -not $NonInteractive) {
                Write-Info (Tf "INFO_EXISTING_IMAGE" @($existingImage))
                $shouldBuild = Read-YesNo (T "PROMPT_REBUILD_IMAGE") $false
            }
        }

        if ($shouldBuild) {
            Write-Info (T "INFO_BUILDING")
            $env:DOCKER_BUILDKIT = "1"
            $env:COMPOSE_DOCKER_CLI_BUILD = "1"
            Invoke-ComposeBuildWithRetry -ComposeFilePath $composeFile -EnvFilePath $envFile -ShouldPullBaseImages $PullBaseImages.IsPresent
            Write-Ok (T "OK_BUILD_DONE")
        }
        else {
            Write-Info (T "INFO_BUILD_SKIPPED")
        }
    }

    if ($resetDataVolume) {
        Write-Info (T "INFO_RESETTING_DATA")
        & docker compose --ansi never -f $composeFile --env-file $envFile down -v --remove-orphans
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_DOWN_FAILED")
        }
    }

    if (-not $SkipStart) {
        Write-Info (T "INFO_STARTING")
        & docker compose --ansi never -f $composeFile --env-file $envFile up -d --force-recreate --no-build
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_UP_FAILED")
        }
        Write-Ok (T "OK_STARTED")

        $readyUrl = "http://localhost:$Port/readyz"
        Write-Info (Tf "INFO_CHECKING_READY" @($readyUrl))
        if (Wait-Ready -ReadyUrl $readyUrl -MaxSeconds 180) {
            Write-Ok (T "OK_READY")
        }
        else {
            Write-Warn (Tf "WARN_READY_TIMEOUT" @($composeFile))
        }
    }
}
finally {
    Pop-Location
}

Write-Host ""
Write-Ok (T "OK_DONE")
Write-Host (Tf "OPEN_URL" @($Port))
Write-Host ""
Write-Host (T "USEFUL_COMMANDS")
Write-Host "  docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env ps"
Write-Host "  docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env logs -f"
Write-Host (T "UPDATE_COMMAND")
Write-Host "  powershell -ExecutionPolicy Bypass -File .\scripts\docker\update-docker.ps1 -PullLatest"
