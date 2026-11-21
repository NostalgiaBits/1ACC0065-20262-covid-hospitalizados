# Riesgo de fallecimiento en pacientes hospitalizados por COVID-19 — regresión logística concurrente y distribuida en Go

**CC65 – Programación Concurrente y Distribuida · Trabajo Final 2026-20 · UPC**

| Integrante | Código | Rol |
|---|---|---|
| Gómez Rubina, Luis David | U20221C621 | Coordinador del grupo |
| Olivera Álvarez, Lizbeth Teresita | U201616851 | Integrante |
| Zegarra Garcia, Luis Enrique | U202123273 | Integrante |

Docente: Montalvo García Peter Jonathan

*Última actualización: 21 de noviembre de 2026 (Entregable 2 · PC4).*

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

Están versionados el CSV original (`data/raw/TB_HOSP_VAC_FALLECIDOS.csv`), el de entrenamiento (`data/train_1M.csv`) y el de prueba real (`data/test.csv`); `hospitalizados_limpio.csv` se genera con los scripts.

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

## Arquitectura

```
Usuario ──HTTP──> API coordinadora (Go, :8080) ──TCP──> nodo1 · nodo2 · nodo3 (Go, :9091)
                      │            │
                   MongoDB       Redis
```

| Parte | Carpeta | Qué hace | Entregable |
|---|---|---|---|
| A — Cargador concurrente | `cargador/`, `comun/datos.go` | lee el CSV por bloques: productor → W workers → agregador | PC3 |
| B — Entrenamiento distribuido | `entrenador/`, `comun/modelo.go`, `comun/cluster.go`, `nodo/` | regresión logística; gradiente repartido entre goroutines (PC3) y entre 3 nodos por TCP (PC4) | PC3 · PC4 |
| C — API de predicciones | `api/` | REST en Go: predicción < 100 ms, entrenamiento en el clúster, JWT, MongoDB, Redis, Swagger | PC4 |
| D — Visualización de impacto | `frontend/` | SPA con predicciones y mapa de riesgo | TB2 |

**Topología en estrella (parameter server).** La API guarda los pesos; en cada época envía los pesos a los 3 nodos, cada nodo
devuelve el gradiente de su rango de filas y la API los suma y actualiza. Se eligió frente a una topología en anillo
porque necesitamos un punto de suma, usa 2 saltos de red por época en lugar de N y, si un nodo cae, su rango se reasigna a otro
nodo sin detener el entrenamiento.

## Levantar el sistema (Entregable 2)

Requisitos: Docker Desktop y Go 1.26 (solo para el lanzador y las pruebas). Los comandos están escritos para PowerShell en Windows.

```powershell
copy .env.example .env                 # editar JWT_SECRETO, ADMIN_EMAIL y ADMIN_PASSWORD (.env no se sube a GitHub)
docker compose config --quiet          # valida el archivo
docker compose up --build -d           # construye y levanta: nodo1, nodo2, nodo3, api, mongo, redis
docker compose ps                      # estado y puertos
docker compose logs -f                 # IPs de los nodos y peticiones atendidas
go run ./lanzador 19091 gradiente      # prueba directa de nodo1 desde el host
```

| Servicio | IP en `my_network` | Puerto en el host |
|---|---|---|
| nodo1 · nodo2 · nodo3 | 172.20.0.2 · .3 · .4 (:9091) | 19091 · 29091 · 39091 |
| api | 172.20.0.10 | 8080 |
| mongo | 172.20.0.20 | 27017 |
| redis | 172.20.0.21 | 6379 |

**Dockerfile.** Usa la estructura básica (`FROM golang`, `WORKDIR`, `COPY`, `EXPOSE`, `CMD`), pero compila en la
construcción (`RUN go mod download` y `RUN go build`) en lugar de `CMD ["go","run",...]`: el proyecto es un módulo con varios
paquetes y dependencias (drivers de MongoDB y Redis, JWT, bcrypt), y así no se descargan ni se compilan en cada arranque, el
contenedor inicia al instante y los errores aparecen al construir. Un mismo Dockerfile sirve a la API y a los nodos
(`ARG PROGRAMA=api|nodo`).

## API

Documentación interactiva (Swagger): http://localhost:8080/docs

| Método | Ruta | Autenticación | Qué hace |
|---|---|---|---|
| GET | `/salud` | — | estado de la API, MongoDB, Redis y nodos |
| POST | `/registro` | — | crea un usuario (contraseña con bcrypt) |
| POST | `/login` | — | token JWT válido por 1 hora |
| POST | `/predecir` | — | probabilidad de fallecer de un paciente, riesgo y factores (< 100 ms) |
| POST | `/entrenar` | JWT, rol admin | entrena en el clúster TCP y activa el modelo nuevo |
| GET | `/modelo` | — | pesos, odds ratios, métricas y categorías válidas |
| GET | `/cluster` | — | topología y estado de los nodos |
| GET | `/metricas` | — | latencias p50/p95/p99, caché y conteos de las bases de datos |
| GET | `/entrenamientos` · `/predicciones` | JWT | historial guardado en MongoDB |

Ejemplo:

```powershell
curl -X POST http://localhost:8080/predecir -H "Content-Type: application/json" `
  -d '{"edad":78,"sexo":"M","dosis_previas":0,"fabricante":"NINGUNA","departamento":"LIMA","periodo":"2021-S1"}'
