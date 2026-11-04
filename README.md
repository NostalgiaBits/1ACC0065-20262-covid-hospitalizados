# Riesgo de fallecimiento en pacientes hospitalizados por COVID-19 — regresión logística concurrente y distribuida en Go

**CC65 – Programación Concurrente y Distribuida · Trabajo Final 2026-20 · UPC**

| Integrante | Código | Rol |
|---|---|---|
| Gómez Rubina, Luis David | U20221C621 | Coordinador del grupo |
| Olivera Álvarez, Lizbeth Teresita | U201616851 | Integrante |
| Zegarra Garcia, Luis Enrique | U202123273 | Integrante |

Docente: Montalvo García Peter Jonathan

## Problema y pregunta de impacto social

Caso 1 del enunciado: *Predicción de riesgo epidemiológico nacional (COVID-19)* con **Logistic Regression Distribuida**.

> Cuando un paciente con COVID-19 ingresa a un hospital del Perú, ¿qué probabilidad tiene de fallecer según su edad, sexo,
> vacunación previa, región y momento de la pandemia?

La respuesta sirve para priorizar camas UCI y oxígeno en los hospitales con pacientes de mayor riesgo y para focalizar las
campañas de refuerzo en los grupos donde la vacuna previa más reduce la mortalidad. El modelo **apoya la asignación de
recursos; nunca decide negar atención a nadie** (ver sección de responsabilidad ética en el informe).

## Dataset

Plataforma Nacional de Datos Abiertos del Perú (MINSA). Búsqueda usada: "HOSPITALIZADOS VACUNADOS Y FALLECIDOS POR COVID 19"
(https://www.datosabiertos.gob.pe/?query=HOSPITALIZADOS%20VACUNADOS%20Y%20FALLECIDOS%20POR%20COVID%2019&sort_by=changed&sort_order=DESC).

| Fuente | Archivo | Uso |
|---|---|---|
| [Hospitalizados, vacunados y fallecidos por COVID-19](https://www.datosabiertos.gob.pe/dataset/hospitalizados-vacunados-y-fallecidos-por-covid-19) · [recurso CSV](https://www.datosabiertos.gob.pe/dataset/hospitalizados-vacunados-y-fallecidos-por-covid-19/resource/89513307-af15-49f3-9045) | `TB_HOSP_VAC_FALLECIDOS.csv`, `Diccionario de variables.xlsx` | entrenamiento y prueba (este entregable) |
| [Fallecidos, hospitalizados y vacunados por COVID-19](https://www.datosabiertos.gob.pe/dataset/fallecidos-hospitalizados-y-vacunados-por-covid-19) | `TB_FALLECIDO_HOSP_VAC.csv`, `DD_TB_FALLECIDOS_HOSP_VAC_0.xlsx` | solo fallecidos (124 485 filas, 81 % no hospitalizados): no sirve para entrenar; reservado para el mapa de impacto (Entregable 3) |

Descargar `TB_HOSP_VAC_FALLECIDOS.csv` en `data/raw/` antes de ejecutar los scripts.

| Etapa | Filas | Descripción |
|---|---|---|
| Original `TB_HOSP_VAC_FALLECIDOS.csv` | 155 345 | una fila por hospitalización, 42 columnas, 2020–2025, datos anonimizados |
| Limpio `data/hospitalizados_limpio.csv` | 143 309 | sin filas con edad o sexo faltante; 8 columnas; 26.0 % fallecidos |
| Prueba `data/test.csv` | 28 644 | 20 % de las **personas**, solo datos reales |
| Entrenamiento `data/train_1M.csv` | **1 050 000** | 114 665 reales + 935 335 sintéticas (bootstrap estratificado, edad ± 2 años, semilla 42) |

Variable objetivo: `cdc_fallecido_covid`. Variables de entrada: edad, sexo, **dosis aplicadas al menos 14 días antes del
ingreso** (0, 1, 2, 3+), fabricante de la primera dosis previa, departamento de domicilio y periodo de ingreso (ola).
Se excluyen las variables que ocurren después del ingreso (UCI, oxígeno, ventilación, evolución) y `flag_vacuna`, que cuenta
dosis posteriores a la hospitalización y filtra el resultado (fuga de información: 62.9 % de mortalidad con 0 dosis frente a
0.4 % con 4; con las dosis previas reales el efecto es 29.2 % → 5.9 %).

## Arquitectura (enunciado, 4 partes)

```
Parte A  cargador/   + comun/datos.go    carga concurrente del CSV: productor por bloques -> W workers -> agregador   (PC3)
Parte B  entrenador/ + comun/modelo.go   regresión logística, gradiente paralelo por datos, parameter server síncrono (PC3)
Parte C  api/                            coordinador REST/WebSocket + nodos TCP + MongoDB + Redis + Docker Compose      (PC4)
Parte D  frontend/                       SPA con predicciones en tiempo real e impacto social                           (TB2)
```

## Ejecutar (Entregable 1)

```powershell
pip install -r python/requirements.txt
python python/limpieza.py && python python/aumento.py            # datos (ver docs/GUIA-EJECUCION.md)
go run ./cargador   data/train_1M.csv 24                          # Parte A: <archivo> <workers>
go run ./entrenador data/train_1M.csv data/test.csv 24 500 1.0    # Parte B: <train> <test> <workers> <épocas> <tasa> [lote]
go test ./...
powershell -ExecutionPolicy Bypass -File bench/run_bench.ps1 ; python bench/analyze.py
```

## Diseño concurrente

- **Carga (fan-out / fan-in):** un productor lee el archivo en bloques de 4 MB cortando siempre en un salto de línea y los
  envía por un canal; W workers parsean, validan y codifican (one-hot, edad estandarizada) cada bloque en memoria propia y
  envían el resultado por otro canal; el agregador los ordena por índice de bloque. Sin memoria compartida ni mutex.
- **Entrenamiento (paralelismo de datos):** en cada iteración W goroutines calculan el gradiente parcial de su rango de filas
  en un buffer propio y lo envían por un canal; la goroutine principal es la única que suma los parciales (en orden fijo) y
  actualiza los pesos. Es el esquema *parameter server* síncrono que en el Entregable 2 se reparte entre nodos por TCP.
- **Resultado determinista:** la pérdida final y las métricas son idénticas con 1 y con 24 workers.
- **Predicción:** pool de workers sobre rangos disjuntos del conjunto de prueba.

## Resultados (Ryzen 9 7900X3D, 12 núcleos / 24 hilos, 1 050 000 filas, 100 épocas, media recortada de 10 corridas)

Ver `bench/tabla.md`. Modelo (500 épocas, tasa 1.0): AUC 0.8137 en prueba real; scikit-learn con los mismos datos: 0.8146.

## Estructura del repositorio

```
python/      limpieza.py · aumento.py · validar_sklearn.py · graficos.py · requirements.txt
comun/       datos.go (Parte A) · modelo.go (Parte B) · *_test.go
cargador/    main.go     entrenador/  main.go
bench/       run_bench.ps1 · analyze.py · results.csv · tabla.md · resumen.json
data/        codebook.json · muestra_1000.csv  (los CSV grandes se generan localmente; no se versionan)
docs/        GUIA-EJECUCION.md
```

## Flujo de trabajo (GitFlow)

`main` (versión estable de cada entregable) ← `develop` (integración) ← `feature/*` (una rama por funcionalidad, con pull
request). Seguimiento de tareas en el tablero Kanban del repositorio (GitHub Projects).
