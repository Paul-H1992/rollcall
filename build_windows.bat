@echo off
REM Windows build script
REM Usage: Double-click build_windows.bat or run from cmd

echo Building RollCall for Windows...

REM Check if pip is available
where pip >nul 2>&1
if %errorlevel% neq 0 (
    echo ERROR: pip not found. Please install Python from python.org
    pause
    exit /b 1
)

REM Install PyInstaller
echo Installing PyInstaller...
pip install pyinstaller

REM Install dependencies
echo Installing dependencies...
pip install -r requirements.txt

REM Create data directory
if not exist "data" mkdir data

REM Build
echo Building...
pyinstaller rollcall.spec --clean

echo.
echo Build complete!
echo Output: dist\rollcall\
echo.
echo To run:
echo   dist\rollcall\rollcall.exe
echo.
pause