```

**Bases de datos.** MongoDB (base `covid`): colecciones `usuarios`, `modelos`, `entrenamientos` y `predicciones`. Redis:
`modelo:actual` (modelo vigente), `pred:*` (respuestas de `/predecir`, 1 hora) y contadores `metricas:*`.

**Postman.** Importar `postman/PC4.postman_collection.json` y `postman/local.postman_environment.json`, escribir `adminEmail` y
`adminPassword` en el entorno y ejecutar la colección: 25 peticiones con pruebas automáticas (códigos, < 100 ms, caché, JWT).

## Ejecutar (Entregable 1)

```powershell
pip install -r python/requirements.txt
python python/limpieza.py && python python/aumento.py            # datos: limpio, train_1M.csv (1 050 000 filas) y test.csv (real)
go run ./cargador   data/train_1M.csv 24                          # Parte A: <archivo> <workers>
go run ./entrenador data/train_1M.csv data/test.csv 24 500 1.0    # Parte B: <train> <test> <workers> <épocas> <tasa> [lote]
go test ./...                                                     # go test -race ./... necesita cgo: en Windows, desde WSL
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

## Resultados del Entregable 1 (Ryzen 9 7900X3D, 12 núcleos / 24 hilos, 1 050 000 filas, 100 épocas, media recortada de 10 corridas)

| Workers | Carga (s) | Speedup carga | Entrenamiento (s) | Speedup entren. | Eficiencia | Total (s) | Speedup total | Pérdida final | AUC | CPU (s) | Memoria máx. (MB) |
|---|---|---|---|---|---|---|---|---|---|---|---|
| 1 | 0.340 | 1.00× | 8.154 | 1.00× | 100 % | 8.494 | 1.00× | 0.450189 | 0.8104 | 7.6 | 583 |
| 2 | 0.194 | 1.75× | 4.210 | 1.94× | 97 % | 4.404 | 1.93× | 0.450189 | 0.8104 | 9.0 | 592 |
| 4 | 0.142 | 2.40× | 2.014 | 4.05× | 101 % | 2.156 | 3.94× | 0.450189 | 0.8104 | 8.7 | 611 |
| 8 | 0.124 | 2.75× | 1.157 | 7.05× | 88 % | 1.280 | 6.63× | 0.450189 | 0.8104 | 9.4 | 688 |
| 12 | 0.101 | 3.35× | 0.868 | 9.39× | 78 % | 0.970 | 8.76× | 0.450189 | 0.8104 | 10.0 | 723 |
| **16** | 0.096 | 3.53× | 0.698 | **11.69×** | 73 % | 0.794 | 10.70× | 0.450189 | 0.8104 | 10.9 | 715 |
| 24 | 0.112 | 3.03× | 0.896 | 9.10× | 38 % | 1.008 | 8.42× | 0.450189 | 0.8104 | 11.3 | 714 |

El speedup del entrenamiento crece casi linealmente hasta 8 workers y alcanza su máximo con **16** (11.7×); con 24 (hilos SMT) baja porque cada iteración dura pocos milisegundos y el cálculo está limitado por el ancho de banda de memoria. La pérdida final y el AUC son idénticos en todas las configuraciones: el paralelismo no cambia el resultado.

Modelo final (500 épocas, tasa 1.0, batch completo): AUC **0.8137** sobre la prueba real; con mini-lotes de 4 096 filas, 0.8146; scikit-learn con los mismos datos: 0.8146. Con el umbral que maximiza F1 (0.28) el modelo detecta el 76 % de los fallecimientos.

## Resultados del Entregable 2

| Nodos | Tiempo de 100 épocas en Docker (mediana de 5) | Speedup | Pérdida final |
|---|---|---|---|
| 1 | 2058 ms | 1.00× | 0.450189 |
| 2 | 1162 ms | 1.77× | 0.450189 |
| 3 | 1005 ms | 2.05× | 0.450189 |

- El entrenamiento distribuido da **el mismo modelo** que el local (prueba `TestClusterIgualALocal`).
- Si un nodo se cae, su rango se reasigna a otro y el entrenamiento termina (prueba `TestClusterReasignaNodoCaido`).
- Postman (newman) en Docker: 25 peticiones y 77 pruebas, todas en verde.
- `/predecir` responde el 100 % de las peticiones en menos de 100 ms (p99 ≈ 4.5 ms con 20 clientes concurrentes en Docker, `bench/latencia`).

## Estructura del repositorio

```
python/      limpieza.py · aumento.py · validar_sklearn.py · graficos.py · requirements.txt
comun/       datos.go (Parte A) · modelo.go (Parte B) · protocolo.go · cluster.go · nodo.go · *_test.go
cargador/    entrenador/   nodo/   lanzador/       programas (main.go)
api/         main.go · handlers.go · seguridad.go · store.go · metricas.go · openapi.yaml · main_test.go
bench/       run_bench.ps1 · analyze.py · run_cluster.ps1 · latencia/ · resultados
postman/     PC4.postman_collection.json · local.postman_environment.json
data/        raw/TB_HOSP_VAC_FALLECIDOS.csv · train_1M.csv · test.csv · codebook.json · modelo.json · muestra_1000.csv
docs/        PARTICIPACION-PC3.md · PARTICIPACION-PC4.md
Dockerfile · docker-compose.yml · .env.example
```

## Flujo de trabajo (GitFlow)

`main` (versión estable de cada entregable) ← `develop` (integración) ← `feature/*` (una rama por funcionalidad, con pull
request). Seguimiento de tareas en el tablero Kanban del repositorio: https://github.com/users/NostalgiaBits/projects/1.
