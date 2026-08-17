param(
    [string]$BinaryPath = "C:\\deploy-agent\\deploy-agent.exe",
    [string]$ConfigPath = "C:\\deploy-agent\\config.json",
    [string]$TaskName = "DeployAgent"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $BinaryPath)) { throw "Binary not found: $BinaryPath" }
if (-not (Test-Path $ConfigPath)) { throw "Config not found: $ConfigPath" }

$action = New-ScheduledTaskAction -Execute $BinaryPath -Argument "-config `"$ConfigPath`" once"
$trigger = New-ScheduledTaskTrigger -Once -At (Get-Date).AddMinutes(1) -RepetitionInterval (New-TimeSpan -Minutes 1)
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
$settings = New-ScheduledTaskSettingsSet -StartWhenAvailable -MultipleInstances IgnoreNew

Register-ScheduledTask -TaskName $TaskName -Action $action -Trigger $trigger -Principal $principal -Settings $settings -Force | Out-Null
Start-ScheduledTask -TaskName $TaskName
Write-Host "Installed scheduled task $TaskName"
