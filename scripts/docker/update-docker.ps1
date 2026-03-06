param(
    [switch]$PullLatest,
    [switch]$AllowDirty,
    [switch]$PullBaseImages,
    [switch]$SkipBuild,
    [switch]$SkipReadyCheck,
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
        TITLE = "=== GitCortex Docker 更新 ==="
        INFO_PULLING = "正在拉取最新代码..."
        INFO_VALIDATING = "正在校验 compose 配置..."
        INFO_BUILDING = "正在重建 Docker 镜像..."
        INFO_STARTING = "正在应用更新并重启容器..."
        INFO_CHECKING_READY = "正在检查服务就绪: {0}"
        OK_PULL_DONE = "代码更新完成。"
        OK_COMPOSE_VALID = "Compose 配置校验通过。"
        OK_BUILD_DONE = "镜像构建完成。"
        OK_STARTED = "容器更新完成。"
        OK_READY = "服务已就绪。"
        OK_DONE = "Docker 更新完成。"
        WARN_READY_TIMEOUT = "服务未在预期时间内就绪，请查看日志: docker compose -f {0} logs -f"
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
        TITLE = "=== GitCortex Docker Update ==="
        INFO_PULLING = "Pulling latest code..."
        INFO_VALIDATING = "Validating compose configuration..."
        INFO_BUILDING = "Rebuilding Docker image..."
        INFO_STARTING = "Applying update and restarting container..."
        INFO_CHECKING_READY = "Checking readiness: {0}"
        OK_PULL_DONE = "Source update completed."
        OK_COMPOSE_VALID = "Compose configuration is valid."
        OK_BUILD_DONE = "Build completed."
        OK_STARTED = "Container update completed."
        OK_READY = "Service is ready."
        OK_DONE = "Docker update completed."
        WARN_READY_TIMEOUT = "Service did not become ready in time. Check logs: docker compose -f {0} logs -f"
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

    Write-Info (T "INFO_VALIDATING")
    & docker compose -f $composeFilePath --env-file $envFilePath config -q
    if ($LASTEXITCODE -ne 0) {
        throw (T "ERR_COMPOSE_CONFIG")
    }
    Write-Ok (T "OK_COMPOSE_VALID")

    if (-not $SkipBuild) {
        Write-Info (T "INFO_BUILDING")
        $env:DOCKER_BUILDKIT = "1"
        $env:COMPOSE_DOCKER_CLI_BUILD = "1"

        $buildArgs = @(
            "compose",
            "--ansi", "never",
            "--progress", "plain",
            "-f", $composeFilePath,
            "--env-file", $envFilePath,
            "build"
        )
        if ($PullBaseImages) {
            $buildArgs += "--pull"
        }

        & docker @buildArgs
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_BUILD_FAILED")
        }
        Write-Ok (T "OK_BUILD_DONE")
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
