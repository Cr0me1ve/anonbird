param(
  [Parameter(Mandatory = $true)]
  [string[]] $Url,

  [Parameter(Mandatory = $true)]
  [string] $Destination,

  [Parameter(Mandatory = $true)]
  [ValidatePattern('^[a-fA-F0-9]{64}$')]
  [string] $Sha256,

  [int] $Attempts = 6,
  [int] $RetryIntervalSeconds = 15,
  [int] $TimeoutSeconds = 120
)

$ErrorActionPreference = 'Stop'

$expected = $Sha256.ToLowerInvariant()
$destDir = Split-Path -Parent $Destination
if ($destDir) {
  New-Item -ItemType Directory -Force -Path $destDir | Out-Null
}

$lastError = $null
$candidateUrls = @($Url | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })

foreach ($candidate in $candidateUrls) {
  for ($attempt = 1; $attempt -le $Attempts; $attempt++) {
    try {
      Remove-Item -Force -ErrorAction SilentlyContinue $Destination
      Write-Host "Downloading $candidate (attempt ${attempt}/${Attempts})"
      Invoke-WebRequest -Uri $candidate -OutFile $Destination -TimeoutSec $TimeoutSeconds

      $actual = (Get-FileHash -Path $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
      if ($actual -ne $expected) {
        throw "Checksum mismatch for $candidate. expected=$expected actual=$actual"
      }

      Write-Host "Downloaded $Destination with sha256=$actual"
      exit 0
    } catch {
      $lastError = $_.Exception.Message
      Write-Warning "Download failed from $candidate on attempt ${attempt}/${Attempts}: $lastError"
      if ($attempt -lt $Attempts) {
        Start-Sleep -Seconds $RetryIntervalSeconds
      }
    }
  }
}

$tried = $candidateUrls -join ', '
throw "Failed to download $Destination after trying URLs: $tried. Last error: $lastError"
