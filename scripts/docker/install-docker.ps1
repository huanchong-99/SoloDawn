param(
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
    [switch]$EnableAutoSetupProjects,
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
        ERR_KEY_LEN = "GITCORTEX_ENCRYPTION_KEY 必须恰好 32 个字符。"
        ERR_ENV_EXISTS = ".env 已存在于 {0}。如需覆盖，请加 -Force。"
        ERR_INSTALL_CANCELLED = "用户取消安装。"
        ERR_COMPOSE_CONFIG = "docker compose 配置校验失败。"
        ERR_BUILD_FAILED = "docker compose build 失败。"
        ERR_UP_FAILED = "docker compose up 失败。"
        ERR_DOWN_FAILED = "docker compose down -v 失败。"

        TITLE = "=== GitCortex Docker 一键安装 ==="
        ISOLATION_TITLE = "隔离模型："
        ISOLATION_1 = "1) 容器始终可访问自身文件系统。"
        ISOLATION_2 = "2) 容器可访问挂载到 {0} 的主机目录。"
        ISOLATION_3 = "3) 默认不会访问你的整块磁盘。"

        PROMPT_HOST_WORKSPACE = "要挂载到容器 {0} 的主机目录"
        PROMPT_PORT = "GitCortex 主机端口"
        PROMPT_RUST_LOG = "RUST_LOG 日志级别"
        PROMPT_INSTALL_AI_CLIS = "构建镜像时安装 AI CLI（更慢，但更省事）"
        PROMPT_AUTO_SETUP_PROJECTS = "首次启动自动创建项目（最多 3 个）"
        PROMPT_RUN_BUILD = "现在执行 docker compose build"
        PROMPT_RUN_UP = "现在执行 docker compose up -d"
        PROMPT_RESET_DATA_VOLUME = "启动前清理旧容器和数据卷（会删除已有项目数据）"
        PROMPT_AUTO_KEY = "自动生成 32 位加密密钥"
        PROMPT_INPUT_KEY = "输入 GITCORTEX_ENCRYPTION_KEY（必须 32 个字符）"
        PROMPT_SET_API_TOKEN = "是否配置 Docker API Bearer Token"
        PROMPT_SET_ANTHROPIC = "现在设置 ANTHROPIC_API_KEY"
        PROMPT_SET_OPENAI = "现在设置 OPENAI_API_KEY"
        PROMPT_SET_GOOGLE = "现在设置 GOOGLE_API_KEY"
        PROMPT_CREATE_MISSING_PATH = "目录不存在，是否立即创建"
        PROMPT_OVERWRITE_ENV = ".env 已存在，是否覆盖"

        INFO_KEY_GENERATED = "已生成加密密钥。"
        INFO_KEY_GENERATED_NON_INTERACTIVE = "非交互模式：已自动生成 32 位加密密钥。"
        INFO_PATH_CREATED = "已创建目录: {0}"
        INFO_MOUNT_SUMMARY = "挂载摘要："
        INFO_VALIDATING = "正在校验 compose 配置..."
        INFO_BUILDING = "正在构建 Docker 镜像..."
        INFO_STARTING = "正在启动容器..."
        INFO_RESETTING_DATA = "正在清理旧容器和数据卷..."
        INFO_CHECKING_READY = "正在检查服务就绪: {0}"
        INFO_ENV_REUSED = "保留已有配置文件: {0}"

        OK_ENV_WRITTEN = "已写入配置文件: {0}"
        OK_COMPOSE_VALID = "Compose 配置校验通过。"
        OK_BUILD_DONE = "镜像构建完成。"
        OK_STARTED = "容器已启动。"
        OK_READY = "服务已就绪。"
        OK_DONE = "安装完成。"

        WARN_READY_TIMEOUT = "服务未在预期时间内就绪，请查看日志: docker compose -f {0} logs -f"
        WARN_KEY_LEN = "密钥长度必须恰好为 32。"

        OPEN_URL = "访问地址: http://localhost:{0}"
        USEFUL_COMMANDS = "常用命令："
    }
    en = @{
        YESNO_INVALID = "Please answer y or n."
        LANG_CHOICE_TITLE = "Choose language / 请选择语言:"
        LANG_CHOICE_PROMPT = "Enter 1/2 (default 2)"

        ERR_COMPOSE_NOT_FOUND = "Compose file not found: {0}"
        ERR_DOCKER_NOT_FOUND = "Docker is not available in PATH. Please install and start Docker Desktop first."
        ERR_HOST_PATH_MISSING = "Host workspace path does not exist: {0}"
        ERR_HOST_PATH_REQUIRED = "Cannot continue without host workspace path."
        ERR_KEY_LEN = "GITCORTEX_ENCRYPTION_KEY must be exactly 32 chars."
        ERR_ENV_EXISTS = ".env already exists at {0}. Re-run with -Force to overwrite."
        ERR_INSTALL_CANCELLED = "Installation cancelled by user."
        ERR_COMPOSE_CONFIG = "docker compose config validation failed."
        ERR_BUILD_FAILED = "docker compose build failed."
        ERR_UP_FAILED = "docker compose up failed."
        ERR_DOWN_FAILED = "docker compose down -v failed."

        TITLE = "=== GitCortex Docker One-Click Installer ==="
        ISOLATION_TITLE = "Isolation model:"
        ISOLATION_1 = "1) Container can always access its own filesystem."
        ISOLATION_2 = "2) Container can also access host path mounted to {0}."
        ISOLATION_3 = "3) By default, it cannot access your full disk."

        PROMPT_HOST_WORKSPACE = "Host folder to mount into container {0}"
        PROMPT_PORT = "Host port for GitCortex"
        PROMPT_RUST_LOG = "RUST_LOG level"
        PROMPT_INSTALL_AI_CLIS = "Install AI CLIs during image build (slower but turnkey)"
        PROMPT_AUTO_SETUP_PROJECTS = "Auto-create starter projects on first launch (up to 3)"
        PROMPT_RUN_BUILD = "Run docker compose build now"
        PROMPT_RUN_UP = "Run docker compose up -d now"
        PROMPT_RESET_DATA_VOLUME = "Clean existing containers and data volume first (deletes existing projects)"
        PROMPT_AUTO_KEY = "Auto-generate a 32-char encryption key"
        PROMPT_INPUT_KEY = "Enter GITCORTEX_ENCRYPTION_KEY (exactly 32 chars)"
        PROMPT_SET_API_TOKEN = "Configure Docker API Bearer token"
        PROMPT_SET_ANTHROPIC = "Set ANTHROPIC_API_KEY now"
        PROMPT_SET_OPENAI = "Set OPENAI_API_KEY now"
        PROMPT_SET_GOOGLE = "Set GOOGLE_API_KEY now"
        PROMPT_CREATE_MISSING_PATH = "Path does not exist. Create it now"
        PROMPT_OVERWRITE_ENV = ".env already exists. Overwrite it"

        INFO_KEY_GENERATED = "Encryption key generated."
        INFO_KEY_GENERATED_NON_INTERACTIVE = "Non-interactive mode: generated 32-char encryption key."
        INFO_PATH_CREATED = "Created: {0}"
        INFO_MOUNT_SUMMARY = "Mount summary:"
        INFO_VALIDATING = "Validating compose file..."
        INFO_BUILDING = "Building Docker image..."
        INFO_STARTING = "Starting containers..."
        INFO_RESETTING_DATA = "Removing existing containers and data volume..."
        INFO_CHECKING_READY = "Checking readiness: {0}"
        INFO_ENV_REUSED = "Keeping existing config file: {0}"

        OK_ENV_WRITTEN = "Wrote config file: {0}"
        OK_COMPOSE_VALID = "Compose configuration is valid."
        OK_BUILD_DONE = "Build completed."
        OK_STARTED = "Container started."
        OK_READY = "Service is ready."
        OK_DONE = "Done."

        WARN_READY_TIMEOUT = "Service did not become ready in time. Check: docker compose -f {0} logs -f"
        WARN_KEY_LEN = "Key length must be exactly 32."

        OPEN_URL = "Open: http://localhost:{0}"
        USEFUL_COMMANDS = "Useful commands:"
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

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot = (Resolve-Path (Join-Path $scriptDir "..\..")).Path
$composeDir = Join-Path $repoRoot "docker\compose"
$composeFile = Join-Path $composeDir "docker-compose.yml"
$envFile = Join-Path $composeDir ".env"
$workspaceMount = "/workspace"
$dataRoot = "/var/lib/gitcortex"

Select-Language

if (-not (Test-Path -LiteralPath $composeFile)) {
    throw (Tf "ERR_COMPOSE_NOT_FOUND" @($composeFile))
}

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    throw (T "ERR_DOCKER_NOT_FOUND")
}

