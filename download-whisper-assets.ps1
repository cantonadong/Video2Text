$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$dir = "D:\Models\asr\whisper.cpp"
New-Item -ItemType Directory -Force -Path $dir | Out-Null

$zip = Join-Path $dir "whisper-bin-x64-v1.8.6.zip"
if (-not (Test-Path -LiteralPath $zip)) {
  Invoke-WebRequest `
    -Uri "https://github.com/ggml-org/whisper.cpp/releases/download/v1.8.6/whisper-bin-x64.zip" `
    -OutFile $zip
}

Expand-Archive -LiteralPath $zip -DestinationPath $dir -Force

$model = Join-Path $dir "ggml-small.bin"
if (-not (Test-Path -LiteralPath $model) -or ((Get-Item -LiteralPath $model).Length -eq 0)) {
  Invoke-WebRequest `
    -Uri "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-small.bin" `
    -OutFile $model
}

Get-ChildItem -File -Recurse -LiteralPath $dir |
  Where-Object { $_.Name -in @("whisper-cli.exe", "main.exe", "ggml-small.bin") } |
  Select-Object FullName, Length, LastWriteTime
