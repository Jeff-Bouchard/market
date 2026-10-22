param([switch]$NoBrowser)

$ErrorActionPreference = 'Stop'
$localUrl = 'http://127.0.0.1:8787'
$localExecutable = Join-Path $PSScriptRoot 'ness-unified.exe'
$localConfigPath = Join-Path $PSScriptRoot 'preview.config.json'

function Get-LocalStatus {
    try { return Invoke-RestMethod -Uri "$localUrl/api/faucet" -TimeoutSec 2 }
    catch { return $null }
}

try {
    $localStatus = Get-LocalStatus
    if (-not $localStatus) {
        if (-not (Test-Path -LiteralPath $localExecutable)) {
            throw 'ness-unified.exe is missing. Build it with: go build -buildvcs=false -o ness-unified.exe .'
        }
        if (-not (Test-Path -LiteralPath $localConfigPath)) {
            $localConfig = Get-Content -LiteralPath (Join-Path $PSScriptRoot 'config.example.json') -Raw | ConvertFrom-Json
            $localConfig.listen = '127.0.0.1:8787'
            $localConfig.data_dir = 'preview-data'
            $localConfig.secure_cookies = $false
            $localConfig.public_origin = ''
            foreach ($key in @('ness_url', 'ness_wallet', 'ness_password', 'ness_api_user', 'ness_api_password', 'emc_url', 'emc_user', 'emc_password', 'exchange_url')) {
                $localConfig.$key = ''
            }
            $localConfig.emc_wallet = $null
            [System.IO.File]::WriteAllText($localConfigPath, ($localConfig | ConvertTo-Json -Depth 8))
        }
        $localConfig = Get-Content -LiteralPath $localConfigPath -Raw | ConvertFrom-Json
        if ($localConfig.listen -ne '127.0.0.1:8787' -or $localConfig.ness_url -or $localConfig.emc_url -or $localConfig.exchange_url) {
            throw 'OPEN-LOCAL uses the local preview only. Use START-WINDOWS for your configured service.'
        }
        $existingProcess = Get-CimInstance Win32_Process -Filter "name = 'ness-unified.exe'" |
            Where-Object { $_.ExecutablePath -eq $localExecutable }
        if (-not $existingProcess) {
            $null = Start-Process -FilePath $localExecutable -ArgumentList 'preview.config.json' -WorkingDirectory $PSScriptRoot -WindowStyle Hidden -PassThru `
                -RedirectStandardOutput (Join-Path $PSScriptRoot 'preview.stdout.log') `
                -RedirectStandardError (Join-Path $PSScriptRoot 'preview.stderr.log')
        }
        for ($attempt = 0; $attempt -lt 15 -and -not $localStatus; $attempt++) {
            Start-Sleep -Milliseconds 300
            $localStatus = Get-LocalStatus
        }
    }
    if (-not $localStatus -or $localStatus.wait_seconds -ne 8 -or -not $localStatus.csrf) {
        throw 'The local app did not start. See preview.stderr.log in this folder. Your saved data has been left in place.'
    }
    Write-Host "PrivateNess is ready at $localUrl"
    if (-not $NoBrowser) { Start-Process "$localUrl/#faucet" }
} catch {
    Write-Host $_.Exception.Message -ForegroundColor Red
    exit 1
}
