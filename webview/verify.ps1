# Verify without placing a binary or runtime artifacts in the checkout.
[CmdletBinding()]
param()

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$originalLocation = Get-Location
$verificationDir = Join-Path ([IO.Path]::GetTempPath()) ('tinymd-verify-' + [guid]::NewGuid().ToString('N'))
$transcribing = $false
$verificationExit = 0

function Invoke-Checked {
    param([string]$Command, [string[]]$Arguments)
    Write-Host ("Running: $Command " + ($Arguments -join ' '))
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$Command failed with exit code $LASTEXITCODE"
    }
}

try {
    New-Item -ItemType Directory -Path $verificationDir | Out-Null
    Write-Host "Verification artifacts: $verificationDir"
    Start-Transcript -Path (Join-Path $verificationDir 'verify.log') | Out-Null
    $transcribing = $true
    Get-Command node, go -ErrorAction Stop | Out-Null
    Set-Location -LiteralPath $PSScriptRoot

    Invoke-Checked -Command node -Arguments @('--check', 'app.js')
    Invoke-Checked -Command node -Arguments @('--check', 'selftest.js')
    Invoke-Checked -Command go -Arguments @('test', './...')
    $testExe = Join-Path $verificationDir 'tinymd-verify.exe'
    Invoke-Checked -Command go -Arguments @('build', '-ldflags=-s -w -H windowsgui', '-trimpath', '-o', $testExe, '.')

    Set-Location -LiteralPath $verificationDir
    $resultsFile = Join-Path $verificationDir 'selftest.json'
    Write-Host 'Running full WebView2 self-tests through the normal CLI.'
    & $testExe --selftest $resultsFile | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "Self-test process failed with exit code $LASTEXITCODE" }
    if (!(Test-Path -LiteralPath $resultsFile -PathType Leaf)) { throw 'Self-test results were not written.' }
    $result = Get-Content -Raw -LiteralPath $resultsFile | ConvertFrom-Json
    if ($result.PSObject.Properties.Name -contains 'error') { throw "Self-test error: $($result.error)" }
    $rows = @($result.results)
    $failedRows = @($rows | Where-Object { $_.ok -ne $true })
    Write-Host "WebView2: $($result.passed) passed, $($result.failed) failed, $(@($result.errors).Count) page errors."
    foreach ($row in $failedRows) { Write-Host "$($row.name): $($row.detail)" }
    if ($result.passed -isnot [int] -or $result.failed -isnot [int] -or
        $result.passed -le 0 -or $result.failed -ne 0 -or
        @($result.errors).Count -ne 0 -or $failedRows.Count -ne 0 -or
        $rows.Count -ne $result.passed) {
        throw 'Self-test results contain failures, page errors, or inconsistent counts.'
    }
    Write-Host "Verification passed. Evidence retained at $verificationDir"
} catch {
    $verificationExit = 1
    Write-Host "Verification failed: $($_.Exception.Message)"
    Write-Host "Evidence retained at $verificationDir"
} finally {
    Set-Location -LiteralPath $originalLocation.Path
    if ($transcribing) { Stop-Transcript | Out-Null }
}
exit $verificationExit
