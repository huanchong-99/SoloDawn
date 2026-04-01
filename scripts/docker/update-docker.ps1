param(
    [switch]$PullLatest,
    [switch]$AllowDirty,
    [switch]$PullBaseImages,
    [switch]$SkipBuild,
    [switch]$SkipReadyCheck,
    [switch]$PreferPrebuiltImage,
    [string]$Port = "",
    [string]$ComposeFile = "",
    [string]$EnvFile = "",
    [ValidateSet("zh", "en")]
    [string]$Lang = "zh"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

try {
    $utf8 = [System.Text.UTF8Encoding]::new($false)
    $OutputEncoding = $utf8
    [Console]::OutputEncoding = $utf8
} catch {
    # Best effort only.
}

$script:Messages = @{
    zh = @{
        ERR_DOCKER_NOT_FOUND = "系统 PATH 中未找到 Docker。请先安装并启动 Docker。"
        ERR_GIT_NOT_FOUND = "启用了 -PullLatest，但系统 PATH 中未找到 Git。"
        ERR_COMPOSE_NOT_FOUND = "未找到 Compose 文件: {0}"
        ERR_ENV_NOT_FOUND = "未找到 Docker 环境文件: {0}"
        ERR_DIRTY_REPO = "检测到仓库存在未提交修改。请先提交/清理，或显式传入 -AllowDirty。"
        ERR_PULL_FAILED = "git pull --ff-only 失败，请先手动处理分支状态。"
        ERR_COMPOSE_CONFIG = "docker compose 配置校验失败。"
        ERR_BUILD_FAILED = "docker compose build 失败。"
        ERR_UP_FAILED = "docker compose up 失败。"
        TITLE = "=== SoloDawn Docker 更新 ==="
        INFO_PULLING = "正在拉取最新代码..."
        INFO_VALIDATING = "正在校验 compose 配置..."
        INFO_BUILDING = "正在重建 Docker 镜像..."
        INFO_BUILD_NETWORK_PROFILE = "构建网络配置: {0}"
        INFO_BUILD_WATCHDOG = "构建已超过 5 分钟，开始巡检是否卡住..."
        INFO_BUILD_POLL = "构建仍在进行，已耗时 {0}，距上次输出 {1} 秒。"
        INFO_BUILD_RETRY = "检测到可重试的构建失败，准备进行第 {0}/{1} 次重试..."
        INFO_PRUNING_BUILD_CACHE = "正在清理 BuildKit 执行缓存后重试..."
        INFO_STARTING = "正在应用更新并重启容器..."
        INFO_CHECKING_READY = "正在检查服务就绪: {0}"
        INFO_TRY_PULL_PREBUILT = "正在尝试拉取预构建镜像: {0}"
        INFO_PREBUILT_PULL_FAILED = "预构建镜像拉取失败，将回退到本地构建。"
        INFO_PREBUILT_PRESENT = "检测到本地已存在预构建镜像: {0}"
        INFO_PREBUILT_USED = "已使用预构建镜像，跳过本地构建。"
        OK_PULL_DONE = "代码更新完成。"
        OK_COMPOSE_VALID = "Compose 配置校验通过。"
        OK_BUILD_DONE = "镜像构建完成。"
        OK_STARTED = "容器更新完成。"
        OK_READY = "服务已就绪。"
        OK_DONE = "Docker 更新完成。"
        WARN_READY_TIMEOUT = "服务未在预期时间内就绪，请查看日志: docker compose -f {0} logs -f"
        WARN_BUILD_STALLED = "构建连续 {0} 秒无输出，判定为卡住。"
        WARN_BUILD_LOG_TAIL = "最后 60 行构建日志："
        USEFUL_COMMANDS = "常用命令："
        OPEN_URL = "访问地址: http://localhost:{0}"
    }
    en = @{
        ERR_DOCKER_NOT_FOUND = "Docker is not available in PATH. Please install and start Docker first."
        ERR_GIT_NOT_FOUND = "Git is required when -PullLatest is used."
        ERR_COMPOSE_NOT_FOUND = "Compose file not found: {0}"
        ERR_ENV_NOT_FOUND = "Docker env file not found: {0}"
        ERR_DIRTY_REPO = "The repository has uncommitted changes. Commit/stash them first or pass -AllowDirty."
        ERR_PULL_FAILED = "git pull --ff-only failed. Resolve repository state manually first."
        ERR_COMPOSE_CONFIG = "docker compose config validation failed."
        ERR_BUILD_FAILED = "docker compose build failed."
        ERR_UP_FAILED = "docker compose up failed."
        TITLE = "=== SoloDawn Docker Update ==="
        INFO_PULLING = "Pulling latest code..."
        INFO_VALIDATING = "Validating compose configuration..."
        INFO_BUILDING = "Rebuilding Docker image..."
        INFO_BUILD_NETWORK_PROFILE = "Build network profile: {0}"
        INFO_BUILD_WATCHDOG = "Build has run for over 5 minutes. Starting stall watchdog..."
        INFO_BUILD_POLL = "Build still running. Elapsed {0}, last output {1}s ago."
        INFO_BUILD_RETRY = "Detected a retryable build failure. Starting retry {0}/{1}..."
        INFO_PRUNING_BUILD_CACHE = "Pruning BuildKit exec cache before retry..."
        INFO_STARTING = "Applying update and restarting container..."
        INFO_CHECKING_READY = "Checking readiness: {0}"
        INFO_TRY_PULL_PREBUILT = "Trying to pull prebuilt image: {0}"
        INFO_PREBUILT_PULL_FAILED = "Failed to pull prebuilt image. Falling back to local build."
        INFO_PREBUILT_PRESENT = "Prebuilt image already exists locally: {0}"
        INFO_PREBUILT_USED = "Using prebuilt image, local build skipped."
        OK_PULL_DONE = "Source update completed."
        OK_COMPOSE_VALID = "Compose configuration is valid."
        OK_BUILD_DONE = "Build completed."
        OK_STARTED = "Container update completed."
        OK_READY = "Service is ready."
        OK_DONE = "Docker update completed."
        WARN_READY_TIMEOUT = "Service did not become ready in time. Check logs: docker compose -f {0} logs -f"
        WARN_BUILD_STALLED = "Build produced no output for {0}s and is treated as stalled."
        WARN_BUILD_LOG_TAIL = "Last 60 build log lines:"
        USEFUL_COMMANDS = "Useful commands:"
        OPEN_URL = "Open: http://localhost:{0}"
    }
}

function T {
    param([string]$Key)
    $langTable = $script:Messages[$Lang]
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
        "error pulling image configuration"
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
            ExitCode = if ($stalled) { -1 } else { $process.ExitCode }
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

        if (-not $result.Stalled -and $result.ExitCode -eq 0) {
            return
        }

        $retryable = Test-RetryableBuildFailure -Output $result.Output -Stalled $result.Stalled
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

function Wait-Ready {
    param(
        [string]$ReadyUrl,
        [int]$MaxSeconds = 180
    )

    $start = Get-Date
    do {
        try {
            $response = Invoke-WebRequest -Uri $ReadyUrl -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -eq 200) {
                return $true
            }
        } catch {
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

    & docker image inspect $Image *> $null
    return ($LASTEXITCODE -eq 0)
}

function Try-PullPrebuiltImage {
    param(
        [string[]]$Candidates,
        [string]$TargetTag
    )

    # ── Diagnostic: show ALL solodawn-related images currently on disk ──
    Write-Host ""
    Write-Host "╔══════════════════════════════════════════════════════╗" -ForegroundColor Cyan
    Write-Host "║  Docker Local Image Scan (solodawn)                 ║" -ForegroundColor Cyan
    Write-Host "╚══════════════════════════════════════════════════════╝" -ForegroundColor Cyan
    try {
        $localImages = & docker images --format "{{.Repository}}:{{.Tag}}  ID={{.ID}}  Created={{.CreatedSince}}  Size={{.Size}}" 2>$null |
            Where-Object { $_ -match "solodawn" }
        if ($localImages) {
            foreach ($img in $localImages) {
                Write-Host "  [LOCAL] $img" -ForegroundColor Yellow
            }
        } else {
            Write-Host "  [LOCAL] (none — no solodawn images found locally)" -ForegroundColor Green
        }
    } catch {
        Write-Host "  [LOCAL] (scan failed: $_)" -ForegroundColor Red
    }
    Write-Host "  [TARGET] Compose expects: $TargetTag" -ForegroundColor Cyan
    $targetExists = Test-ImageExistsLocal -Image $TargetTag
    Write-Host "  [TARGET] $TargetTag exists locally: $targetExists" -ForegroundColor $(if ($targetExists) { "Yellow" } else { "Green" })
    Write-Host ""

    foreach ($candidate in $Candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        $candidateLocal = Test-ImageExistsLocal -Image $candidate
        Write-Host "  [CHECK] $candidate exists locally: $candidateLocal" -ForegroundColor $(if ($candidateLocal) { "Yellow" } else { "DarkGray" })

        # Always try pull first to get the latest image from remote
        Write-Info (Tf "INFO_TRY_PULL_PREBUILT" @($candidate))
        Write-Host "  [PULL]  Running: docker pull $candidate" -ForegroundColor Cyan
        $pullOk = $false
        try {
            & docker pull $candidate 2>&1
            if ($LASTEXITCODE -eq 0) {
                $pullOk = $true
                Write-Host "  [PULL]  SUCCESS — pulled from remote" -ForegroundColor Green
            } else {
                Write-Host "  [PULL]  FAILED — docker pull exited with code $LASTEXITCODE" -ForegroundColor Red
            }
        } catch {
            Write-Host "  [PULL]  EXCEPTION — $_" -ForegroundColor Red
        }

        if (-not $pullOk) {
            if ($candidateLocal) {
                Write-Host "  [FALLBACK] Pull failed, but local copy exists — using local" -ForegroundColor Yellow
            } else {
                Write-Host "  [SKIP] Pull failed and no local copy — trying next candidate" -ForegroundColor Red
                continue
            }
        }

        Write-Host "  [TAG]   Tagging $candidate → $TargetTag" -ForegroundColor Cyan
        try {
            & docker image tag $candidate $TargetTag 2>&1 | Out-Null
            if ($LASTEXITCODE -eq 0) {
                Write-Host "  [TAG]   SUCCESS" -ForegroundColor Green
                Write-Info (T "INFO_PREBUILT_USED")
                Write-Host ""
                return $true
            } else {
                Write-Host "  [TAG]   FAILED — exit code $LASTEXITCODE" -ForegroundColor Red
            }
        } catch {
            Write-Host "  [TAG]   EXCEPTION — $_" -ForegroundColor Red
        }
    }

    Write-Host ""
    Write-Warn (T "INFO_PREBUILT_PULL_FAILED")
    return $false
}

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$composeFilePath = if ([string]::IsNullOrWhiteSpace($ComposeFile)) {
    Join-Path $repoRoot "docker\compose\docker-compose.yml"
} else {
    (Resolve-Path -LiteralPath $ComposeFile).Path
}
$envFilePath = if ([string]::IsNullOrWhiteSpace($EnvFile)) {
    Join-Path $repoRoot "docker\compose\.env"
} else {
    (Resolve-Path -LiteralPath $EnvFile).Path
}

if (-not (Test-Path -LiteralPath $composeFilePath)) {
    throw (Tf "ERR_COMPOSE_NOT_FOUND" @($composeFilePath))
}

if (-not (Test-Path -LiteralPath $envFilePath)) {
    throw (Tf "ERR_ENV_NOT_FOUND" @($envFilePath))
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw (T "ERR_DOCKER_NOT_FOUND")
}

Write-Host ""
Write-Host (T "TITLE") -ForegroundColor Magenta
Write-Host ""

Push-Location $repoRoot
try {
    if ($PullLatest) {
        if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
            throw (T "ERR_GIT_NOT_FOUND")
        }

        $gitStatus = & git -C $repoRoot status --porcelain
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_PULL_FAILED")
        }

        if (-not $AllowDirty -and -not [string]::IsNullOrWhiteSpace(($gitStatus | Out-String))) {
            throw (T "ERR_DIRTY_REPO")
        }

        Write-Info (T "INFO_PULLING")
        & git -C $repoRoot pull --ff-only
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_PULL_FAILED")
        }
        Write-Ok (T "OK_PULL_DONE")
    }

    $resolvedPort = if ([string]::IsNullOrWhiteSpace($Port)) {
        Get-EnvValue -Path $envFilePath -Name "PORT"
    } else {
        $Port
    }

    if ([string]::IsNullOrWhiteSpace($resolvedPort)) {
        $resolvedPort = "23456"
    }

    $buildNetworkProfile = Get-EnvValue -Path $envFilePath -Name "SOLODAWN_BUILD_NETWORK_PROFILE"
    if ([string]::IsNullOrWhiteSpace($buildNetworkProfile)) {
        $buildNetworkProfile = "official"
    }

    $imageRegistry = Get-EnvValue -Path $envFilePath -Name "SOLODAWN_IMAGE_REGISTRY"
    if ([string]::IsNullOrWhiteSpace($imageRegistry)) {
        $imageRegistry = "ghcr.io"
    }
    else {
        $imageRegistry = $imageRegistry.Trim().TrimEnd("/")
    }

    $imageNamespace = Get-EnvValue -Path $envFilePath -Name "SOLODAWN_IMAGE_NAMESPACE"
    if (-not [string]::IsNullOrWhiteSpace($imageNamespace)) {
        $imageNamespace = $imageNamespace.Trim().Trim("/")
    }

    $imagePullPolicy = Get-EnvValue -Path $envFilePath -Name "SOLODAWN_IMAGE_PULL_POLICY"
    if (-not ($imagePullPolicy -in @("always", "missing", "never"))) {
        $imagePullPolicy = "always"
    }

    Write-Info (Tf "INFO_BUILD_NETWORK_PROFILE" @($buildNetworkProfile))

    Write-Info (T "INFO_VALIDATING")
    & docker compose -f $composeFilePath --env-file $envFilePath config -q
    if ($LASTEXITCODE -ne 0) {
        throw (T "ERR_COMPOSE_CONFIG")
    }
    Write-Ok (T "OK_COMPOSE_VALID")

    if (-not $SkipBuild) {
        $targetComposeImage = "compose-solodawn:latest"
        $usedPrebuilt = $false

        if ($PreferPrebuiltImage -and $imagePullPolicy -ne "never") {
            $pullCandidates = Resolve-PrebuiltImageCandidates -Registry $imageRegistry -Namespace $imageNamespace -BuildNetworkProfile $buildNetworkProfile

            $shouldTryPull = $false
            switch ($imagePullPolicy) {
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

        if (-not $usedPrebuilt) {
            Write-Info (T "INFO_BUILDING")
            $env:DOCKER_BUILDKIT = "1"
            $env:COMPOSE_DOCKER_CLI_BUILD = "1"
            Invoke-ComposeBuildWithRetry -ComposeFilePath $composeFilePath -EnvFilePath $envFilePath -ShouldPullBaseImages $PullBaseImages.IsPresent
            Write-Ok (T "OK_BUILD_DONE")
        }
    }

    Write-Info (T "INFO_STARTING")
    & docker compose --ansi never -f $composeFilePath --env-file $envFilePath up -d --force-recreate --remove-orphans --no-build
    if ($LASTEXITCODE -ne 0) {
        throw (T "ERR_UP_FAILED")
    }
    Write-Ok (T "OK_STARTED")

    if (-not $SkipReadyCheck) {
        $readyUrl = "http://localhost:$resolvedPort/readyz"
        Write-Info (Tf "INFO_CHECKING_READY" @($readyUrl))
        if (Wait-Ready -ReadyUrl $readyUrl) {
            Write-Ok (T "OK_READY")
        } else {
            Write-Warn (Tf "WARN_READY_TIMEOUT" @($composeFilePath))
        }
    }

    Write-Host ""
    Write-Ok (T "OK_DONE")
    Write-Host (Tf "OPEN_URL" @($resolvedPort))
    Write-Host ""
    Write-Host (T "USEFUL_COMMANDS")
    Write-Host "  docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env ps"
    Write-Host "  docker compose -f docker/compose/docker-compose.yml --env-file docker/compose/.env logs -f"
} finally {
    Pop-Location
}
