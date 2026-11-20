# Speedup del entrenamiento distribuido: entrena con 1, 2 y 3 nodos via la API (cada uno N repeticiones).
#   powershell -ExecutionPolicy Bypass -File bench/run_cluster.ps1 -Email admin@... -Password ... [-Epocas 100] [-Repeticiones 5]
# Salida: bench/cluster.csv
param(
    [Parameter(Mandatory = $true)][string]$Email,
    [Parameter(Mandatory = $true)][string]$Password,
    [string]$Url = "http://localhost:8080",
    [int]$Epocas = 100,
    [int]$Repeticiones = 5
)
$ErrorActionPreference = "Stop"
$raiz = Split-Path $PSScriptRoot -Parent
$login = Invoke-RestMethod -Method Post -Uri "$Url/login" -ContentType "application/json" -Body (@{ email = $Email; password = $Password } | ConvertTo-Json)
$cab = @{ Authorization = "Bearer $($login.token)" }
$csv = Join-Path $raiz "bench/cluster.csv"
"nodos,repeticion,ms,ms_por_epoca,perdida,auc,reasignaciones" | Out-File $csv -Encoding utf8
foreach ($n in 1, 2, 3) {
    for ($rep = 0; $rep -le $Repeticiones; $rep++) {   # rep 0 = calentamiento
        $cuerpo = @{ epocas = $Epocas; tasa = 1.0; nodos = $n } | ConvertTo-Json
        $r = Invoke-RestMethod -Method Post -Uri "$Url/entrenar" -Headers $cab -ContentType "application/json" -Body $cuerpo -TimeoutSec 600
        $linea = "$n,$rep,$($r.ms),$($r.ms_por_epoca),$($r.perdida_final),$($r.auc),$($r.reasignaciones)"
        Write-Host "nodos=$n rep=$rep  $([math]::Round($r.ms)) ms  perdida=$($r.perdida_final)  auc=$($r.auc)"
        if ($rep -gt 0) { $linea | Out-File $csv -Append -Encoding utf8 }
    }
}
Write-Host "Listo: $csv"
