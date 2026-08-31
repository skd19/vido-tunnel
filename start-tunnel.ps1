# ============================================
# VIDOVEO CLOUDFLARE TUNNEL MANAGER
# ============================================

$TunnelName = "vidoveo"

# ============================================
# FIND CLOUDFLARED
# ============================================

$CloudflaredCommand = Get-Command cloudflared.exe -ErrorAction SilentlyContinue

if (-not $CloudflaredCommand) {
    $CloudflaredCommand = Get-Command cloudflared -ErrorAction SilentlyContinue
}

if (-not $CloudflaredCommand) {
    Clear-Host

    Write-Host "=========================================" -ForegroundColor Red
    Write-Host "     CLOUDFLARED NOT FOUND" -ForegroundColor Red
    Write-Host "=========================================" -ForegroundColor Red
    Write-Host ""

    Write-Host "Make sure cloudflared is installed." -ForegroundColor Yellow
    Write-Host ""

    Start-Sleep -Seconds 5
    exit
}

$Cloudflared = $CloudflaredCommand.Source


# ============================================
# CHECK CLOUDFLARED
# ============================================

function Test-VidoveoTunnel {

    $Process = Get-CimInstance Win32_Process `
        -Filter "Name = 'cloudflared.exe'" `
        -ErrorAction SilentlyContinue

    foreach ($Item in $Process) {

        if ($Item.CommandLine -like "*tunnel run $TunnelName*") {
            return $true
        }
    }

    return $false
}


# ============================================
# START TUNNEL
# ============================================

function Start-VidoveoTunnel {

    if (Test-VidoveoTunnel) {
        return
    }

    try {

        Start-Process `
            -FilePath $Cloudflared `
            -ArgumentList "tunnel", "run", $TunnelName `
            -WindowStyle Hidden

    }
    catch {

        Write-Host ""
        Write-Host "Unable to start tunnel!" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red

        Start-Sleep -Seconds 3
    }
}


# ============================================
# STOP TUNNEL
# ============================================

function Stop-VidoveoTunnel {

    $Processes = Get-CimInstance Win32_Process `
        -Filter "Name = 'cloudflared.exe'" `
        -ErrorAction SilentlyContinue

    foreach ($Process in $Processes) {

        if ($Process.CommandLine -like "*tunnel run $TunnelName*") {

            try {
                Stop-Process -Id $Process.ProcessId -Force
            }
            catch {
                Write-Host ""
                Write-Host "Unable to stop tunnel." -ForegroundColor Red
                Start-Sleep -Seconds 2
            }
        }
    }
}


# ============================================
# DRAW MENU
# ============================================

function Show-Menu {

    Clear-Host

    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host "       VIDOVEO CLOUDFLARE TUNNEL" -ForegroundColor Cyan
    Write-Host "=========================================" -ForegroundColor Cyan
    Write-Host ""

    if (Test-VidoveoTunnel) {

        Write-Host "Tunnel Status: " -NoNewline
        Write-Host "[ RUNNING ]" -ForegroundColor Green

    }
    else {

        Write-Host "Tunnel Status: " -NoNewline
        Write-Host "[ STOPPED ]" -ForegroundColor Red
    }

    Write-Host ""
    Write-Host "-----------------------------------------" -ForegroundColor DarkGray
    Write-Host ""

    Write-Host " [1] " -ForegroundColor Yellow -NoNewline
    Write-Host "Start Tunnel"

    Write-Host " [2] " -ForegroundColor Cyan -NoNewline
    Write-Host "Stop Tunnel"

    Write-Host " [3] " -ForegroundColor White -NoNewline
    Write-Host "Refresh Status"

    Write-Host " [4] " -ForegroundColor White -NoNewline
    Write-Host "Exit"

    Write-Host ""
    Write-Host "-----------------------------------------" -ForegroundColor DarkGray
    Write-Host ""
}


# ============================================
# INITIAL MENU
# ============================================

Show-Menu

# Keep menu visible for 2 seconds
Write-Host "Checking tunnel in 2 seconds..." -ForegroundColor DarkGray

Start-Sleep -Seconds 2


# ============================================
# INITIAL AUTO START
# ============================================

if (-not (Test-VidoveoTunnel)) {

    Write-Host ""
    Write-Host "Tunnel is stopped." -ForegroundColor Yellow
    Write-Host "Starting Vidoveo tunnel..." -ForegroundColor Yellow

    Start-VidoveoTunnel

    # Give cloudflared time to launch
    Start-Sleep -Seconds 2
}


# ============================================
# MAIN MENU LOOP
# ============================================

while ($true) {

    Show-Menu

    Write-Host "Press 1-4..." -ForegroundColor DarkGray

    # Immediate key press
    $Key = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")


    switch ($Key.Character) {

        # ------------------------------------
        # START
        # ------------------------------------

        "1" {

            if (Test-VidoveoTunnel) {

                # Already running
                continue
            }

            Write-Host ""
            Write-Host "Starting Vidoveo tunnel..." -ForegroundColor Yellow

            Start-VidoveoTunnel

            Start-Sleep -Seconds 2
        }


        # ------------------------------------
        # STOP
        # ------------------------------------

        "2" {

            if (-not (Test-VidoveoTunnel)) {
                continue
            }

            Write-Host ""
            Write-Host "Stopping Vidoveo tunnel..." -ForegroundColor Yellow

            Stop-VidoveoTunnel

            Start-Sleep -Seconds 2
        }


        # ------------------------------------
        # REFRESH
        # ------------------------------------

        "3" {
            continue
        }


        # ------------------------------------
        # EXIT
        # ------------------------------------

        "4" {

            Clear-Host
            exit
        }
    }
}
