; ============================================================================
; GitCortex Windows Installer - Inno Setup Script (Lightweight)
; Bundles: server + tray only. Requires system Node.js, Git, npm.
; ============================================================================

#define MyAppName "GitCortex"
#define MyAppVersion "0.1.0"
#define MyAppPublisher "GitCortex"
#define MyAppURL "https://github.com/huanchong-99/GitCortex"
#define MyAppExeName "gitcortex-tray.exe"
#define DefaultPort "23456"

[Setup]
AppId={{7B8C4D2E-3F1A-4E5B-9C6D-8A7B3E2F1D4C}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}/issues
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
AllowNoIcons=yes
LicenseFile=..\LICENSE
OutputDir=output
OutputBaseFilename=GitCortex-Setup-v{#MyAppVersion}
Compression=lzma2/ultra64
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
SetupIconFile=assets\GitCortex.ico
UninstallDisplayIcon={app}\gitcortex-tray.exe
MinVersion=10.0
ShowLanguageDialog=auto

[Languages]
Name: "chinesesimplified"; MessagesFile: "compiler:Languages\ChineseSimplified.isl"
Name: "english"; MessagesFile: "compiler:Default.isl"

[Types]
Name: "full"; Description: "Full installation (recommended)"
Name: "compact"; Description: "Compact installation (server only)"
Name: "custom"; Description: "Custom installation"; Flags: iscustom

[Components]
Name: "server"; Description: "GitCortex Server"; Types: full compact custom; Flags: fixed
Name: "tray"; Description: "System Tray Helper"; Types: full custom

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional shortcuts:"; Components: tray
Name: "startupicon"; Description: "Start with Windows"; GroupDescription: "Startup:"; Components: tray
Name: "firewall"; Description: "Add Windows Firewall exception (port {#DefaultPort})"; GroupDescription: "Network:"

[Files]
; Core binaries
Source: "build\gitcortex-server.exe"; DestDir: "{app}"; Components: server; Flags: ignoreversion
Source: "build\gitcortex-tray.exe"; DestDir: "{app}"; Components: tray; Flags: ignoreversion

; Installer helper scripts
Source: "scripts\generate-key.ps1"; DestDir: "{app}\scripts"; Flags: ignoreversion
Source: "scripts\install-single-cli.ps1"; DestDir: "{app}\scripts"; Flags: ignoreversion
Source: "scripts\post-install-check.ps1"; DestDir: "{app}\scripts"; Flags: ignoreversion

; Environment setup scripts (dev tools + AI CLIs one-click installer)
Source: "..\scripts\setup-windows.cmd"; DestDir: "{app}\scripts"; Flags: ignoreversion
Source: "..\scripts\setup-windows.ps1"; DestDir: "{app}\scripts"; Flags: ignoreversion

; Assets
Source: "assets\GitCortex.ico"; DestDir: "{app}\assets"; Flags: ignoreversion

[Icons]
Name: "{group}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Components: tray
Name: "{group}\Open {#MyAppName} Web UI"; Filename: "http://127.0.0.1:{#DefaultPort}"
Name: "{group}\Setup Dev Environment"; Filename: "{app}\scripts\setup-windows.cmd"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"
Name: "{commondesktop}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: desktopicon; Components: tray
Name: "{commonstartup}\{#MyAppName}"; Filename: "{app}\{#MyAppExeName}"; Tasks: startupicon; Components: tray

[Run]
; Generate encryption key and .env
Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\scripts\generate-key.ps1"" -EnvFile ""{app}\.env"" -InstallDir ""{app}"""; StatusMsg: "Generating encryption keys..."; Flags: runhidden waituntilterminated

; Launch tray app after install
Filename: "{app}\{#MyAppExeName}"; Description: "Launch {#MyAppName}"; Components: tray; Flags: nowait postinstall skipifsilent

; Run environment setup (dev tools + AI CLIs)
Filename: "{app}\scripts\setup-windows.cmd"; Description: "Setup dev environment && AI CLIs (recommended for first install)"; Flags: postinstall skipifsilent unchecked nowait

; Run post-installation self-check
Filename: "powershell.exe"; Parameters: "-ExecutionPolicy Bypass -File ""{app}\scripts\post-install-check.ps1"" -SkipServerTest"; Description: "Run post-installation self-check"; Flags: postinstall skipifsilent unchecked nowait runascurrentuser

; Open web UI after install
Filename: "http://127.0.0.1:{#DefaultPort}"; Description: "Open {#MyAppName} in browser"; Flags: shellexec nowait postinstall skipifsilent unchecked

[UninstallRun]
; Stop server before uninstall
Filename: "taskkill"; Parameters: "/F /IM gitcortex-server.exe"; Flags: runhidden
Filename: "taskkill"; Parameters: "/F /IM gitcortex-tray.exe"; Flags: runhidden

[UninstallDelete]
Type: filesandordirs; Name: "{app}\scripts"
Type: filesandordirs; Name: "{app}\assets"
Type: files; Name: "{app}\.env"

[Code]
// Check if system locale is Chinese
function IsChineseLocale(): Boolean;
var
  UILang: Integer;
begin
  UILang := GetUILanguage();
  Result := (UILang = $0804) or (UILang = $0404) or
            (UILang and $00FF = $04);
end;

// Check if system Node.js >= 18 is available
function IsSystemNodeAvailable(): Boolean;
var
  ResultCode: Integer;
  Output: AnsiString;
begin
  Result := False;
  if Exec('cmd.exe', '/C node --version > "%TEMP%\gc_node_ver.txt" 2>&1', '', SW_HIDE, ewWaitUntilTerminated, ResultCode) then
  begin
    if ResultCode = 0 then
    begin
      if LoadStringFromFile(ExpandConstant('{tmp}\gc_node_ver.txt'), Output) then
      begin
        if (Pos('v18.', String(Output)) > 0) or (Pos('v19.', String(Output)) > 0) or
           (Pos('v20.', String(Output)) > 0) or (Pos('v21.', String(Output)) > 0) or
           (Pos('v22.', String(Output)) > 0) or (Pos('v23.', String(Output)) > 0) or
           (Pos('v24.', String(Output)) > 0) or (Pos('v25.', String(Output)) > 0) then
          Result := True;
      end;
    end;
  end;
end;

// Check if system Git is available
function IsSystemGitAvailable(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec('cmd.exe', '/C git --version', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
           and (ResultCode = 0);
end;

// Check if npm is available
function IsSystemNpmAvailable(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec('cmd.exe', '/C npm --version', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
           and (ResultCode = 0);
end;

// Check if gh CLI is available
function IsSystemGhAvailable(): Boolean;
var
  ResultCode: Integer;
begin
  Result := Exec('cmd.exe', '/C gh --version', '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
           and (ResultCode = 0);
end;

// Build prerequisite check message for finish page
function GetPrereqMessage(): String;
var
  Msg: String;
begin
  Msg := 'System Prerequisites:' + #13#10;

  if IsSystemNodeAvailable() then
    Msg := Msg + '  [OK]      Node.js' + #13#10
  else
    Msg := Msg + '  [MISSING] Node.js (>= 18) -- required for AI CLIs' + #13#10;

  if IsSystemGitAvailable() then
    Msg := Msg + '  [OK]      Git' + #13#10
  else
    Msg := Msg + '  [MISSING] Git -- required for version control' + #13#10;

  if IsSystemNpmAvailable() then
    Msg := Msg + '  [OK]      npm' + #13#10
  else
    Msg := Msg + '  [MISSING] npm -- required to install AI CLIs' + #13#10;

  if IsSystemGhAvailable() then
    Msg := Msg + '  [OK]      GitHub CLI (gh)' + #13#10
  else
    Msg := Msg + '  [MISSING] GitHub CLI (gh) -- optional, needed for Copilot' + #13#10;

  Msg := Msg + #13#10 + 'Install AI CLIs after setup via:' + #13#10;
  Msg := Msg + '  npm install -g @anthropic-ai/claude-code';

  Result := Msg;
end;

// Update finish page memo with prerequisites
function UpdateReadyMemo(Space, NewLine, MemoUserInfoInfo, MemoDirInfo, MemoTypeInfo,
  MemoComponentsInfo, MemoGroupInfo, MemoTasksInfo: String): String;
begin
  Result := '';
  if MemoComponentsInfo <> '' then
    Result := Result + MemoComponentsInfo + NewLine + NewLine;
  if MemoTasksInfo <> '' then
    Result := Result + MemoTasksInfo + NewLine + NewLine;
  Result := Result + GetPrereqMessage();
end;

// Add to user PATH (only {app} itself)
procedure ModifyPath();
var
  OldPath: String;
begin
  if RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OldPath) then
  begin
    if Pos(ExpandConstant('{app}'), OldPath) = 0 then
      RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path',
        ExpandConstant('{app}') + ';' + OldPath);
  end;
end;

// Remove from user PATH on uninstall
procedure RemoveFromPath();
var
  OldPath, AppDir: String;
begin
  if RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OldPath) then
  begin
    AppDir := ExpandConstant('{app}');
    StringChangeEx(OldPath, AppDir + ';', '', True);
    RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OldPath);
  end;
end;

// Add firewall rule
procedure AddFirewallRule();
var
  ResultCode: Integer;
begin
  Exec('netsh', ExpandConstant('advfirewall firewall add rule name="GitCortex Server" dir=in action=allow protocol=TCP localport={#DefaultPort} program="{app}\gitcortex-server.exe"'),
       '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

// Remove firewall rule on uninstall
procedure RemoveFirewallRule();
var
  ResultCode: Integer;
begin
  Exec('netsh', 'advfirewall firewall delete rule name="GitCortex Server"',
       '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    ModifyPath();
    if WizardIsTaskSelected('firewall') then
      AddFirewallRule();
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  if CurUninstallStep = usPostUninstall then
  begin
    RemoveFromPath();
    RemoveFirewallRule();
  end;
end;
