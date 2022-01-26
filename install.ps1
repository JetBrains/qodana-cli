#!/usr/bin/env pwsh
# https://deno.land/x/install@v0.1.4/install.ps1
# Copyright 2018 the Deno authors. All rights reserved. MIT License.
# TODO: make the script more fancy

$ErrorActionPreference = 'Stop'
$inputRepo = "JetBrains/qodana-cli"
$inputVersion = if ($v) {
  "${v}"
} else {
  "0.6.1"
}
$inputExe = "qodana"

$githubUrl = if ($github) {
  "${github}"
} elseif ($g) {
  "${g}"
} else {
  "https://github.com"
}

$arr = $inputRepo.Split('/')
$owner = $arr.Get(0)
$repoName = $arr.Get(1)
$exeName = "${inputExe}"

if ($exeName -eq "") {
  $exeName = "${repoName}"
}

if ($inputVersion) {
  $version = "${inputVersion}"
}

if ([Environment]::Is64BitProcess) {
  $arch = "x86_64"
} else {
  $arch = "arm64"
}

$BinDir = "$Home\bin"
$downloadedTagGz = "$BinDir\${exeName}.tar.gz"
$downloadedExe = "$BinDir\${exeName}.exe"
$Target = "windows_$arch"

# GitHub requires TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$ResourceUri = "${githubUrl}/${owner}/${repoName}/releases/download/v${Version}/${exeName}_${Version}_${Target}.tar.gz"

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

Invoke-WebRequest $ResourceUri -OutFile $downloadedTagGz -UseBasicParsing -ErrorAction Stop

function Check-Command {
  param($Command)
  $found = $false
  try
  {
      $Command | Out-Null
      $found = $true
  }
  catch [System.Management.Automation.CommandNotFoundException]
  {
      $found = $false
  }

  $found
}

if (Check-Command -Command tar) {
  Invoke-Expression "tar -xvzf $downloadedTagGz -C $BinDir"
} else {
  function Expand-Tar($tarFile, $dest) {

      if (-not (Get-Command Expand-7Zip -ErrorAction Ignore)) {
          Install-Package -Scope CurrentUser -Force 7Zip4PowerShell > $null
      }

      Expand-7Zip $tarFile $dest
  }

  Expand-Tar $downloadedTagGz $BinDir
}

Remove-Item $downloadedTagGz

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Write-Output "${exeName} was installed successfully to $downloadedExe"
Write-Output "Run '${exeName} --help' to get started"