Write-Host ""
Write-Host (T "TITLE") -ForegroundColor Magenta
Write-Host ""
Write-Host (T "ISOLATION_TITLE") -ForegroundColor White
Write-Host (T "ISOLATION_1")
Write-Host (Tf "ISOLATION_2" @($workspaceMount))
Write-Host (T "ISOLATION_3")
Write-Host ""

if ([string]::IsNullOrWhiteSpace($HostWorkspaceRoot)) {
    $HostWorkspaceRoot = $repoRoot
}

$autoSetupProjectsEnabled = $EnableAutoSetupProjects.IsPresent
$resetDataVolume = $ResetDataVolume.IsPresent

if (-not $NonInteractive) {
    $HostWorkspaceRoot = Read-Default (Tf "PROMPT_HOST_WORKSPACE" @($workspaceMount)) $HostWorkspaceRoot
    $Port = Read-Default (T "PROMPT_PORT") $Port
    $RustLog = Read-Default (T "PROMPT_RUST_LOG") $RustLog
    $InstallAiClis = Read-YesNo (T "PROMPT_INSTALL_AI_CLIS") $InstallAiClis.IsPresent
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
            $DockerApiToken = Read-Host "GITCORTEX_DOCKER_API_TOKEN"
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
GITCORTEX_ENCRYPTION_KEY=$EncryptionKey
GITCORTEX_DOCKER_API_TOKEN=$DockerApiToken
ANTHROPIC_API_KEY=$AnthropicApiKey
OPENAI_API_KEY=$OpenAiApiKey
GOOGLE_API_KEY=$GoogleApiKey
PORT=$Port
RUST_LOG=$RustLog
HOST_WORKSPACE_ROOT=$composeHostWorkspaceRoot
GITCORTEX_WORKSPACE_ROOT=$workspaceMount
GITCORTEX_ALLOWED_ROOTS=$allowedRoots
INSTALL_AI_CLIS=$installAiClisValue
GITCORTEX_AUTO_SETUP_PROJECTS=$autoSetupProjectsValue
"@

[System.IO.File]::WriteAllText($envFile, $envContent, [System.Text.UTF8Encoding]::new($false))
Write-Ok (Tf "OK_ENV_WRITTEN" @($envFile))
}

Write-Info (T "INFO_MOUNT_SUMMARY")
Write-Host "  Host:      $composeHostWorkspaceRoot"
Write-Host "  Container: $workspaceMount"
Write-Host "  Allowed roots: $allowedRoots"

Push-Location $repoRoot
try {
    Write-Info (T "INFO_VALIDATING")
    & docker compose -f $composeFile --env-file $envFile config -q
    if ($LASTEXITCODE -ne 0) {
        throw (T "ERR_COMPOSE_CONFIG")
    }
    Write-Ok (T "OK_COMPOSE_VALID")

    if (-not $SkipBuild) {
        Write-Info (T "INFO_BUILDING")
        & docker compose --ansi never --progress plain -f $composeFile --env-file $envFile build
        if ($LASTEXITCODE -ne 0) {
            throw (T "ERR_BUILD_FAILED")
        }
        Write-Ok (T "OK_BUILD_DONE")
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
        & docker compose --ansi never -f $composeFile --env-file $envFile up -d --force-recreate
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
