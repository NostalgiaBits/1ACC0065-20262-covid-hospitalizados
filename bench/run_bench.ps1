# Benchmark: ejecuta el entrenador con distintos numeros de workers (un proceso por corrida), 1 calentamiento + N repeticiones.
# Mide por separado la carga (Parte A) y el entrenamiento (Parte B), y muestrea CPU y memoria del proceso.
#
#   powershell -ExecutionPolicy Bypass -File bench/run_bench.ps1
#   powershell -ExecutionPolicy Bypass -File bench/run_bench.ps1 -Workers "1,2,4,8,12,16,24" -Repeticiones 10 -Epocas 100
#
# Salida: bench/results.csv (una fila por corrida) y bench/salidas/<workers>_<rep>.txt (consola completa)
param(
    [string]$Workers = "1,2,4,8,12,16,24",
    [int]$Repeticiones = 10,
    [int]$Epocas = 100,
    [double]$Tasa = 1.0,
    [string]$Train = "data/train_1M.csv",
    [string]$Test = "data/test.csv"
)
$ErrorActionPreference = "Stop"
$raiz = Split-Path $PSScriptRoot -Parent
Set-Location $raiz
New-Item -ItemType Directory -Force bench/salidas | Out-Null
go build -o bin/entrenador.exe ./entrenador
if ($LASTEXITCODE -ne 0) { throw "no compila" }

$csv = "bench/results.csv"
if (-not (Test-Path $csv)) {
    "workers,repeticion,carga_ms,entren_ms,epocas,perdida,auc,f1,cpu_s,mem_max_mb" | Out-File $csv -Encoding utf8
}

foreach ($w in ($Workers -split "," | ForEach-Object { [int]$_.Trim() })) {
    for ($rep = 0; $rep -le $Repeticiones; $rep++) {   # rep 0 = calentamiento (no se guarda)
        $salida = "bench/salidas/${w}_${rep}.txt"
        $p = Start-Process -FilePath "bin/entrenador.exe" -ArgumentList "$Train $Test $w $Epocas $Tasa 0 bench/salidas/modelo_tmp.json" `
             -RedirectStandardOutput $salida -PassThru -NoNewWindow
        $memMax = 0
        while (-not $p.HasExited) {
            try { $p.Refresh(); if ($p.WorkingSet64 -gt $memMax) { $memMax = $p.WorkingSet64 } } catch {}
            Start-Sleep -Milliseconds 100
        }
        $cpu = 0; try { $cpu = $p.TotalProcessorTime.TotalSeconds } catch {}
        $linea = Select-String -Path $salida -Pattern "^RESUMEN" | Select-Object -Last 1
        if (-not $linea) { Write-Host "W=$w rep=${rep}: sin RESUMEN (ver $salida)"; continue }
        $v = @{}
        ($linea.Line -replace "^RESUMEN ", "") -split " " | ForEach-Object { $k, $val = $_ -split "="; $v[$k] = $val }
        $msg = "W=$w rep=$rep carga=$($v.carga_ms) ms entren=$($v.entren_ms) ms perdida=$($v.perdida) auc=$($v.auc) mem=$([math]::Round($memMax/1MB)) MB"
        if ($rep -eq 0) { Write-Host "$msg (calentamiento)"; continue }
        Write-Host $msg
        "$w,$rep,$($v.carga_ms),$($v.entren_ms),$($v.epocas),$($v.perdida),$($v.auc),$($v.f1),$([math]::Round($cpu,2)),$([math]::Round($memMax/1MB))" |
            Out-File $csv -Append -Encoding utf8
    }
}
Remove-Item bench/salidas/modelo_tmp.json -ErrorAction SilentlyContinue
Write-Host "Listo: $csv"
