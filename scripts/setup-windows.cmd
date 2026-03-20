@echo off

echo ============================================
echo  GitCortex Windows Environment Setup
echo ============================================
echo.

:: Check that setup-windows.ps1 exists next to this .cmd file
if not exist "%~dp0setup-windows.ps1" (
    echo [FAIL] Cannot find setup-windows.ps1
    echo.
    echo This file must be in the same folder as setup-windows.cmd
    echo.
    echo Expected location: %~dp0setup-windows.ps1
    echo.
    pause
    exit /b 1
)

:: Run the PowerShell script with -File (not -Command)
:: Use full path to powershell.exe to avoid file-association issues.
:: The .ps1 script handles its own UAC elevation internally.
:: %~dp0 = directory of this .cmd file (always ends with \)
:: %* = forward all arguments to the .ps1 script
"%SystemRoot%\System32\WindowsPowerShell\v1.0\powershell.exe" -NoProfile -ExecutionPolicy Bypass -File "%~dp0setup-windows.ps1" %*

echo.
pause
